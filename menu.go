package main

import (
	"fmt"
	"image/color"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/inpututil"
	"github.com/hajimehoshi/ebiten/v2/text/v2"
	"github.com/hajimehoshi/ebiten/v2/vector"
)

type skinDef struct {
	file     string
	fallback color.RGBA
	name     string
}

var skinMeta = []skinDef{
	{"player.png", color.RGBA{R: 35, G: 50, B: 140, A: 255}, "RIO NAVY"},
	{"player_red.png", color.RGBA{R: 205, G: 50, B: 45, A: 255}, "RED STORM"},
	{"player_yellow.png", color.RGBA{R: 245, G: 195, B: 40, A: 255}, "SUNFIRE"},
	{"player_green.png", color.RGBA{R: 45, G: 165, B: 80, A: 255}, "EMERALD"},
	{"player_orange.png", color.RGBA{R: 240, G: 120, B: 35, A: 255}, "SUNSET"},
	{"player_purple.png", color.RGBA{R: 150, G: 62, B: 196, A: 255}, "VIOLET GT"},
	{"player_police.png", color.RGBA{R: 235, G: 238, B: 244, A: 255}, "PATRIOT POLICE"},
}

// drawTextCenteredX — центрирование по произвольной точке, с тенью.
func drawTextCenteredX(dst *ebiten.Image, s string, cx int, y int, size float64, clr color.Color) {
	sh := &text.DrawOptions{}
	sh.GeoM.Translate(float64(cx+2), float64(y+2))
	sh.ColorScale.ScaleWithColor(color.RGBA{R: 0, G: 0, B: 0, A: 180})
	sh.PrimaryAlign = text.AlignCenter
	text.Draw(dst, s, face(size), sh)

	op := &text.DrawOptions{}
	op.GeoM.Translate(float64(cx), float64(y))
	op.ColorScale.ScaleWithColor(clr)
	op.PrimaryAlign = text.AlignCenter
	text.Draw(dst, s, face(size), op)
}

func (g *Game) updateMenu() error {
	if inpututil.IsKeyJustPressed(ebiten.KeyEscape) {
		g.saveSettings()
		return ebiten.Termination
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyM) {
		g.muted = !g.muted
		g.saveSettings()
	}

	// выбор раскраски
	if inpututil.IsKeyJustPressed(ebiten.KeyLeft) {
		g.skinIdx = (g.skinIdx + len(g.playerSkins) - 1) % len(g.playerSkins)
		g.saveSettings()
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyRight) {
		g.skinIdx = (g.skinIdx + 1) % len(g.playerSkins)
		g.saveSettings()
	}

	// выбор сложности
	if inpututil.IsKeyJustPressed(ebiten.KeyTab) {
		g.difficulty = (g.difficulty + 1) % len(diffMult)
		g.saveSettings()
	}

	// выбор стартовой локации
	if inpututil.IsKeyJustPressed(ebiten.KeyUp) {
		g.startStage = (g.startStage + len(stageDefs) - 1) % len(stageDefs)
		g.saveSettings()
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyDown) {
		g.startStage = (g.startStage + 1) % len(stageDefs)
		g.saveSettings()
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyEnter) || inpututil.IsKeyJustPressed(ebiten.KeySpace) {
		g.reset(g.startStage)
		g.state = StatePlaying
	}
	return nil
}

