package main

import (
	"bytes"
	"image"
	"image/color"
	_ "image/png"
	"os"
	"path/filepath"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/text/v2"
	"github.com/hajimehoshi/ebiten/v2/vector"
	"golang.org/x/image/font/gofont/goregular"
)

type Sprite struct {
	Img  *ebiten.Image
	Fill color.RGBA // цвет-заглушка, если PNG нет
}

// LoadSprite читает assets/<name>; если файла нет — остаётся заглушка-цвет.
func LoadSprite(name string, fallback color.RGBA) *Sprite {
	s := &Sprite{Fill: fallback}
	data, err := os.ReadFile(filepath.Join("assets", name))
	if err != nil {
		return s
	}
	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return s
	}
	s.Img = ebiten.NewImageFromImage(img)
	return s
}

func drawCar(dst *ebiten.Image, c Car, s *Sprite) {
	if s.Img != nil {
		op := &ebiten.DrawImageOptions{}
		bw, bh := float64(s.Img.Bounds().Dx()), float64(s.Img.Bounds().Dy())
		if bw > 0 && bh > 0 {
			op.GeoM.Scale(c.W/bw, c.H/bh)
		}
		op.GeoM.Translate(c.X, c.Y)
		dst.DrawImage(s.Img, op)
		return
	}
	vector.FillRect(dst, float32(c.X), float32(c.Y), float32(c.W), float32(c.H), s.Fill, false)
}

// drawBlinker — мигающий оранжевый указатель на боку машины.
func drawBlinker(dst *ebiten.Image, c Car, blink, frame int) {
	if blink == 0 || (frame/12)%2 != 0 {
		return // нет поворотника или фаза «выключено»
	}
	blinkCol := color.RGBA{R: 255, G: 150, B: 30, A: 255}
	midY := float32(c.Y + c.H/2)
	if blink < 0 {
		vector.FillRect(dst, float32(c.X-9), midY-6, 7, 12, blinkCol, false)
	} else {
		vector.FillRect(dst, float32(c.X+c.W+2), midY-6, 7, 12, blinkCol, false)
	}
}

// ---------- текст ----------

var faceSource *text.GoTextFaceSource

func init() {
	s, err := text.NewGoTextFaceSource(bytes.NewReader(goregular.TTF))
	if err != nil {
		panic(err)
	}
	faceSource = s
}

func face(size float64) *text.GoTextFace {
	return &text.GoTextFace{Source: faceSource, Size: size}
}

func drawText(dst *ebiten.Image, s string, x, y int, size float64, clr color.Color) {
	op := &text.DrawOptions{}
	op.GeoM.Translate(float64(x), float64(y))
	op.ColorScale.ScaleWithColor(clr)
	text.Draw(dst, s, face(size), op)
}

func drawTextCentered(dst *ebiten.Image, s string, y int, size float64, clr color.Color) {
	op := &text.DrawOptions{}
	op.GeoM.Translate(float64(screenWidth/2), float64(y))
	op.ColorScale.ScaleWithColor(clr)
	op.PrimaryAlign = text.AlignCenter
	text.Draw(dst, s, face(size), op)
}
