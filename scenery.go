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
	ai, bi, t := g.stageBlend()
	if ai == bi || t <= 0.001 {
		g.drawDecorAlpha(screen, stageDefs[ai].Name, 1)
	} else {
		g.drawDecorAlpha(screen, stageDefs[ai].Name, 1-t)
		g.drawDecorAlpha(screen, stageDefs[bi].Name, t)
	}
}

// drawDecorAlpha — рисует декорации этапа name с прозрачностью alpha:
// сперва на временный холст, затем на экран с масштабом альфы.
func (g *Game) drawDecorAlpha(screen *ebiten.Image, name string, alpha float64) {
	if alpha <= 0.01 {
		return
	}
	if g.scratch == nil {
		g.scratch = ebiten.NewImage(screenWidth, screenHeight)
	}
	g.scratch.Clear()
	g.drawDecorFor(g.scratch, name)

	a := uint8(clamp(alpha*255, 0, 255))
	op := &ebiten.DrawImageOptions{}
	op.ColorScale.ScaleWithColor(color.RGBA{R: a, G: a, B: a, A: a})
	screen.DrawImage(g.scratch, op)
}

// drawDecorFor — декорации конкретного этапа, включая его отбойники.
func (g *Game) drawDecorFor(screen *ebiten.Image, name string) {
	frac := math.Mod(g.markOffset, scenerySpacing)
	base := int(g.markOffset / scenerySpacing)
	rows := screenHeight/int(scenerySpacing) + 2

	for j := -1; j <= rows; j++ {
		y := frac + float64(j)*scenerySpacing
		if y < -scenerySpacing || y > screenHeight+scenerySpacing {
			continue
		}
		h := hashIdx(base - j)

		switch name {
		case "CITY", "DAWN", "DUSK":
			g.drawCityRow(screen, y, h)
		case "DESERT":
			g.drawDesertRow(screen, y, h)
		case "HIGHWAY":
			g.drawHighwayRow(screen, y, h)
		case "NIGHT":
			g.drawNightRow(screen, y, h)
		case "BRIDGE":
			g.drawBridgeRow(screen, y, h)
		}
	}
	if name == "HIGHWAY" {
		g.drawRails(screen)
	}
	if name == "BRIDGE" {
		g.drawBridgeRails(screen)
	}
}

// drawBridgeRails — бетонные парапеты моста вместо отбойников.
// Непрерывные, как рельсы шоссе, — тают и проявляются вместе с этапом.
func (g *Game) drawBridgeRails(screen *ebiten.Image) {
	conc := color.RGBA{R: 172, G: 174, B: 182, A: 255}
	concHi := color.RGBA{R: 204, G: 206, B: 212, A: 255}
	concDark := color.RGBA{R: 122, G: 124, B: 132, A: 255}
	// левый парапет
	vector.FillRect(screen, float32(roadX0-11), 0, 9, screenHeight, concDark, false)
	vector.FillRect(screen, float32(roadX0-11), 0, 7, screenHeight, conc, false)
	vector.FillRect(screen, float32(roadX0-11), 0, 2, screenHeight, concHi, false)
	// правый парапет
	vector.FillRect(screen, float32(roadX1+2), 0, 9, screenHeight, concDark, false)
	vector.FillRect(screen, float32(roadX1+2), 0, 7, screenHeight, conc, false)
	vector.FillRect(screen, float32(roadX1+7), 0, 2, screenHeight, concHi, false)
}

