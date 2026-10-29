package main

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	_ "image/png"
	"math"
	"math/rand"
	"os"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/inpututil"
	"github.com/hajimehoshi/ebiten/v2/vector"
)

// ---------- константы экрана и дороги ----------

const (
	screenWidth  = 520
	screenHeight = 1000

	roadX0    = 40
	roadX1    = 460
	laneCount = 6
	laneWidth = (roadX1 - roadX0) / laneCount // 70

	playerYBase = screenHeight - 210
)

type State int

const (
	StateMenu State = iota
	StatePlaying
	StatePaused
	StateGameOver
)

// spark — искра взрыва. Своя мини-система, чтобы не зависеть от полей
// Particle из particle.go.
type spark struct {
	X, Y, Vx, Vy    float64
	Life, Max, Size float64
	Clr             color.RGBA
}

type Game struct {
	state State

	player  Car
	enemies []Car
	sparks  []spark

	// оставлено для совместимости с particle.go — здесь не используется
	particles []Particle

	highScore int
	bonus     int
	distance  float64

	scrollSpeed float64
	spawnTimer  float64
	markOffset  float64

	stageIdx  int
	stageTime float64
	stageLoop int
	banner    float64

	boost         float64
	playerBraking bool

	frame float64

	muted      bool
	skinIdx    int
	startStage int

	audio *AudioSys

	sprites     map[CarKind]Sprite
	playerSkins []Sprite
	skinNames   []string
}

// ---------- загрузка спрайтов ----------

func loadSprite(path string, fallback color.RGBA) Sprite {
	data, err := os.ReadFile(path)
	if err != nil {
		return Sprite{Fill: fallback}
	}
	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return Sprite{Fill: fallback}
	}
	return Sprite{Img: ebiten.NewImageFromImage(img), Fill: fallback}
}

// ---------- создание и сброс ----------

func NewGame() *Game {
	g := &Game{
		state: StateMenu,
		audio: NewAudio(),
		sprites: map[CarKind]Sprite{
			KindSedan:     loadSprite("assets/car_sedan.png", color.RGBA{R: 170, G: 60, B: 55, A: 255}),
			KindSport:     loadSprite("assets/car_sport.png", color.RGBA{R: 220, G: 170, B: 40, A: 255}),
			KindSUV:       loadSprite("assets/car_suv.png", color.RGBA{R: 55, G: 120, B: 75, A: 255}),
			KindBus:       loadSprite("assets/bus.png", color.RGBA{R: 235, G: 185, B: 40, A: 255}),
			KindMinibus:   loadSprite("assets/minibus.png", color.RGBA{R: 220, G: 225, B: 230, A: 255}),
			KindTruck:     loadSprite("assets/truck.png", color.RGBA{R: 130, G: 135, B: 145, A: 255}),
			KindAmbulance: loadSprite("assets/ambulance.png", color.RGBA{R: 240, G: 244, B: 248, A: 255}),
		},
	}
	for _, m := range skinMeta {
		g.playerSkins = append(g.playerSkins, loadSprite("assets/"+m.file, m.fallback))
		g.skinNames = append(g.skinNames, m.name)
	}
	g.readSettings()
	g.readHighScore()
	if g.skinIdx < 0 || g.skinIdx >= len(g.playerSkins) {
		g.skinIdx = 0
	}
	if g.startStage < 0 || g.startStage >= len(stageDefs) {
		g.startStage = 0
	}
	g.reset(g.startStage)
	return g
}

func (g *Game) reset(startStage int) {
	g.player = Car{
		X:    laneCenterX(3, carW),
		Y:    float64(playerYBase),
		W:    carW,
		H:    carH,
		Kind: KindPlayer,
	}
	g.enemies = nil
	g.sparks = nil
	g.boost = 0
	g.spawnTimer = 0.8
	g.bonus = 0
	g.distance = 0
	g.markOffset = 0
	g.stageIdx = startStage % len(stageDefs)
	g.stageTime = 0
	g.stageLoop = 0
	g.banner = 2.0
	g.scrollSpeed = stageDefs[g.stageIdx].ScrollStart
	g.readHighScore()
}

func (g *Game) curScore() int {
	return int(g.distance/10) + g.bonus
}

// ---------- Update: диспетчер состояний ----------

