package main

import (
	"fmt"
	"image/color"
	"math"
	"math/rand"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/inpututil"
	"github.com/hajimehoshi/ebiten/v2/vector"
)

const (
	screenWidth  = 430
	screenHeight = 900

	roadMargin = 12
	roadX0     = roadMargin
	roadX1     = screenWidth - roadMargin
	roadW      = roadX1 - roadX0

	laneCount = 6
	laneWidth = roadW / laneCount

	// вертикальное «дыхание» игрока: разгон — вперёд, торможение — назад
	playerYBase  = screenHeight - carH - 30
	playerYMax   = screenHeight - carH - 10
	playerYMinF  = 0.45 // доля высоты экрана: выше середины не поднимаемся
	playerYReach = 26.0 // пикселей смещения на 1.0 boost
	playerYEase  = 0.08 // плавность возврата

	stageClearBonus = 25
	bannerDuration  = 2.5 // секунды показа баннера STAGE CLEAR
)

type GameState int

const (
	StateMenu GameState = iota
	StatePlaying
	StatePaused
	StateGameOver
)

type Game struct {
	state       GameState
	player      Car
	enemies     []Car
	particles   []Particle
	sprites     map[CarKind]*Sprite
	playerSkins []*Sprite
	skinNames   []string
	skinIdx     int

	scrollSpeed float64
	boost       float64
	spawnTimer  float64
	markOffset  float64
	frame       float64 // игровые кадры (в единицах 60 FPS) — для фаз мигания

	score     int
	highScore int
	distance  float64

	playerBraking bool

	// этапы
	stageIdx  int
	stageTime float64
	stageLoop int
	banner    float64 // таймер баннера, в игровых кадрах

	audio *AudioSys
	muted bool

	lastTickFrame float64
}

func NewGame() *Game {
	g := &Game{}
	g.loadSprites()
	g.loadSkins()
	g.readSettings()
	if g.skinIdx >= len(g.playerSkins) || g.skinIdx < 0 {
		g.skinIdx = 0
	}
	g.audio = NewAudio()
	g.readHighScore()
	return g
}

func (g *Game) loadSprites() {
	g.sprites = map[CarKind]*Sprite{
		KindSedan:     LoadSprite("car_sedan.png", color.RGBA{R: 200, G: 60, B: 60, A: 255}),
		KindSport:     LoadSprite("car_sport.png", color.RGBA{R: 60, G: 120, B: 220, A: 255}),
		KindSUV:       LoadSprite("car_suv.png", color.RGBA{R: 80, G: 150, B: 80, A: 255}),
		KindMinibus:   LoadSprite("minibus.png", color.RGBA{R: 240, G: 200, B: 60, A: 255}),
		KindBus:       LoadSprite("bus.png", color.RGBA{R: 70, G: 140, B: 80, A: 255}),
		KindAmbulance: LoadSprite("ambulance.png", color.RGBA{R: 245, G: 245, B: 245, A: 255}),
		KindTruck:     LoadSprite("truck.png", color.RGBA{R: 220, G: 160, B: 40, A: 255}),
	}
}

func (g *Game) loadSkins() {
	for _, m := range skinMeta {
		g.playerSkins = append(g.playerSkins, LoadSprite(m.file, m.fallback))
		g.skinNames = append(g.skinNames, m.name)
	}
}

// timeScale — множитель времени относительно 60 FPS. На 144 Гц ≈ 2.4,
// на 60 Гц = 1. Все константы движения в коде рассчитаны на 60 FPS
// и умножаются на dt, поэтому скорость игры одинакова на любом мониторе.
func (g *Game) timeScale() float64 {
	tps := ebiten.ActualTPS()
	if tps <= 1 {
		return 1
	}
	s := tps / 60.0
	if s < 0.5 {
		s = 0.5
	}
	if s > 3 {
		s = 3
	}
	return s
}

func (g *Game) reset() {
	g.player = Car{
		X:    laneCenterX(3, carW),
		Y:    float64(playerYBase),
		W:    carW,
		H:    carH,
		Kind: KindPlayer,
	}
	g.enemies = nil
	g.particles = nil
	g.scrollSpeed = stageDefs[0].ScrollStart
	g.boost = 0
	g.spawnTimer = 0
	g.score = 0
	g.distance = 0
	g.stageIdx = 0
	g.stageTime = 0
	g.stageLoop = 0
	g.banner = 0
	g.readHighScore()
}