// drawBridgeRow — ряд моста: деформационные швы на проезжей части,
// столбики парапета, речная рябь и тёплые блики низкого солнца по воде.
func (g *Game) drawBridgeRow(screen *ebiten.Image, y float64, h int) {
	// шов поперёк дороги — изредка, бежит вместе с миром
	if h%7 == 2 {
		vector.FillRect(screen, float32(roadX0), float32(y+38), float32(roadX1-roadX0), 3,
			color.RGBA{R: 66, G: 68, B: 76, A: 255}, false)
	}

	// столбики парапета — тёмные блоки на бетонных перилах
	post := color.RGBA{R: 138, G: 140, B: 148, A: 255}
	vector.FillRect(screen, float32(roadX0-11), float32(y+2), 7, 5, post, false)
	vector.FillRect(screen, float32(roadX1+4), float32(y+2), 7, 5, post, false)

	// рябь: короткие светлые штрихи по воде с обеих сторон
	water := color.RGBA{R: 150, G: 196, B: 224, A: 60}
	sun := color.RGBA{R: 255, G: 226, B: 176, A: 46}
	for i := 0; i < 3; i++ {
		r := hashIdx(h*7 + i*131)
		var x, w float32
		if r%2 == 0 {
			x = float32(4 + r%22)
			w = float32(14 + r%22)
		} else {
			x = float32(screenWidth - 20 - r%24)
			w = float32(14 + r%20)
		}
		c := water
		if r%9 == 3 {
			c = sun // редкий тёплый блик — солнце только что взошло
		}
		vector.FillRect(screen, x, float32(y+float64(8+r%70)), w, 2, c, false)
	}

	// знак на парапете — изредка: белая табличка с красным ободом
	if h%9 == 4 {
		x := float32(roadX0 - 18)
		if h%18 == 4 {
			x = float32(roadX1 + 11)
		}
		vector.FillCircle(screen, x, float32(y+14), 7, color.RGBA{R: 226, G: 48, B: 48, A: 255}, false)
		vector.FillCircle(screen, x, float32(y+14), 5, color.RGBA{R: 240, G: 240, B: 244, A: 255}, false)
	}
}

