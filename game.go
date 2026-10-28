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
	roadW      = roadX1 - roadX0 // 406

	laneCount = 6
	laneWidth = roadW / laneCount // ~67
)

type Game struct {
	player      Car
	enemies     []Car
	particles   []Particle
	sprites     map[CarKind]*Sprite
	scrollSpeed float64
	boost       float64
	spawnTimer  float64
	markOffset  float64
	frame       int
	gameOver    bool
	score       int
	highScore   int
}

func NewGame() *Game {
	g := &Game{}
	g.loadSprites()
	g.reset()
	return g
}

func (g *Game) loadSprites() {
	g.sprites = map[CarKind]*Sprite{
		KindPlayer:    LoadSprite("player.png", color.RGBA{G: 200, B: 60, A: 255}),
		KindSedan:     LoadSprite("car_sedan.png", color.RGBA{R: 200, G: 60, B: 60, A: 255}),
		KindSport:     LoadSprite("car_sport.png", color.RGBA{R: 60, G: 120, B: 220, A: 255}),
		KindSUV:       LoadSprite("car_suv.png", color.RGBA{R: 80, G: 150, B: 80, A: 255}),
		KindMinibus:   LoadSprite("minibus.png", color.RGBA{R: 240, G: 200, B: 60, A: 255}),
		KindBus:       LoadSprite("bus.png", color.RGBA{R: 70, G: 140, B: 80, A: 255}),
		KindAmbulance: LoadSprite("ambulance.png", color.RGBA{R: 245, G: 245, B: 245, A: 255}),
		KindTruck:     LoadSprite("truck.png", color.RGBA{R: 220, G: 160, B: 40, A: 255}),
	}
}

func (g *Game) reset() {
	g.player = Car{
		X:    laneCenterX(3, carW),
		Y:    float64(screenHeight - carH - 30),
		W:    carW,
		H:    carH,
		Kind: KindPlayer,
	}
	g.enemies = nil
	g.particles = nil
	g.scrollSpeed = startSpeed
	g.boost = 0
	g.spawnTimer = 0
	g.gameOver = false
	g.score = 0
	g.readHighScore()
}

func (g *Game) effectiveSpeed() float64 {
	eff := g.scrollSpeed + g.boost
	if eff < 1.2 {
		eff = 1.2
	}
	if eff > maxSpeed+boostMax {
		eff = maxSpeed + boostMax
	}
	return eff
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
		return // гарантия проходимости: минимум 3 свободные полосы
	}
	g.enemies = append(g.enemies, RandomEnemy(free[rand.Intn(len(free))]))
}

