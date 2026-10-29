package main

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"math"
	"os"
	"path/filepath"
	"strings"
)

type skinDef struct {
	name   string
	hex    string
	stripe string
	box    bool
}

type kindDef struct {
	dir          string
	base         string
	repaintLight bool
	fixBumpers   bool // седан: ярко-красный бампер и декор красим, угловые огни сохраняем
	skins        []skinDef
}

// Зоны света седана — в долях размеров спрайта, по оригиналу и по скрину из
// игры (лучи фар идут от передних углов, стопы — у нижних углов).
// Если на готовых скинах огни сядут чуть не туда — крутятся только эти числа:
// sedanLightZone ниже — огни крупнее; sedanCornerKeep больше — огни шире.
const (
	sedanHeadZone   = 0.08 // выше этой доли высоты сидят кремовые фары
	sedanLightZone  = 0.90 // ниже — зона задних огней и бампера
	sedanCornerKeep = 0.20 // доля ширины с каждого края, остающаяся огнями
)

var kinds = []kindDef{
	{
		dir: "sedan", base: "assets/car_sedan.png", fixBumpers: true,
		skins: []skinDef{
			{name: "red", hex: "C42B2B"},
			{name: "white", hex: "E8E9EB"},
			{name: "silver", hex: "B9BEC6"},
			{name: "graphite", hex: "3A3D42", stripe: "D8DADD"},
			{name: "darkred", hex: "8C1F28"},
			{name: "navy", hex: "1F3A5F"},
			{name: "beige", hex: "C9B38C"},
			{name: "forest", hex: "3E5C43"},
			{name: "steelblue", hex: "4F6E8C"},
			{name: "taupe", hex: "8C8478"},
		},
	},
	{
		dir: "sport", base: "assets/car_sport.png",
		skins: []skinDef{
			{name: "rosso", hex: "C1272D", stripe: "F2F2F4"},
			{name: "orange", hex: "D97B29", stripe: "26282C"},
			{name: "white", hex: "F0F1F3", stripe: "26282C"},
			{name: "navy", hex: "20304E", stripe: "E4E5E7"},
			{name: "black", hex: "23252A", stripe: "E4E5E7"},
			{name: "silver", hex: "C9CCD0"},
		},
	},
	{
		dir: "suv", base: "assets/car_suv.png",
		skins: []skinDef{
			{name: "black", hex: "2B2E33"},
			{name: "white", hex: "EBECED"},
			{name: "grey", hex: "8B9096"},
			{name: "blue", hex: "2E4A6B", box: true},
			{name: "bronze", hex: "8A6A45", box: true},
			{name: "forest", hex: "39543F", box: true},
			{name: "maroon", hex: "5E2630"},
		},
	},
	{
		dir: "minibus", base: "assets/minibus.png", repaintLight: true,
		skins: []skinDef{
			{name: "white", hex: "F2F3F4"},
			{name: "grey", hex: "AEB2B8"},
			{name: "sand", hex: "D8C79A"},
			{name: "paleblue", hex: "9FB6C8"},
			{name: "palegreen", hex: "A8C0A8"},
			{name: "beige", hex: "CFC3A5"},
		},
	},
	{
		dir: "bus", base: "assets/bus.png",
		skins: []skinDef{
			{name: "orange", hex: "C4611E"},
			{name: "green", hex: "3E7A44"},
			{name: "blue", hex: "2C5C8A"},
			{name: "white", hex: "F0F1F2"},
			{name: "school", hex: "E8B820"},
		},
	},
	{
		dir: "truck", base: "assets/truck.png", repaintLight: true,
		skins: []skinDef{
			{name: "blue", hex: "37527A"},
			{name: "red", hex: "8C2B2B"},
			{name: "green", hex: "40603F"},
			{name: "orange", hex: "C87A28"},
		},
	},
}