func (g *Game) Update() error {
	switch g.state {
	case StateMenu:
		g.audio.SilenceEngine() // в меню мотор молчит
		return g.updateMenu()
	case StatePlaying:
		g.updatePlaying()
	case StatePaused:
		g.audio.SilenceEngine()
		g.updatePaused()
	case StateGameOver:
		g.audio.SilenceEngine()
		g.updateGameOver()
	}
	return nil
}

func (g *Game) Draw(screen *ebiten.Image) {
	switch g.state {
	case StateMenu:
		g.drawMenu(screen)
	default:
		g.drawWorld(screen)
		g.drawHUD(screen)
		if g.banner > 0 {
			g.drawBanner(screen)
		}
		if g.state == StatePaused {
			g.drawPause(screen)
		}
		if g.state == StateGameOver {
			g.drawGameOver(screen)
		}
	}
}

func (g *Game) Layout(outsideWidth, outsideHeight int) (int, int) {
	return screenWidth, screenHeight
}

// ---------- игровая логика ----------

func (g *Game) updatePlaying() {
	dt := 1.0 / 60.0
	g.frame++

	if inpututil.IsKeyJustPressed(ebiten.KeyP) {
		g.state = StatePaused
		return
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyM) {
		g.muted = !g.muted
		g.saveSettings()
	}

	// руль: поворотник в «кадрах», как у врагов
	const steer = 340.0
	if ebiten.IsKeyPressed(ebiten.KeyLeft) || ebiten.IsKeyPressed(ebiten.KeyA) {
		g.player.X -= steer * dt
		g.player.Blink = -1
		g.player.BlinkTimer = 36
	}
	if ebiten.IsKeyPressed(ebiten.KeyRight) || ebiten.IsKeyPressed(ebiten.KeyD) {
		g.player.X += steer * dt
		g.player.Blink = 1
		g.player.BlinkTimer = 36
	}
	if g.player.BlinkTimer > 0 {
		g.player.BlinkTimer -= 60 * dt
	}
	g.player.X = clamp(g.player.X, float64(roadX0+4), float64(roadX1-4-carW))

	// газ и тормоз
	g.playerBraking = ebiten.IsKeyPressed(ebiten.KeyS) || ebiten.IsKeyPressed(ebiten.KeyDown)
	if ebiten.IsKeyPressed(ebiten.KeyW) || ebiten.IsKeyPressed(ebiten.KeyUp) {
		g.boost += (1 - g.boost) * math.Min(1, 5*dt)
	} else {
		g.boost += (0 - g.boost) * math.Min(1, 4*dt)
	}

	// смена этапа
	def := stageDefs[g.stageIdx]
	g.stageTime += dt
	if g.stageTime >= def.Duration {
		g.stageTime = 0
		g.stageIdx = (g.stageIdx + 1) % len(stageDefs)
		if g.stageIdx == 0 {
			g.stageLoop++
		}
		g.banner = 2.0
	}
	if g.banner > 0 {
		g.banner -= dt
	}

	// плавный разгон внутри этапа
	prog := math.Min(1, g.stageTime/def.Duration)
	target := def.ScrollStart + (def.ScrollMax-def.ScrollStart)*prog
	g.scrollSpeed += (target - g.scrollSpeed) * math.Min(1, 2*dt)
	eff := g.scrollSpeed * (1 + 0.35*g.boost)
	if g.playerBraking {
		eff *= 0.6
	}

	// мир едет
	mark := eff * dt
	g.markOffset += mark
	g.distance += mark / 30

	// звук мотора
	g.audio.UpdateEngine(eff/100, g.boost, dt, g.playerBraking, g.muted)

	// спавн врагов
	g.spawnTimer -= dt
	if g.spawnTimer <= 0 {
		g.spawnEnemy()
		g.spawnTimer = (260 + rand.Float64()*260) / math.Max(eff, 1)
	}

	// ИИ врагов: eff/60 — px за кадр (единицы OwnSpeed), 60*dt — dt в кадрах
	g.updateEnemies(eff/60.0, 60.0*dt)

	// ближний обгон, столкновения, вылет за экран
	for i := len(g.enemies) - 1; i >= 0; i-- {
		e := &g.enemies[i]

		if !e.NearMiss && e.Y+e.H > g.player.Y && e.Y < g.player.Y+g.player.H {
			e.NearMiss = true
			dx := math.Abs((e.X + e.W/2) - (g.player.X + g.player.W/2))
			if dx < (e.W+g.player.W)/2+10 {
				g.bonus += e.Points
			}
		}

		if hit(g.player, *e) {
			g.crash()
			return
		}

		if e.Y > screenHeight+200 {
			g.enemies = append(g.enemies[:i], g.enemies[i+1:]...)
		}
	}

	// искры взрыва
	for i := len(g.sparks) - 1; i >= 0; i-- {
		p := &g.sparks[i]
		p.X += p.Vx * dt
		p.Y += p.Vy * dt
		p.Vx *= 0.98
		p.Vy = p.Vy*0.98 + 60*dt
		p.Life -= dt
		if p.Life <= 0 {
			g.sparks = append(g.sparks[:i], g.sparks[i+1:]...)
		}
	}
}