// drawRails — отбойники шоссе. Теперь часть декораций этапа,
// тают и проявляются вместе со всем остальным.
func (g *Game) drawRails(screen *ebiten.Image) {
	rail := color.RGBA{R: 165, G: 168, B: 175, A: 255}
	railDark := color.RGBA{R: 110, G: 112, B: 118, A: 255}
	vector.FillRect(screen, float32(roadX0-7), 0, 5, screenHeight, railDark, false)
	vector.FillRect(screen, float32(roadX0-6), 0, 3, screenHeight, rail, false)
	vector.FillRect(screen, float32(roadX1+2), 0, 5, screenHeight, railDark, false)
	vector.FillRect(screen, float32(roadX1+3), 0, 3, screenHeight, rail, false)
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

// drawDesertRow — пятна дюн, сагуаро, камни, сухие кусты. Изредка —
// череп у обочины, куда без него в пустыне.
func (g *Game) drawDesertRow(screen *ebiten.Image, y float64, h int) {
	side := h % 2
	ox := float64(h%9) - 4

	// редкое тёмное пятно «дюны» на обочине — оживляет ровный песок
	if r := h % 6; r == 1 || r == 4 {
		sand := color.RGBA{R: 196, G: 170, B: 118, A: 90}
		cx := float32(14 + ox)
		if side == 1 {
			cx = float32(screenWidth - 14 - ox)
		}
		vector.FillCircle(screen, cx, float32(y+16), 22, sand, false)
		vector.FillCircle(screen, cx+26, float32(y+26), 14, sand, false)
	}

	green := color.RGBA{R: 58, G: 104, B: 54, A: 255}
	dark := color.RGBA{R: 44, G: 84, B: 42, A: 255}

	switch r := h % 20; {
	case r == 0 || r == 10:
		// сагуаро: ствол, макушка и одна рука
		x := float32(10 + ox*0.5)
		if side == 1 {
			x = float32(screenWidth - 16 - ox*0.5)
		}
		vector.FillRect(screen, x, float32(y-8), 6, 30, green, false)
		vector.FillCircle(screen, x+3, float32(y-9), 3, green, false)
		if h%2 == 0 {
			vector.FillRect(screen, x-7, float32(y+2), 5, 10, green, false)
			vector.FillRect(screen, x-7, float32(y+9), 9, 4, green, false)
		} else {
			vector.FillRect(screen, x+8, float32(y+4), 5, 10, green, false)
			vector.FillRect(screen, x+5, float32(y+11), 8, 4, green, false)
		}
		vector.FillRect(screen, x+1, float32(y-8), 1, 30, dark, false) // тень на стволе
	case r == 2 || r == 12:
		// камни
		rock := color.RGBA{R: 150, G: 132, B: 108, A: 255}
		rockHi := color.RGBA{R: 176, G: 158, B: 132, A: 255}
		x := float32(6 + ox*0.5)
		if side == 1 {
			x = float32(screenWidth - 12 - ox*0.5)
		}
		vector.FillCircle(screen, x, float32(y+10), 8, rock, false)
		vector.FillCircle(screen, x+9, float32(y+13), 5, rock, false)
		vector.FillCircle(screen, x-2, float32(y+7), 4, rockHi, false)
	case r == 4 || r == 15:
		// сухой кустик
		dry := color.RGBA{R: 168, G: 140, B: 84, A: 255}
		x := float32(screenWidth - 10 - ox*0.5)
		if side == 1 {
			x = float32(8 + ox*0.5)
		}
		vector.FillCircle(screen, x, float32(y+12), 6, dry, false)
		vector.FillCircle(screen, x+6, float32(y+8), 4, dry, false)
	case r == 7:
		// череп у обочины — пустыня обязана
		bone := color.RGBA{R: 236, G: 228, B: 208, A: 255}
		x := float32(roadX0 - 12)
		vector.FillCircle(screen, x, float32(y+14), 5, bone, false)
		vector.FillRect(screen, x-6, float32(y+9), 5, 3, bone, false)
		vector.FillRect(screen, x-6, float32(y+15), 5, 3, bone, false)
	}
}

// drawHighwayRow — облик шоссе: фонари-стойки с тёплым светом через
// ряд, километровые столбики, изредка билборд.
func (g *Game) drawHighwayRow(screen *ebiten.Image, y float64, h int) {
	pole := color.RGBA{R: 108, G: 110, B: 118, A: 255}
	glowOut := color.RGBA{R: 255, G: 222, B: 160, A: 22}
	glowIn := color.RGBA{R: 255, G: 230, B: 175, A: 44}

	// фонарь через ряд, стороны чередуются
	if h%2 == 0 {
		vector.FillRect(screen, float32(roadX0-9), float32(y+6), 3, 22, pole, false)
		vector.FillRect(screen, float32(roadX0-9), float32(y+4), 9, 3, pole, false)
		vector.FillCircle(screen, float32(roadX0-4), float32(y+7), 16, glowOut, false)
		vector.FillCircle(screen, float32(roadX0-4), float32(y+7), 8, glowIn, false)
	} else {
		vector.FillRect(screen, float32(roadX1+6), float32(y+6), 3, 22, pole, false)
		vector.FillRect(screen, float32(roadX1+0), float32(y+4), 9, 3, pole, false)
		vector.FillCircle(screen, float32(roadX1+4), float32(y+7), 16, glowOut, false)
		vector.FillCircle(screen, float32(roadX1+4), float32(y+7), 8, glowIn, false)
	}

	// километровый столбик — маленькая белая тумба с серой шляпкой
	if h%5 == 0 {
		x := float32(roadX0 - 14)
		if h%10 == 5 {
			x = float32(roadX1 + 8)
		}
		vector.FillRect(screen, x, float32(y+12), 6, 10, color.RGBA{R: 228, G: 228, B: 232, A: 255}, false)
		vector.FillRect(screen, x, float32(y+10), 6, 4, color.RGBA{R: 60, G: 60, B: 66, A: 255}, false)
	}

	// билборд — редко, стоит на высоких ногах
	if h%11 == 3 {
		x := float32(screenWidth - 58)
		if h%22 == 3 {
			x = float32(4)
		}
		legs := color.RGBA{R: 96, G: 98, B: 104, A: 255}
		panel := color.RGBA{R: 30, G: 90, B: 170, A: 255}
		panelEdge := color.RGBA{R: 210, G: 214, B: 222, A: 255}
		vector.FillRect(screen, x+6, float32(y-16), 3, 18, legs, false)
		vector.FillRect(screen, x+38, float32(y-16), 3, 18, legs, false)
		vector.FillRect(screen, x, float32(y-44), 48, 30, panelEdge, false)
		vector.FillRect(screen, x+2, float32(y-42), 44, 26, panel, false)
		// «дорога» на щите — белая стрелка в перспективе
		vector.FillRect(screen, x+21, float32(y-38), 5, 16, color.RGBA{R: 235, G: 235, B: 240, A: 255}, false)
	}
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