func (g *Game) drawMenu(screen *ebiten.Image) {
	vector.FillRect(screen, 0, 0, screenWidth, screenHeight, color.RGBA{R: 22, G: 24, B: 32, A: 255}, false)

	drawTextCentered(screen, "GO RACING", 60, 44, color.RGBA{R: 255, G: 210, B: 60, A: 255})
	drawTextCentered(screen, "MINI", 112, 30, color.RGBA{R: 255, G: 210, B: 60, A: 255})

	// ---------- превью машины ----------
	s := g.playerSkins[g.skinIdx]
	const scale = 1.4
	pw, ph := 106.0*scale, 152.0*scale
	px := float64(screenWidth)/2 - pw/2
	py := 210.0
	if s.Img != nil {
		op := &ebiten.DrawImageOptions{}
		op.GeoM.Scale(scale, scale)
		op.GeoM.Translate(px, py)
		screen.DrawImage(s.Img, op)
	} else {
		vector.FillRect(screen, float32(px), float32(py), float32(pw), float32(ph), s.Fill, false)
	}

	white := color.White
	grey := color.RGBA{R: 160, G: 160, B: 170, A: 255}
	gold := color.RGBA{R: 255, G: 210, B: 60, A: 255}

	// ---------- выбор раскраски ----------
	nameY := int(py+ph) + 18
	drawTextCentered(screen, "<  "+g.skinNames[g.skinIdx]+"  >", nameY, 20, gold)
	drawText(screen, "◀", 60, nameY, 20, white)
	drawText(screen, "▶", screenWidth-78, nameY, 20, white)

	// ---------- выбор стартовой локации: вертикальный список ----------
	const cardW, cardH, gap = 210, 36, 8

	listH := len(stageDefs)*cardH + (len(stageDefs)-1)*gap
	cardsY := nameY + 44
	listX := screenWidth/2 - cardW/2 + 14 // сдвиг вправо — место под стрелки

	for i, def := range stageDefs {
		y := cardsY + i*(cardH+gap)
		sel := i == g.startStage
		border := color.RGBA{R: 90, G: 92, B: 105, A: 255}
		if sel {
			border = gold
		}
		// плашка: рамка, фон-«природа», полоска дороги, название
		vector.FillRect(screen, float32(listX), float32(y), cardW, cardH, border, false)
		vector.FillRect(screen, float32(listX+3), float32(y+3), cardW-6, cardH-6, def.Grass, false)
		vector.FillRect(screen, float32(listX+12), float32(y+3), 46, cardH-6, def.Asphalt, false)
		if def.Night > 0.5 {
			vector.FillRect(screen, float32(listX+12), float32(y+3), 46, cardH-6,
				color.RGBA{R: 6, G: 8, B: 24, A: 90}, false)
		} else if def.Night > 0.2 {
			vector.FillRect(screen, float32(listX+12), float32(y+3), 46, cardH-6,
				color.RGBA{R: 6, G: 8, B: 24, A: 45}, false)
		} else if def.Name == "HIGHWAY" {
			vector.FillRect(screen, float32(listX+12), float32(y+3), 4, cardH-6,
				color.RGBA{R: 165, G: 168, B: 175, A: 255}, false)
		} else if def.Name == "DESERT" {
			cact := color.RGBA{R: 58, G: 104, B: 54, A: 255}
			cx := float32(listX + 28)
			vector.FillRect(screen, cx-2, float32(y+cardH-14), 3, 9, cact, false)
			vector.FillRect(screen, cx-6, float32(y+cardH-11), 3, 5, cact, false)
			vector.FillRect(screen, cx+3, float32(y+cardH-12), 3, 5, cact, false)
		} else if def.Name == "BRIDGE" {
			conc := color.RGBA{R: 172, G: 174, B: 182, A: 255}
			vector.FillRect(screen, float32(listX+14), float32(y+3), 3, cardH-6, conc, false)
			vector.FillRect(screen, float32(listX+53), float32(y+3), 3, cardH-6, conc, false)
		}

		clr := color.RGBA{R: 200, G: 200, B: 210, A: 255}
		if sel {
			clr = gold
		}
		drawText(screen, def.Name, listX+68, y+8, 16, clr)
	}

	drawText(screen, "▲", listX-26, cardsY+listH/2-28, 18, white)
	drawText(screen, "▼", listX-26, cardsY+listH/2+4, 18, white)
	drawTextCentered(screen, "↑ ↓ — стартовая локация", cardsY+listH+12, 15, white)

	// ---------- подсказки ----------
	hintY := cardsY + listH + 44
	drawTextCentered(screen, "ENTER — гонка", hintY, 17, color.RGBA{R: 120, G: 255, B: 120, A: 255})
	drawTextCentered(screen, "TAB — сложность: "+diffNames[g.difficulty], hintY+30, 16, gold)
	drawTextCentered(screen, "W/S — газ и тормоз, ← → — руль", hintY+54, 14, grey)
	drawTextCentered(screen, "P — пауза, M — звук вкл/выкл", hintY+76, 14, grey)
	drawTextCentered(screen, fmt.Sprintf("Best: %d", g.highScore), hintY+112, 18,
		color.RGBA{R: 120, G: 220, B: 255, A: 255})
	drawTextCentered(screen, "ESC — выход", hintY+144, 14, grey)
}