func (g *Game) crash() {
	g.audio.PlayCrash(0.5, g.muted)
	cx, cy := g.player.X+g.player.W/2, g.player.Y+g.player.H/2
	for i := 0; i < 46; i++ {
		ang := rand.Float64() * 2 * math.Pi
		sp := 60 + rand.Float64()*260
		life := 0.5 + rand.Float64()*0.6
		col := color.RGBA{R: 255, G: uint8(120 + rand.Intn(120)), B: 40, A: 255}
		if rand.Intn(3) == 0 {
			col = color.RGBA{R: 90, G: 90, B: 95, A: 255}
		}
		g.sparks = append(g.sparks, spark{
			X: cx, Y: cy,
			Vx:   math.Cos(ang) * sp,
			Vy:   math.Sin(ang)*sp - 60,
			Life: life, Max: life,
			Size: 2 + rand.Float64()*3,
			Clr:  col,
		})
	}
	if g.curScore() > g.highScore {
		g.highScore = g.curScore()
		g.saveHighScore()
	}
	g.state = StateGameOver
}

func (g *Game) updatePaused() {
	if inpututil.IsKeyJustPressed(ebiten.KeyP) {
		g.state = StatePlaying
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyEscape) {
		g.state = StateMenu
	}
}

func (g *Game) updateGameOver() {
	if inpututil.IsKeyJustPressed(ebiten.KeyR) {
		g.reset(g.startStage)
		g.state = StatePlaying
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyEscape) {
		g.state = StateMenu
	}
}

// spawnEnemy — через фабрику RandomEnemy из car.go, с проверкой,
// что верхняя часть полосы свободна.
func (g *Game) spawnEnemy() {
	lx := rand.Intn(laneCount)
	lcx := laneCenterX(lx, carW)
	for _, e := range g.enemies {
		if math.Abs(e.X-lcx) < laneWidth && e.Y < -60 {
			return // полоса сверху занята — пропускаем спавн
		}
	}
	g.enemies = append(g.enemies, RandomEnemy(lx))
}

// ---------- отрисовка ----------

