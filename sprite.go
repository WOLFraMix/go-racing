package main

import (
	"bytes"
	"image"
	"image/color"
	_ "image/png"
	"math"
	"os"
	"path/filepath"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/text/v2"
	"github.com/hajimehoshi/ebiten/v2/vector"
	"golang.org/x/image/font/gofont/goregular"
)

type Sprite struct {
	Img  *ebiten.Image
	Fill color.RGBA
}

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

func drawBlinker(dst *ebiten.Image, c Car, blink, frame int) {
	if blink == 0 || (frame/10)%2 != 0 {
		return
	}
	blinkCol := color.RGBA{R: 255, G: 150, B: 30, A: 255}
	lampW := float32(7)
	lampH := float32(10)
	var lampX float32
	if blink < 0 {
		lampX = float32(c.X) - lampW - 1
	} else {
		lampX = float32(c.X + c.W + 1)
	}
	frontY := float32(c.Y + c.H*0.12)
	rearY := float32(c.Y + c.H*0.82)
	vector.FillRect(dst, lampX, frontY, lampW, lampH, blinkCol, false)
	vector.FillRect(dst, lampX, rearY, lampW, lampH, blinkCol, false)
}

func drawBrakeLights(dst *ebiten.Image, c Car, braking bool) {
	dim := color.RGBA{R: 100, G: 10, B: 10, A: 255}
	hot := color.RGBA{R: 255, G: 30, B: 30, A: 255}

	w, h := 9.0, 7.0
	y := c.Y + c.H - h - 2
	lx := c.X + 2
	rx := c.X + c.W - w - 2

	lf, rf := float32(lx), float32(rx)
	yy, ww, hh := float32(y), float32(w), float32(h)
	vector.FillRect(dst, lf, yy, ww, hh, dim, false)
	vector.FillRect(dst, rf, yy, ww, hh, dim, false)

	if braking {
		bg := color.RGBA{R: 40, G: 0, B: 0, A: 255}
		vector.FillRect(dst, lf-1, yy-1, ww+2, hh+2, bg, false)
		vector.FillRect(dst, rf-1, yy-1, ww+2, hh+2, bg, false)
		vector.FillRect(dst, lf, yy, ww, hh, hot, false)
		vector.FillRect(dst, rf, yy, ww, hh, hot, false)
		coreW, coreH := w*0.4, h*0.4
		cx1 := lx + (w-coreW)/2
		cx2 := rx + (w-coreW)/2
		cy := y + (h-coreH)/2
		core := color.RGBA{R: 255, G: 170, B: 160, A: 255}
		vector.FillRect(dst, float32(cx1), float32(cy), float32(coreW), float32(coreH), core, false)
		vector.FillRect(dst, float32(cx2), float32(cy), float32(coreW), float32(coreH), core, false)
	}
}

// ---------- предрендеренный свет фар ----------

var beamImg *ebiten.Image

func clamp255(v float64) float64 {
	if v > 255 {
		return 255
	}
	if v < 0 {
		return 0
	}
	return v
}

func init() {
	const W, H = 160, 180
	src := image.NewRGBA(image.Rect(0, 0, W, H))
	cx := float64(W) / 2
	for y := 0; y < H; y++ {
		t := float64(H-1-y) / float64(H-1) // 0 у машины, 1 на дальнем конце
		fall := (1 - t) * (1 - t)          // свет тает к дальнему концу
		half := 0.28 + 0.52*t
		for x := 0; x < W; x++ {
			dx := math.Abs(float64(x)+0.5-cx) / (float64(W) / 2)
			f := (half - dx) / 0.30 // мягкий край вместо резкой границы
			if f > 1 {
				f = 1
			}
			if f < 0 {
				f = 0
			}
			intensity := fall * f
			if intensity <= 0.004 {
				continue
			}
			// премультиплированный цвет: RGB = цвет × альфа (тёплый белый)
			a := 140 * intensity
			src.SetRGBA(x, y, color.RGBA{
				R: uint8(clamp255(a)),
				G: uint8(clamp255(a * 0.96)),
				B: uint8(clamp255(a * 0.82)),
				A: uint8(clamp255(a)),
			})
		}
	}
	beamImg = ebiten.NewImageFromImage(src)
}

// drawHeadlights — два луча от левой и правой фар; рисуется ДО машин.
func drawHeadlights(dst *ebiten.Image, c Car) {
	if beamImg == nil {
		return
	}
	const length = 175.0
	w := c.W * 1.7 // ширина каждого конуса: сливаются в один поток, но видно две лампы
	for _, lx := range []float64{c.X + c.W*0.18, c.X + c.W*0.82} {
		op := &ebiten.DrawImageOptions{}
		op.GeoM.Scale(w/160, length/180)
		op.GeoM.Translate(lx-w/2, c.Y-length)
		dst.DrawImage(beamImg, op)
	}
}

// drawTailLights — два задних фонаря со свечением. Ночью горят всегда;
// при торможении поверх дорисовываются яркие стопы drawBrakeLights.
func drawTailLights(dst *ebiten.Image, c Car) {
	lampW, lampH := 9.0, 5.0
	y := c.Y + c.H - lampH - 2
	lamps := []float64{c.X + 3, c.X + c.W - lampW - 3}

	// очень сдержанное свечение позади каждого фонаря
	for _, lx := range lamps {
		cxp := lx + lampW/2
		vector.FillCircle(dst, float32(cxp), float32(y+lampH+1), 9,
			color.RGBA{R: 200, G: 40, B: 30, A: 10}, false)
		vector.FillCircle(dst, float32(cxp), float32(y+lampH+1), 4,
			color.RGBA{R: 220, G: 50, B: 35, A: 18}, false)
	}

	// сами фонари — насыщенные тёмно-красные плашки
	lamp := color.RGBA{R: 150, G: 26, B: 26, A: 210}
	for _, lx := range lamps {
		vector.FillRect(dst, float32(lx), float32(y), float32(lampW), float32(lampH), lamp, false)
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
	// тень для читаемости на любом фоне
	sh := &text.DrawOptions{}
	sh.GeoM.Translate(float64(x+2), float64(y+2))
	sh.ColorScale.ScaleWithColor(color.RGBA{R: 0, G: 0, B: 0, A: 180})
	text.Draw(dst, s, face(size), sh)

	op := &text.DrawOptions{}
	op.GeoM.Translate(float64(x), float64(y))
	op.ColorScale.ScaleWithColor(clr)
	text.Draw(dst, s, face(size), op)
}

func drawTextCentered(dst *ebiten.Image, s string, y int, size float64, clr color.Color) {
	sh := &text.DrawOptions{}
	sh.GeoM.Translate(float64(screenWidth/2+2), float64(y+2))
	sh.ColorScale.ScaleWithColor(color.RGBA{R: 0, G: 0, B: 0, A: 180})
	sh.PrimaryAlign = text.AlignCenter
	text.Draw(dst, s, face(size), sh)

	op := &text.DrawOptions{}
	op.GeoM.Translate(float64(screenWidth/2), float64(y))
	op.ColorScale.ScaleWithColor(clr)
	op.PrimaryAlign = text.AlignCenter
	text.Draw(dst, s, face(size), op)
}