func (g *Game) effectiveSpeed() float64 {
	eff := g.scrollSpeed + g.boost
	if eff < 1.2 {
		eff = 1.2
	}
	if eff > g.scrollSpeed+boostMax {
		eff = g.scrollSpeed + boostMax
	}
	return eff
}

func (g *Game) stageName(i int) string {
	name := stageDefs[i].Name
	if g.stageLoop > 0 {
		name = fmt.Sprintf("%s · LAP %d", name, g.stageLoop+1)
	}
	return name
}

// ---------- Update: диспетчер состояний ----------

func (g *Game) Update() error {
	switch g.state {
	case StateMenu:
		return g.updateMenu()
	case StatePlaying:
		g.updatePlaying()
	case StatePaused:
		return g.updatePaused()
	case StateGameOver:
		g.updateGameOver()
	}
	return nil
}

func (g *Game) updatePlaying() {
	dt := g.timeScale()
	g.frame += dt

	// пауза
	if inpututil.IsKeyJustPressed(ebiten.KeyP) || inpututil.IsKeyJustPressed(ebiten.KeyEscape) {
		g.state = StatePaused
		g.audio.SilenceEngine()
		return
	}
	// звук вкл/выкл
	if inpututil.IsKeyJustPressed(ebiten.KeyM) {
		g.muted = !g.muted
		g.saveSettings()
	}

	g.updateParticles(dt)

	// разгон / торможение игрока
	up := ebiten.IsKeyPressed(ebiten.KeyUp) || ebiten.IsKeyPressed(ebiten.KeyW)
	down := ebiten.IsKeyPressed(ebiten.KeyDown) || ebiten.IsKeyPressed(ebiten.KeyS)
	g.playerBraking = down && !up
	switch {
	case up && !down:
		g.boost += boostGain * dt
	case down && !up:
		g.boost -= brakeGain * dt
	default:
		if g.boost > 0 {
			g.boost -= boostGain * dt
			if g.boost < 0 {
				g.boost = 0
			}
		} else {
			g.boost += brakeGain * dt
			if g.boost > 0 {
				g.boost = 0
			}
		}
	}
	if g.boost > boostMax {
		g.boost = boostMax
	}
	if g.boost < -brakeMax {
		g.boost = -brakeMax
	}

	// вертикальное положение: газ — вперёд, тормоз — назад
	targetY := float64(playerYBase) - g.boost*playerYReach
	if targetY < float64(screenHeight)*playerYMinF {
		targetY = float64(screenHeight) * playerYMinF
	}
	if targetY > float64(playerYMax) {
		targetY = float64(playerYMax)
	}
	g.player.Y += (targetY - g.player.Y) * math.Min(1, playerYEase*dt)

	// движение влево-вправо
	move := 0.0
	if ebiten.IsKeyPressed(ebiten.KeyLeft) || ebiten.IsKeyPressed(ebiten.KeyA) {
		move -= playerSpeed
	}
	if ebiten.IsKeyPressed(ebiten.KeyRight) || ebiten.IsKeyPressed(ebiten.KeyD) {
		move += playerSpeed
	}
	g.player.X += move * dt
	if g.player.X < float64(roadX0+2) {
		g.player.X = float64(roadX0 + 2)
	}
	if maxX := float64(roadX1 - carW - 2); g.player.X > maxX {
		g.player.X = maxX
	}

	eff := g.effectiveSpeed()
	g.markOffset += eff * dt
	g.distance += eff * dt * 5 / 60 // условные метры (eff*18 км/ч → м/с)

	g.updateStage(dt)

	// спавн: интервал зависит от этапа и круга
	every := stageDefs[g.stageIdx].SpawnEvery - float64(g.stageLoop)*15
	if every < 110 {
		every = 110
	}
	g.spawnTimer += eff * dt
	if g.spawnTimer > every {
		g.spawnTimer = 0
		g.spawnEnemy()
	}

	g.updateEnemies(eff, dt)

	// ближний обгон + уехавшие за экран
	kept := g.enemies[:0]
	for _, e := range g.enemies {
		if !e.NearMiss && e.Y+e.H > g.player.Y {
			e.NearMiss = true
			dx := math.Abs((e.X + e.W/2) - (g.player.X + g.player.W/2))
			if dx < (e.W+g.player.W)/2+8 {
				g.audio.PlaySqueal(0.7, g.muted)
				g.score += 1
			}
		}
		if e.Y > float64(screenHeight) {
			g.score += e.Points
			continue
		}
		kept = append(kept, e)
	}
	g.enemies = kept
	if g.score > g.highScore {
		g.highScore = g.score
	}

	// столкновения
	for _, e := range g.enemies {
		if hit(g.player, e) {
			g.state = StateGameOver
			g.saveHighScore()
			g.audio.PlayCrash(0.9, g.muted)
			g.audio.SilenceEngine()
			g.explode(g.player.X+g.player.W/2, g.player.Y+g.player.H/2)
			break
		}
	}

	g.audio.UpdateEngine(eff, g.muted)
	g.updateBlinkerSound()
}