func (g *Game) Update() error {
	// выход по ESC (и крестик окна работает штатно)
	if inpututil.IsKeyJustPressed(ebiten.KeyEscape) {
		return ebiten.Termination
	}

	g.frame++
	g.updateParticles()

	if g.gameOver {
		if inpututil.IsKeyJustPressed(ebiten.KeyR) {
			g.reset()
		}
		return nil
	}

	// разгон / торможение игрока
	up := ebiten.IsKeyPressed(ebiten.KeyUp) || ebiten.IsKeyPressed(ebiten.KeyW)
	down := ebiten.IsKeyPressed(ebiten.KeyDown) || ebiten.IsKeyPressed(ebiten.KeyS)
	switch {
	case up && !down:
		g.boost += boostGain
	case down && !up:
		g.boost -= brakeGain
	default:
		if g.boost > 0 {
			g.boost -= boostGain
			if g.boost < 0 {
				g.boost = 0
			}
		} else {
			g.boost += brakeGain
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

	// движение игрока влево-вправо
	move := 0.0
	if ebiten.IsKeyPressed(ebiten.KeyLeft) || ebiten.IsKeyPressed(ebiten.KeyA) {
		move -= playerSpeed
	}
	if ebiten.IsKeyPressed(ebiten.KeyRight) || ebiten.IsKeyPressed(ebiten.KeyD) {
		move += playerSpeed
	}
	g.player.X += move
	if g.player.X < float64(roadX0+2) {
		g.player.X = float64(roadX0 + 2)
	}
	if maxX := float64(roadX1 - carW - 2); g.player.X > maxX {
		g.player.X = maxX
	}

	eff := g.effectiveSpeed()
	g.markOffset += eff

	// спавн врагов: чаще на высокой скорости
	g.spawnTimer += eff
	if g.spawnTimer > 160 {
		g.spawnTimer = 0
		g.spawnEnemy()
		g.scrollSpeed += 0.02
		if g.scrollSpeed > maxSpeed {
			g.scrollSpeed = maxSpeed
		}
	}

	g.updateEnemies(eff)

	// уехавшие за экран — очки за обгон
	kept := g.enemies[:0]
	for _, e := range g.enemies {
		if e.Y > float64(screenHeight) {
			g.score += e.Points
			if g.score > g.highScore {
				g.highScore = g.score
			}
			continue
		}
		kept = append(kept, e)
	}
	g.enemies = kept

	// столкновения с игроком
	for _, e := range g.enemies {
		if hit(g.player, e) {
			g.gameOver = true
			g.saveHighScore()
			g.explode(g.player.X+g.player.W/2, g.player.Y+g.player.H/2)
			break
		}
	}

	return nil
}

func (g *Game) Draw(screen *ebiten.Image) {
	// фон и дорога
	vector.FillRect(screen, 0, 0, screenWidth, screenHeight, color.RGBA{R: 96, G: 128, B: 70, A: 255}, false)
	vector.FillRect(screen, roadX0, 0, roadW, screenHeight, color.RGBA{R: 60, G: 62, B: 66, A: 255}, false)
	vector.FillRect(screen, roadX0, 0, 4, screenHeight, color.RGBA{R: 230, G: 200, B: 60, A: 255}, false)
	vector.FillRect(screen, roadX1-4, 0, 4, screenHeight, color.RGBA{R: 230, G: 200, B: 60, A: 255}, false)

	// движущаяся пунктирная разметка
	cycle := 60.0
	offset := math.Mod(g.markOffset, cycle)
	for i := 1; i < laneCount; i++ {
		x := float32(roadX0 + i*laneWidth - 2)
		for y := -cycle + offset; y < screenHeight; y += cycle {
			vector.FillRect(screen, x, float32(y), 4, 30, color.RGBA{R: 235, G: 235, B: 200, A: 255}, false)
		}
	}

	for _, e := range g.enemies {
		drawCar(screen, e, g.sprites[e.Kind])
		drawBlinker(screen, e, e.Blink, g.frame)
		drawBrakeLights(screen, e, e.Braking)
	}
	drawCar(screen, g.player, g.sprites[KindPlayer])
	// стопы игрока — при торможении стрелками
	drawBrakeLights(screen, g.player, g.boost < -0.01)

	g.drawParticles(screen)

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

	if g.gameOver {
		vector.FillRect(screen, 0, 0, screenWidth, screenHeight, color.RGBA{R: 0, G: 0, B: 0, A: 170}, false)
		drawTextCentered(screen, "GAME OVER", screenHeight/2-60, 30, color.RGBA{R: 255, G: 90, B: 90, A: 255})
		drawTextCentered(screen, fmt.Sprintf("Score: %d   Best: %d", g.score, g.highScore), screenHeight/2, 20, color.White)
		if g.score >= g.highScore && g.score > 0 {
			drawTextCentered(screen, "NEW RECORD!", screenHeight/2+30, 20, color.RGBA{R: 255, G: 220, B: 80, A: 255})
		}
		drawTextCentered(screen, "Press R to restart", screenHeight/2+64, 18, color.White)
	}
}

func (g *Game) Layout(outsideWidth, outsideHeight int) (int, int) {
	return screenWidth, screenHeight
}
