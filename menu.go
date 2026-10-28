package main

import (
	"fmt"
	"image/color"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/inpututil"
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

func (g *Game) updateMenu() error {
	if inpututil.IsKeyJustPressed(ebiten.KeyEscape) {
		g.saveSettings()
		return ebiten.Termination
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyM) {
		g.muted = !g.muted
		g.saveSettings()
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyLeft) {
		g.skinIdx = (g.skinIdx + len(g.playerSkins) - 1) % len(g.playerSkins)
		g.saveSettings()
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyRight) {
		g.skinIdx = (g.skinIdx + 1) % len(g.playerSkins)
		g.saveSettings()
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyEnter) || inpututil.IsKeyJustPressed(ebiten.KeySpace) {
		g.reset()
		g.state = StatePlaying
	}
	return nil
}

func (g *Game) drawMenu(screen *ebiten.Image) {
	vector.FillRect(screen, 0, 0, screenWidth, screenHeight, color.RGBA{R: 22, G: 24, B: 32, A: 255}, false)
	drawTextCentered(screen, "GO RACING", 90, 44, color.RGBA{R: 255, G: 210, B: 60, A: 255})
	drawTextCentered(screen, "MINI", 142, 30, color.RGBA{R: 255, G: 210, B: 60, A: 255})

	// превью выбранной машины ×2
	s := g.playerSkins[g.skinIdx]
	if s.Img != nil {
		op := &ebiten.DrawImageOptions{}
		op.GeoM.Scale(2, 2)
		op.GeoM.Translate(float64(screenWidth/2-s.Img.Bounds().Dx()), 200)
		screen.DrawImage(s.Img, op)
	} else {
		vector.FillRect(screen, float32(screenWidth/2-53), 200, 106, 152, s.Fill, false)
	}

	white := color.White
	grey := color.RGBA{R: 160, G: 160, B: 170, A: 255}
	drawTextCentered(screen, "<  "+g.skinNames[g.skinIdx]+"  >", 530, 22, white)
	drawText(screen, "◀", 40, 330, 26, white)
	drawText(screen, "▶", screenWidth-60, 330, 26, white)

	drawTextCentered(screen, "← →  — выбрать машину", 580, 16, white)
	drawTextCentered(screen, "ENTER — гонка", 610, 16, color.RGBA{R: 120, G: 255, B: 120, A: 255})
	drawTextCentered(screen, "W/S — газ и тормоз, ←→ — руль", 640, 14, grey)
	drawTextCentered(screen, "P — пауза, M — звук", 664, 14, grey)
	drawTextCentered(screen, fmt.Sprintf("Best: %d", g.highScore), 710, 18, color.RGBA{R: 120, G: 220, B: 255, A: 255})
	drawTextCentered(screen, "ESC — выход", 750, 14, grey)
}