func (g *Game) updateStage(dt float64) {
	def := stageDefs[g.stageIdx]
	g.stageTime += dt

	// плавный разгон внутри этапа + усложнение с каждым кругом
	k := g.stageTime / def.Frames
	if k > 1 {
		k = 1
	}
	g.scrollSpeed = def.ScrollStart + (def.ScrollEnd-def.ScrollStart)*k +
		float64(g.stageLoop)*0.3

	if g.stageTime >= def.Frames {
		g.score += stageClearBonus
		if g.score > g.highScore {
			g.highScore = g.score
		}
		g.saveHighScore()
		g.banner = bannerDuration * 60
		g.stageIdx++
		if g.stageIdx >= len(stageDefs) {
			g.stageIdx = 0
			g.stageLoop++
		}
		g.stageTime = 0
		g.spawnTimer = 0
	}
	if g.banner > 0 {
		g.banner -= dt
	}
}

func (g *Game) updatePaused() error {
	if inpututil.IsKeyJustPressed(ebiten.KeyP) || inpututil.IsKeyJustPressed(ebiten.KeyEnter) {
		g.state = StatePlaying
		return nil
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyEscape) {
		g.saveSettings()
		return ebiten.Termination
	}
	return nil
}

func (g *Game) updateGameOver() {
	dt := g.timeScale()
	g.frame += dt
	g.updateParticles(dt)
	// мир живёт без игрока: машины разъезжаются
	g.updateEnemies(g.effectiveSpeed(), dt)

	if inpututil.IsKeyJustPressed(ebiten.KeyR) {
		g.reset()
		g.state = StatePlaying
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyEscape) {
		g.state = StateMenu
	}
}

// updateBlinkerSound — «тик-тик» поворотников ближайших машин.
func (g *Game) updateBlinkerSound() {
	if int(g.frame/10)%2 != 0 {
		return
	}
	if g.frame-g.lastTickFrame < 9 {
		return
	}
	for i := range g.enemies {
		e := &g.enemies[i]
		if e.Blink == 0 {
			continue
		}
		dy := e.Y - g.player.Y
		if dy < -250 || dy > 350 {
			continue
		}
		prox := 1 - math.Min(1, math.Abs(dy)/350)
		g.audio.PlayTick(0.12+0.3*prox, g.muted)
		g.lastTickFrame = g.frame
		return
	}
}

func (g *Game) occupiedLanes(bandY float64) map[int]bool {
	occ := map[int]bool{}
	for _, e := range g.enemies {
		if e.Y < bandY {
			occ[laneOfX(e.X+e.W/2)] = true
		}
	}
	return occ
}

func (g *Game) spawnEnemy() {
	const bandY = 300
	occ := g.occupiedLanes(bandY)

	free := make([]int, 0, laneCount)
	for lane := 0; lane < laneCount; lane++ {
		if !occ[lane] {
			free = append(free, lane)
		}
	}
	if len(free) <= 2 {
		return
	}
	g.enemies = append(g.enemies, RandomEnemy(free[rand.Intn(len(free))]))
}

// ---------- Draw ----------

func (g *Game) Draw(screen *ebiten.Image) {
	if g.state == StateMenu {
		g.drawMenu(screen)
		return
	}
	g.drawWorld(screen)
}

