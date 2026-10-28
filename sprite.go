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

// drawBlinker — мигающие оранжевые указатели на боку машины:
// пара ламп спереди и сзади, вспышки с периодом 10 кадров.
func drawBlinker(dst *ebiten.Image, c Car, blink, frame int) {
	if blink == 0 || (frame/10)%2 != 0 {
		return // нет поворотника или фаза «выключено»
	}
	blinkCol := color.RGBA{R: 255, G: 150, B: 30, A: 255}

	// координата X лампы: чуть выступает за борт машины
	lampW := float32(7)
	lampH := float32(10)
	var lampX float32
	if blink < 0 {
		lampX = float32(c.X) - lampW - 1
	} else {
		lampX = float32(c.X + c.W + 1)
	}

	// передняя и задняя лампы
	frontY := float32(c.Y + c.H*0.12) // у переднего бампера
	rearY := float32(c.Y + c.H*0.82)  // у заднего бампера

	vector.FillRect(dst, lampX, frontY, lampW, lampH, blinkCol, false)
	vector.FillRect(dst, lampX, rearY, lampW, lampH, blinkCol, false)
}

// drawBrakeLights — красные стоп-сигналы у заднего бампера.
// Тусклые габариты горят всегда, яркие стопы — при торможении.
func drawBrakeLights(dst *ebiten.Image, c Car, braking bool) {
	dim := color.RGBA{R: 100, G: 10, B: 10, A: 255} // габариты
	hot := color.RGBA{R: 255, G: 30, B: 30, A: 255} // стопы: насыщенный красный

	w, h := 9.0, 7.0 // чуть крупнее, чем раньше (8×6)
	y := c.Y + c.H - h - 2
	lx := c.X + 2
	rx := c.X + c.W - w - 2

	// габариты
	lf, rf := float32(lx), float32(rx)
	yy, ww, hh := float32(y), float32(w), float32(h)
	vector.FillRect(dst, lf, yy, ww, hh, dim, false)
	vector.FillRect(dst, rf, yy, ww, hh, dim, false)

	if braking {
		// тёмная «подложка» — рамка, чтобы стопы читались на любом цвете кузова
		bg := color.RGBA{R: 40, G: 0, B: 0, A: 255}
		vector.FillRect(dst, lf-1, yy-1, ww+2, hh+2, bg, false)
		vector.FillRect(dst, rf-1, yy-1, ww+2, hh+2, bg, false)
		// яркие стопы
		vector.FillRect(dst, lf, yy, ww, hh, hot, false)
		vector.FillRect(dst, rf, yy, ww, hh, hot, false)
		// белое ядро в центре каждой лампы — эффект раскалённой нити
		coreW, coreH := w*0.4, h*0.4
		cx1 := lx + (w-coreW)/2
		cx2 := rx + (w-coreW)/2
		cy := y + (h-coreH)/2
		core := color.RGBA{R: 255, G: 170, B: 160, A: 255}
		vector.FillRect(dst, float32(cx1), float32(cy), float32(coreW), float32(coreH), core, false)
		vector.FillRect(dst, float32(cx2), float32(cy), float32(coreW), float32(coreH), core, false)
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