func main() {
	for _, kd := range kinds {
		data, err := os.ReadFile(kd.base)
		if err != nil {
			fmt.Println("нет файла", kd.base, "— пропуск")
			continue
		}
		src, _, err := image.Decode(bytes.NewReader(data))
		if err != nil {
			panic(err)
		}

		outDir := filepath.Join("assets", "skins", kd.dir)
		if err := os.MkdirAll(outDir, 0o755); err != nil {
			panic(err)
		}
		if entries, err := os.ReadDir(outDir); err == nil {
			for _, en := range entries {
				if en.IsDir() || !strings.EqualFold(filepath.Ext(en.Name()), ".png") {
					continue
				}
				known := false
				for _, sk := range kd.skins {
					if en.Name() == sk.name+".png" {
						known = true
						break
					}
				}
				if !known {
					os.Remove(filepath.Join(outDir, en.Name()))
				}
			}
		}

		for _, sk := range kd.skins {
			img := repaint(src, hexColor(sk.hex), kd.repaintLight, kd.fixBumpers)
			if sk.stripe != "" {
				addStripe(img, hexColor(sk.stripe))
			}
			if sk.box {
				addBox(img)
			}
			path := filepath.Join(outDir, sk.name+".png")
			f, err := os.Create(path)
			if err != nil {
				panic(err)
			}
			if err := png.Encode(f, img); err != nil {
				panic(err)
			}
			f.Close()
			fmt.Println("OK", path)
		}
	}

	if err := os.MkdirAll("assets/skins/ambulance", 0o755); err != nil {
		panic(err)
	}
	data, _ := os.ReadFile("assets/ambulance.png")
	os.WriteFile("assets/skins/ambulance/ambulance.png", data, 0o644)
	fmt.Println("OK assets/skins/ambulance/ambulance.png")
}

func repaint(src image.Image, target color.RGBA, repaintLight, fixBumpers bool) *image.RGBA {
	b := src.Bounds()
	w, h := b.Dx(), b.Dy()
	dst := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			dst.Set(x, y, src.At(b.Min.X+x, b.Min.Y+y))
		}
	}
	tLum := lum(float64(target.R)/255, float64(target.G)/255, float64(target.B)/255)

	// эталон кузова: доминирующий тон по 20-градусным корзинам
	var bins [18]int
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			c := dst.RGBAAt(x, y)
			if c.A == 0 {
				continue
			}
			hh, s, v := rgbToHSV(float64(c.R)/255, float64(c.G)/255, float64(c.B)/255)
			if s > 0.15 && v > 0.15 && v < 0.92 {
				bins[int(math.Mod(hh+10, 360)/20)]++
			}
		}
	}
	dom := 0
	for i := 1; i < 18; i++ {
		if bins[i] > bins[dom] {
			dom = i
		}
	}
	center := float64(dom*20 + 10)

	var rs, gs, bs, n float64
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			c := dst.RGBAAt(x, y)
			if c.A == 0 {
				continue
			}
			r, g, bl := float64(c.R)/255, float64(c.G)/255, float64(c.B)/255
			hh, s, v := rgbToHSV(r, g, bl)
			if s > 0.15 && v > 0.15 && v < 0.92 && hueDiff(hh, center) < 35 {
				rs += r
				gs += g
				bs += bl
				n++
			}
		}
	}
	bodyLum := 0.3
	bodyHue := center
	if n > 0 {
		bodyHue, _, _ = rgbToHSV(rs/n, gs/n, bs/n)
		bodyLum = lum(rs/n, gs/n, bs/n)
	}

	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			c := dst.RGBAAt(x, y)
			if c.A == 0 {
				continue
			}
			r, g, bl := float64(c.R)/255, float64(c.G)/255, float64(c.B)/255
			a := c.A
			hh, s, v := rgbToHSV(r, g, bl)

			paint := false
			protected := false

			if fixBumpers {
				// фары: кремовые точки в передних углах — не красим никогда
				if float64(y) < sedanHeadZone*float64(h) && hh >= 15 && hh <= 80 && s < 0.5 && v > 0.85 {
					protected = true
				}
				// ярко-красное: у нижней кромки остаются только угловые
				// сегменты (огни), полоса бампера и весь декор — под кузов
				if isBrightRed(c) {
					inCorner := float64(x) < float64(w)*sedanCornerKeep ||
						float64(x) >= float64(w)*(1-sedanCornerKeep)
					if float64(y) >= sedanLightZone*float64(h) && inCorner {
						protected = true
					} else {
						paint = true
					}
				}
			} else {
				// прежние правила для остальных типов — они работают
				if float64(y) < float64(h)*0.16 && hh >= 15 && hh <= 80 && s < 0.5 && v > 0.85 {
					protected = true
				}
				if float64(y) > float64(h)*0.88 && (hh < 25 || hh > 335) && s > 0.5 && v > 0.55 {
					protected = true
				}
				if (hh < 25 || hh > 335) && s > 0.5 && v > 0.55 && hueDiff(hh, bodyHue) > 30 {
					protected = true
				}
			}

			if !protected {
				if hueDiff(hh, bodyHue) < 55 && s > 0.08 && v > 0.15 {
					paint = true
				}
				if repaintLight && s < 0.28 && v > 0.6 {
					paint = true
				}
			}
			// светлые цели: тёмное насыщенное (стёкла) не трогаем
			if paint && tLum > 0.55 && v < 0.35 && s > 0.3 {
				paint = false
			}
			// серая фурнитура и номер — не красим
			if paint && !repaintLight && s < 0.15 && v > 0.4 {
				paint = false
			}
			if !paint {
				continue
			}
			ratio := lum(r, g, bl) / bodyLum
			// яркие акценты не «выбеливаем» на светлых кузовах
			if fixBumpers && ratio > 1.25 {
				ratio = 1.25
			}
			dst.SetRGBA(x, y, color.RGBA{
				R: clamp255(float64(target.R) * ratio),
				G: clamp255(float64(target.G) * ratio),
				B: clamp255(float64(target.B) * ratio),
				A: a,
			})
		}
	}
	return dst
}