func (g *Game) drawWorld(screen *ebiten.Image) {
	def := stageDefs[g.stageIdx]
	night := def.Night

	vector.FillRect(screen, 0, 0, screenWidth, screenHeight, def.Grass, false)
	vector.FillRect(screen, roadX0, 0, roadW, screenHeight, def.Asphalt, false)

	edge := color.RGBA{R: 230, G: 200, B: 60, A: 255}
	mark := color.RGBA{R: 235, G: 235, B: 200, A: 255}
	if night {
		edge = color.RGBA{R: 140, G: 122, B: 40, A: 255}
		mark = color.RGBA{R: 170, G: 170, B: 140, A: 255}
	}
	vector.FillRect(screen, roadX0, 0, 4, screenHeight, edge, false)
	vector.FillRect(screen, roadX1-4, 0, 4, screenHeight, edge, false)

	cycle := 60.0
	offset := math.Mod(g.markOffset, cycle)
	for i := 1; i < laneCount; i++ {
		x := float32(roadX0 + i*laneWidth - 2)
		for y := -cycle + offset; y < screenHeight; y += cycle {
			vector.FillRect(screen, x, float32(y), 4, 30, mark, false)
		}
	}

	for _, e := range g.enemies {
		drawCar(screen, e, g.sprites[e.Kind])
		drawBlinker(screen, e, e.Blink, int(g.frame))
		drawBrakeLights(screen, e, e.Braking)
	}
	drawCar(screen, g.player, g.playerSkins[g.skinIdx])
	drawBrakeLights(screen, g.player, g.playerBraking)

	// ночь: затемнение + конусы фар + красные ореолы стопов
	if night {
		vector.FillRect(screen, 0, 0, screenWidth, screenHeight, color.RGBA{R: 6, G: 8, B: 24, A: 150}, false)
		for _, e := range g.enemies {
			drawHeadlights(screen, e)
			drawTailGlow(screen, e)
		}
		drawHeadlights(screen, g.player)
		drawTailGlow(screen, g.player)
	}

	g.drawParticles(screen)

	// HUD
	speedKmh := int(g.effectiveSpeed() * 18)
	speedClr := color.RGBA{R: 200, G: 200, B: 200, A: 255}
	if g.boost > 0.01 {
		speedClr = color.RGBA{R: 120, G: 255, B: 120, A: 255}
	} else if g.boost < -0.01 {
		speedClr = color.RGBA{R: 255, G: 120, B: 120, A: 255}
	}
	drawText(screen, fmt.Sprintf("Score: %d", g.score), 16, 24, 18, color.RGBA{R: 255, G: 230, B: 120, A: 255})
	drawText(screen, fmt.Sprintf("Best: %d", g.highScore), 16, 50, 18, color.RGBA{R: 120, G: 220, B: 255, A: 255})
	drawText(screen, fmt.Sprintf("Speed: %d km/h", speedKmh), 16, 76, 16, speedClr)
	drawText(screen, fmt.Sprintf("Dist: %d m", int(g.distance)), 16, 100, 14, color.White)
	drawText(screen, g.stageName(g.stageIdx), screenWidth-170, 24, 16, color.White)
	if g.muted {
		drawText(screen, "MUTED (M)", screenWidth-120, 50, 14, color.RGBA{R: 160, G: 160, B: 160, A: 255})
	}

	// баннер STAGE CLEAR
	if g.banner > 0 {
		vector.FillRect(screen, 0, screenHeight/2-80, screenWidth, 116, color.RGBA{R: 0, G: 0, B: 0, A: 210}, false)
		drawTextCentered(screen, "STAGE CLEAR!", screenHeight/2-62, 34, color.RGBA{R: 255, G: 220, B: 80, A: 255})
		drawTextCentered(screen, fmt.Sprintf("BONUS +%d", stageClearBonus), screenHeight/2-14, 22, color.White)
		drawTextCentered(screen, "NEXT: "+g.stageName(g.stageIdx), screenHeight/2+16, 18, color.RGBA{R: 160, G: 220, B: 255, A: 255})
	}

	if g.state == StatePaused {
		vector.FillRect(screen, 0, 0, screenWidth, screenHeight, color.RGBA{R: 0, G: 0, B: 0, A: 170}, false)
		drawTextCentered(screen, "PAUSED", screenHeight/2-40, 34, color.White)
		drawTextCentered(screen, "P — продолжить", screenHeight/2+10, 18, color.White)
		drawTextCentered(screen, "ESC — выход", screenHeight/2+40, 18, color.White)
	}

	if g.state == StateGameOver {
		vector.FillRect(screen, 0, 0, screenWidth, screenHeight, color.RGBA{R: 0, G: 0, B: 0, A: 170}, false)
		drawTextCentered(screen, "GAME OVER", screenHeight/2-60, 30, color.RGBA{R: 255, G: 90, B: 90, A: 255})
		drawTextCentered(screen, fmt.Sprintf("Score: %d   Best: %d", g.score, g.highScore), screenHeight/2, 20, color.White)
		if g.score >= g.highScore && g.score > 0 {
			drawTextCentered(screen, "NEW RECORD!", screenHeight/2+30, 20, color.RGBA{R: 255, G: 220, B: 80, A: 255})
		}
		drawTextCentered(screen, "R — restart    ESC — menu", screenHeight/2+64, 18, color.White)
	}
}

func (g *Game) Layout(outsideWidth, outsideHeight int) (int, int) {
	return screenWidth, screenHeight
}
