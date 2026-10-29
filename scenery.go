package main

import (
	"image/color"
	"math"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/vector"
)

const scenerySpacing = 90.0

// hashIdx — детерминированный «шум» по индексу ряда: узор стабилен
// при каждом появлении ряда на экране.
func hashIdx(idx int) int {
	h := idx * 2654435761
	h ^= h >> 13
	h *= 1274126177
	h ^= h >> 16
	if h < 0 {
		h = -h
	}
	return h
}

func (g *Game) drawScenery(screen *ebiten.Image, def StageDef) {
	// frac растёт с пройденной дистанцией → все ряды уезжают вниз,
	// как дорожная разметка. base смещает хеш рядов, чтобы сверху
	// появлялись новые декорации, а не зацикленный узор.
	frac := math.Mod(g.markOffset, scenerySpacing)
	base := int(g.markOffset / scenerySpacing)
	rows := screenHeight/int(scenerySpacing) + 2

	for j := -1; j <= rows; j++ {
		y := frac + float64(j)*scenerySpacing
		if y < -scenerySpacing || y > screenHeight+scenerySpacing {
			continue
		}
		idx := base - j
		h := hashIdx(idx)

		switch def.Name {
		case "CITY", "DAWN":
			g.drawCityRow(screen, y, h)
		case "DUSK":
			g.drawCityRow(screen, y, h)
		case "HIGHWAY":
			g.drawHighwayRow(screen, y, h)
		case "NIGHT":
			g.drawNightRow(screen, y, h)
		}
	}

	// отбойники на трассе — сплошные полосы вдоль всей дороги
	if def.Name == "HIGHWAY" {
		rail := color.RGBA{R: 165, G: 168, B: 175, A: 255}
		railDark := color.RGBA{R: 110, G: 112, B: 118, A: 255}
		vector.FillRect(screen, float32(roadX0-7), 0, 5, screenHeight, railDark, false)
		vector.FillRect(screen, float32(roadX0-6), 0, 3, screenHeight, rail, false)
		vector.FillRect(screen, float32(roadX1+2), 0, 5, screenHeight, railDark, false)
		vector.FillRect(screen, float32(roadX1+3), 0, 3, screenHeight, rail, false)
	}
}

// drawCityRow — деревья и кусты вдоль дороги.
func (g *Game) drawCityRow(screen *ebiten.Image, y float64, h int) {
	side := h % 2
	ox := float64(h%9) - 4 // лёгкий сдвиг по X

	trunk := color.RGBA{R: 92, G: 64, B: 40, A: 255}
	if side == 0 {
		// большое дерево слева
		vector.FillRect(screen, float32(3+ox*0.4), float32(y+6), 3, 10, trunk, false)
		vector.FillCircle(screen, float32(4+ox*0.4), float32(y+2), 13,
			color.RGBA{R: 46, G: 110, B: 52, A: 255}, false)
		vector.FillCircle(screen, float32(0+ox*0.4), float32(y-3), 9,
			color.RGBA{R: 58, G: 130, B: 62, A: 255}, false)
	} else {
		// куст справа
		vector.FillCircle(screen, float32(screenWidth-6-ox*0.4), float32(y+8), 7,
			color.RGBA{R: 54, G: 118, B: 58, A: 255}, false)
		vector.FillCircle(screen, float32(screenWidth-11-ox*0.4), float32(y+13), 5,
			color.RGBA{R: 66, G: 136, B: 66, A: 255}, false)
	}
	// изредка второе дерево с той же стороны
	if h%5 == 0 {
		vector.FillCircle(screen, float32(screenWidth-5-ox*0.4), float32(y+40), 11,
			color.RGBA{R: 40, G: 100, B: 48, A: 255}, false)
	}
}

// drawHighwayRow — редкие дорожные знаки на серых полях.
func (g *Game) drawHighwayRow(screen *ebiten.Image, y float64, h int) {
	if h%4 != 0 {
		return
	}
	pole := color.RGBA{R: 120, G: 122, B: 126, A: 255}
	sign := color.RGBA{R: 40, G: 90, B: 200, A: 255}
	x := float32(roadX0 - 8)
	if h%8 == 0 {
		x = float32(roadX1 + 2)
	}
	vector.FillRect(screen, x+2, float32(y-10), 2, 14, pole, false)
	vector.FillRect(screen, x, float32(y-18), 6, 9, sign, false)
}

// drawNightRow — фонари: столбик на обочине и тёплое пятно света на асфальте.
// Ряды плавно проявляются сверху и тают снизу — без «выскакивания».
func (g *Game) drawNightRow(screen *ebiten.Image, y float64, h int) {
	fadeTop := clamp01((y + 34) / 50)
	fadeBot := clamp01((screenHeight - y) / 50)
	fade := fadeTop
	if fadeBot < fade {
		fade = fadeBot
	}
	if fade <= 0.01 {
		return
	}

	pole := color.RGBA{R: 96, G: 98, B: 106, A: 255}
	glowOut := color.RGBA{R: 255, G: 214, B: 140, A: uint8(16 * fade)}
	glowMid := color.RGBA{R: 255, G: 220, B: 150, A: uint8(26 * fade)}
	glowIn := color.RGBA{R: 255, G: 228, B: 165, A: uint8(40 * fade)}

	if h%3 == 0 {
		vector.FillRect(screen, float32(roadX0-8), float32(y+4), 3, 12, pole, false)
		vector.FillCircle(screen, float32(roadX0-4), float32(y+6), 24, glowOut, false)
		vector.FillCircle(screen, float32(roadX0-4), float32(y+6), 14, glowMid, false)
		vector.FillCircle(screen, float32(roadX0-4), float32(y+6), 7, glowIn, false)
	}
	if h%3 == 1 {
		vector.FillRect(screen, float32(roadX1+5), float32(y+40), 3, 12, pole, false)
		vector.FillCircle(screen, float32(roadX1+4), float32(y+42), 24, glowOut, false)
		vector.FillCircle(screen, float32(roadX1+4), float32(y+42), 14, glowMid, false)
		vector.FillCircle(screen, float32(roadX1+4), float32(y+42), 7, glowIn, false)
	}
}