func (g *Game) drawWorld(screen *ebiten.Image) {
	def := g.currentDef()

	vector.FillRect(screen, 0, 0, screenWidth, screenHeight, def.Grass, false)
	vector.FillRect(screen, roadX0, 0, roadX1-roadX0, screenHeight, def.Asphalt, false)

	edge := color.RGBA{R: 235, G: 235, B: 240, A: 255}
	vector.FillRect(screen, roadX0+2, 0, 3, screenHeight, edge, false)
	vector.FillRect(screen, roadX1-5, 0, 3, screenHeight, edge, false)

	// разметка полос — едет вниз вместе с миром
	const dashLen, spacing = 42, 88
	frac := math.Mod(g.markOffset, spacing)
	for j := -1; j <= screenHeight/spacing+1; j++ {
		y := frac + float64(j)*spacing
		for l := 1; l < laneCount; l++ {
			x := roadX0 + laneWidth*l - 2
			vector.FillRect(screen, float32(x), float32(y), 4, dashLen, def.Mark, false)
		}
	}

	g.drawScenery(screen, def)

	// ночь: затемнение и лучи фар — до машин
	dark := def.Night
	if dark > 0.02 {
		vector.FillRect(screen, 0, 0, screenWidth, screenHeight,
			color.RGBA{R: 8, G: 10, B: 30, A: uint8(110 * dark)}, false)
	}
	if dark > 0.2 {
		k := (dark - 0.2) / 0.8
		for _, e := range g.enemies {
			drawHeadlights(screen, e, k)
		}
		drawHeadlights(screen, g.player, k)
	}

	for _, e := range g.enemies {
		sp := g.sprites[e.Kind]
		drawCar(screen, e, &sp)
		drawBlinker(screen, e, e.Blink, int(g.frame))
		drawBrakeLights(screen, e, e.Braking)
	}
	ps := g.playerSkins[g.skinIdx]
	drawCar(screen, g.player, &ps)
	drawBrakeLights(screen, g.player, g.playerBraking)

	// ночь: задние габариты поверх машин
	if dark > 0.2 {
		for _, e := range g.enemies {
			drawTailLights(screen, e)
		}
		drawTailLights(screen, g.player)
	}

	// искры взрыва
	for _, p := range g.sparks {
		c := p.Clr
		c.A = uint8(clamp(255*p.Life/p.Max, 0, 255))
		vector.FillCircle(screen, float32(p.X), float32(p.Y), float32(p.Size), c, false)
	}
}

func (g *Game) drawHUD(screen *ebiten.Image) {
	def := g.currentDef()
	eff := g.scrollSpeed * (1 + 0.35*g.boost)
	if g.playerBraking {
		eff *= 0.6
	}

	gold := color.RGBA{R: 255, G: 210, B: 60, A: 255}
	cyan := color.RGBA{R: 120, G: 220, B: 255, A: 255}
	grey := color.RGBA{R: 200, G: 200, B: 210, A: 255}
	green := color.RGBA{R: 120, G: 255, B: 120, A: 255}

	drawText(screen, fmt.Sprintf("Score: %d", g.curScore()), 12, 10, 20, gold)
	drawText(screen, fmt.Sprintf("Best: %d", g.highScore), 12, 38, 20, cyan)
	drawText(screen, fmt.Sprintf("Speed: %d km/h", int(eff*0.3)), 12, 66, 18, green)
	drawText(screen, fmt.Sprintf("Dist: %d m", int(g.distance)), 12, 92, 18, grey)
	drawTextCentered(screen, def.Name, 14, 18, grey)
	if g.muted {
		drawTextCentered(screen, "MUTED (M)", 42, 16, grey)
	}
}

func (g *Game) drawBanner(screen *ebiten.Image) {
	a := g.banner / 2
	if a > 1 {
		a = 1
	}
	clr := color.RGBA{R: 255, G: 210, B: 60, A: uint8(255 * a)}
	drawTextCentered(screen, stageDefs[g.stageIdx].Name, 160, 44, clr)
}

func (g *Game) drawPause(screen *ebiten.Image) {
	vector.FillRect(screen, 0, 0, screenWidth, screenHeight,
		color.RGBA{R: 0, G: 0, B: 0, A: 130}, false)
	drawTextCentered(screen, "PAUSE", screenHeight/2-50, 40,
		color.RGBA{R: 255, G: 210, B: 60, A: 255})
	drawTextCentered(screen, "P — продолжить, ESC — в меню", screenHeight/2+16, 18, color.White)
}

func (g *Game) drawGameOver(screen *ebiten.Image) {
	vector.FillRect(screen, 0, 0, screenWidth, screenHeight,
		color.RGBA{R: 60, G: 10, B: 10, A: 150}, false)
	drawTextCentered(screen, "CRASH!", screenHeight/2-140, 48,
		color.RGBA{R: 255, G: 80, B: 60, A: 255})
	drawTextCentered(screen, fmt.Sprintf("Score: %d", g.curScore()), screenHeight/2-60, 26,
		color.RGBA{R: 255, G: 210, B: 60, A: 255})
	drawTextCentered(screen, fmt.Sprintf("Best: %d", g.highScore), screenHeight/2-16, 22,
		color.RGBA{R: 120, G: 220, B: 255, A: 255})
	drawTextCentered(screen, "R — заново, ESC — меню", screenHeight/2+40, 18, color.White)
}

// ---------- утилиты ----------

func clamp(v, lo, hi float64) float64 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}
