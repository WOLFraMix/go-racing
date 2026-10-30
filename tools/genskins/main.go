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
	fixBumpers   bool // седан: красим ярко-красный бампер и декор, фары и огни сохраняем по маске
	skins        []skinDef
}

var kinds = []kindDef{
	{
		dir: "sedan", base: "assets/car_sedan.png", fixBumpers: true,
		skins: []skinDef{
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

		var headMask, tailMask []bool
		if kd.fixBumpers {
			tmp := image.NewRGBA(image.Rect(0, 0, src.Bounds().Dx(), src.Bounds().Dy()))
			for y := 0; y < src.Bounds().Dy(); y++ {
				for x := 0; x < src.Bounds().Dx(); x++ {
					tmp.Set(x, y, src.At(src.Bounds().Min.X+x, src.Bounds().Min.Y+y))
				}
			}
			headMask, tailMask = lightMasks(tmp)
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
			img := repaint(src, hexColor(sk.hex), kd.repaintLight, kd.fixBumpers, headMask, tailMask)
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
}

func repaint(src image.Image, target color.RGBA, repaintLight, fixBumpers bool, headMask, tailMask []bool) *image.RGBA {
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
				idx := y*w + x
				if headMask[idx] || tailMask[idx] {
					protected = true // точные пиксели фар/огней с оригинала
				} else if isBrightRed(c) {
					paint = true // весь остальной ярко-красный — под кузов
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
			// серая фурнитура (бампер, хром) и номер — не красим
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

// lightMasks снимает маски точных пикселей фар и задних огней с оригинала.
// Фары: кремово-лососёвые пиксели в верхней зоне (вся пилюля целиком,
// включая теневую половину: s<0.65 отличает её от ярких огней).
// Огни: связные компоненты ярко-красного в нижней зоне шириной до трети
// спрайта — полоса бампера (широкая компонента) в маску не попадает.
func lightMasks(src *image.RGBA) (head, tail []bool) {
	w, h := src.Bounds().Dx(), src.Bounds().Dy()
	head = make([]bool, w*h)
	tail = make([]bool, w*h)
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			c := src.RGBAAt(x, y)
			if c.A == 0 {
				continue
			}
			hh, s, v := rgbToHSV(float64(c.R)/255, float64(c.G)/255, float64(c.B)/255)
			if float64(y) < float64(h)*0.18 && hh <= 90 && s < 0.65 && v > 0.6 {
				head[y*w+x] = true
			}
		}
	}
	// ярко-красные пиксели нижней зоны, разбитые на связные компоненты
	seen := make([]bool, w*h)
	for y := int(float64(h) * 0.75); y < h; y++ {
		for x := 0; x < w; x++ {
			idx := y*w + x
			if seen[idx] || !isBrightRed(src.RGBAAt(x, y)) {
				continue
			}
			// BFS по компоненте
			var comp []int
			stack := []int{idx}
			seen[idx] = true
			for len(stack) > 0 {
				cur := stack[len(stack)-1]
				stack = stack[:len(stack)-1]
				comp = append(comp, cur)
				cx, cy := cur%w, cur/w
				for _, d := range [4][2]int{{1, 0}, {-1, 0}, {0, 1}, {0, -1}} {
					nx, ny := cx+d[0], cy+d[1]
					if nx < 0 || ny < 0 || nx >= w || ny >= h || ny < int(float64(h)*0.75) {
						continue
					}
					ni := ny*w + nx
					if !seen[ni] && isBrightRed(src.RGBAAt(nx, ny)) {
						seen[ni] = true
						stack = append(stack, ni)
					}
				}
			}
			// ширина компоненты: компактный блок = огонь, лента во всю
			// ширину = бампер — его не защищаем
			minx, maxx := w, 0
			for _, i := range comp {
				cx := i % w
				if cx < minx {
					minx = cx
				}
				if cx > maxx {
					maxx = cx
				}
			}
			if maxx-minx+1 <= w*32/100 {
				for _, i := range comp {
					tail[i] = true
				}
			}
		}
	}
	return head, tail
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
