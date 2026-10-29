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
}

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

	// ---------- превью машины (×1.4) ----------
	s := g.playerSkins[g.skinIdx]
	const scale = 1.4
	pw, ph := 106.0*scale, 152.0*scale
	px := float64(screenWidth)/2 - pw/2
	py := 160.0
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

	// ---------- выбор стартовой локации ----------
	const cardW, cardH, gap = 95, 66, 10
	totalW := len(stageDefs)*cardW + (len(stageDefs)-1)*gap
	startX := (screenWidth - totalW) / 2
	cardsY := nameY + 40

	for i, def := range stageDefs {
		x := startX + i*(cardW+gap)
		sel := i == g.startStage
		border := color.RGBA{R: 90, G: 92, B: 105, A: 255}
		if sel {
			border = gold
		}
		vector.FillRect(screen, float32(x), float32(cardsY), cardW, cardH, border, false)
		vector.FillRect(screen, float32(x+3), float32(cardsY+3), cardW-6, cardH-6, def.Grass, false)
		vector.FillRect(screen, float32(x+12), float32(cardsY+3), cardW-24, cardH-6, def.Asphalt, false)
		if def.Night {
			vector.FillRect(screen, float32(x+12), float32(cardsY+3), cardW-24, cardH-6,
				color.RGBA{R: 6, G: 8, B: 24, A: 90}, false)
		} else if def.Name == "HIGHWAY" {
			vector.FillRect(screen, float32(x+12), float32(cardsY+3), 4, cardH-6,
				color.RGBA{R: 165, G: 168, B: 175, A: 255}, false)
		}
		// название ПОД своей карточкой, по центру карточки (а не экрана)
		clr := color.RGBA{R: 200, G: 200, B: 210, A: 255}
		if sel {
			clr = gold
		}
		drawTextCenteredX(screen, def.Name, x+cardW/2, cardsY+cardH+8, 13, clr)
	}

	// стрелки выбора локации — слева от ряда карточек, вертикальной парой
	drawText(screen, "▲", 24, cardsY+cardH/2-26, 18, white)
	drawText(screen, "▼", 24, cardsY+cardH/2+4, 18, white)
	drawTextCentered(screen, "↑ ↓ — стартовая локация", cardsY+cardH+34, 15, white)

	// ---------- подсказки ----------
	hintY := cardsY + cardH + 66
	drawTextCentered(screen, "ENTER — гонка", hintY, 17, color.RGBA{R: 120, G: 255, B: 120, A: 255})
	drawTextCentered(screen, "W/S — газ и тормоз, ← → — руль", hintY+30, 14, grey)
	drawTextCentered(screen, "P — пауза, M — звук вкл/выкл", hintY+52, 14, grey)
	drawTextCentered(screen, fmt.Sprintf("Best: %d", g.highScore), hintY+88, 18,
		color.RGBA{R: 120, G: 220, B: 255, A: 255})
	drawTextCentered(screen, "ESC — выход", hintY+120, 14, grey)
}