func addStripe(img *image.RGBA, c color.RGBA) {
	w, h := img.Bounds().Dx(), img.Bounds().Dy()
	sw := w * 6 / 100
	if sw < 3 {
		sw = 3
	}
	x0 := w/2 - sw*3/2
	y0, y1 := h*6/100, h*94/100
	fillRect(img, x0, y0, sw, y1-y0, c)
	fillRect(img, x0+2*sw, y0, sw, y1-y0, c)
}

func addBox(img *image.RGBA) {
	w, h := img.Bounds().Dx(), img.Bounds().Dy()
	x0, y0 := w*28/100, h*34/100
	x1, y1 := w*72/100, h*54/100
	fillRect(img, x0, y0, x1-x0, y1-y0, color.RGBA{R: 70, G: 74, B: 82, A: 255})
	fillRect(img, x0, y0, x1-x0, 3, color.RGBA{R: 110, G: 114, B: 124, A: 255})
}

func fillRect(img *image.RGBA, x, y, w, h int, c color.RGBA) {
	for j := y; j < y+h; j++ {
		for i := x; i < x+w; i++ {
			if i < 0 || j < 0 || i >= img.Bounds().Dx() || j >= img.Bounds().Dy() {
				continue
			}
			img.SetRGBA(i, j, c)
		}
	}
}

func hexColor(s string) color.RGBA {
	var r, g, b uint8
	fmt.Sscanf(strings.ToUpper(s), "%2X%2X%2X", &r, &g, &b)
	return color.RGBA{R: r, G: g, B: b, A: 255}
}

func lum(r, g, b float64) float64 { return 0.299*r + 0.587*g + 0.114*b }

func clamp255(v float64) uint8 { return uint8(math.Max(0, math.Min(255, v))) }

func hueDiff(a, b float64) float64 {
	d := math.Abs(a - b)
	if d > 180 {
		d = 360 - d
	}
	return d
}

func rgbToHSV(r, g, b float64) (h, s, v float64) {
	mx := math.Max(r, math.Max(g, b))
	mn := math.Min(r, math.Min(g, b))
	v = mx
	d := mx - mn
	if mx == 0 || d == 0 {
		return 0, 0, v
	}
	s = d / mx
	switch mx {
	case r:
		h = math.Mod((g-b)/d, 6)
	case g:
		h = (b-r)/d + 2
	default:
		h = (r-g)/d + 4
	}
	h *= 60
	if h < 0 {
		h += 360
	}
	return h, s, v
}

func isBrightRed(c color.RGBA) bool {
	if c.A == 0 {
		return false
	}
	hh, s, v := rgbToHSV(float64(c.R)/255, float64(c.G)/255, float64(c.B)/255)
	return (hh < 25 || hh > 335) && s > 0.4 && v > 0.55
}
