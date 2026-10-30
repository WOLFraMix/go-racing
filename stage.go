package main

import (
	"image/color"
)

// StageDef — описание этапа. Night: 0 — день, 1 — ночь.
type StageDef struct {
	Name        string
	Grass       color.RGBA
	Asphalt     color.RGBA
	Mark        color.RGBA
	Night       float64
	ScrollStart float64
	ScrollMax   float64
	Duration    float64 // длительность этапа, сек
}

var stageDefs = []StageDef{
	{Name: "CITY", Grass: color.RGBA{R: 56, G: 118, B: 58, A: 255}, Asphalt: color.RGBA{R: 72, G: 74, B: 84, A: 255}, Mark: color.RGBA{R: 225, G: 225, B: 230, A: 255}, Night: 0, ScrollStart: 270, ScrollMax: 520, Duration: 40},
	{Name: "DESERT", Grass: color.RGBA{R: 214, G: 190, B: 140, A: 255}, Asphalt: color.RGBA{R: 124, G: 112, B: 98, A: 255}, Mark: color.RGBA{R: 228, G: 214, B: 186, A: 255}, Night: 0, ScrollStart: 285, ScrollMax: 545, Duration: 40},
	{Name: "HIGHWAY", Grass: color.RGBA{R: 110, G: 128, B: 86, A: 255}, Asphalt: color.RGBA{R: 76, G: 78, B: 88, A: 255}, Mark: color.RGBA{R: 225, G: 225, B: 230, A: 255}, Night: 0.1, ScrollStart: 310, ScrollMax: 580, Duration: 40},
	{Name: "DUSK", Grass: color.RGBA{R: 62, G: 92, B: 64, A: 255}, Asphalt: color.RGBA{R: 62, G: 64, B: 82, A: 255}, Mark: color.RGBA{R: 205, G: 200, B: 190, A: 255}, Night: 0.5, ScrollStart: 300, ScrollMax: 560, Duration: 40},
	{Name: "NIGHT", Grass: color.RGBA{R: 14, G: 16, B: 30, A: 255}, Asphalt: color.RGBA{R: 30, G: 32, B: 48, A: 255}, Mark: color.RGBA{R: 150, G: 150, B: 165, A: 255}, Night: 1, ScrollStart: 330, ScrollMax: 600, Duration: 40},
	{Name: "DAWN", Grass: color.RGBA{R: 82, G: 108, B: 74, A: 255}, Asphalt: color.RGBA{R: 62, G: 64, B: 80, A: 255}, Mark: color.RGBA{R: 210, G: 205, B: 195, A: 255}, Night: 0.5, ScrollStart: 300, ScrollMax: 560, Duration: 40},
	{Name: "BRIDGE", Grass: color.RGBA{R: 58, G: 112, B: 152, A: 255}, Asphalt: color.RGBA{R: 104, G: 106, B: 116, A: 255}, Mark: color.RGBA{R: 238, G: 238, B: 242, A: 255}, Night: 0.1, ScrollStart: 290, ScrollMax: 540, Duration: 40},
}

// lerpColor — линейное смешение цветов, t от 0 (a) до 1 (b).
// Важно: простое uint8(v) усекает дробь, и при малых t канал
// «замирает», не дойдя до цели — сглаживание в updatePlaying
// вызывает lerpColor с шагом меньше единицы, и усечение съедало
// его целиком: светлые каналы навсегда оставались тёмными.
// Поэтому округляем до ближайшего и, если шаг не сдвинул канал,
// гарантированно шагаем на единицу к цели.
func lerpColor(a, b color.RGBA, t float64) color.RGBA {
	mix := func(x, y uint8) uint8 {
		if x == y || t <= 0 {
			return x
		}
		r := uint8(float64(x) + (float64(y)-float64(x))*t + 0.5)
		if r == x { // шаг меньше 0.5 — принудительный минимальный шаг
			if y > x {
				r = x + 1
			} else {
				r = x - 1
			}
		}
		return r
	}
	return color.RGBA{R: mix(a.R, b.R), G: mix(a.G, b.G), B: mix(a.B, b.B), A: 255}
}

// blendDefs — плавный переход между этапами: цвета и «темнота» тают,
// декорации переключаются на середине перехода.
func blendDefs(a, b StageDef, t float64) StageDef {
	name := a.Name
	if t >= 0.5 {
		name = b.Name
	}
	return StageDef{
		Name:        name,
		Grass:       lerpColor(a.Grass, b.Grass, t),
		Asphalt:     lerpColor(a.Asphalt, b.Asphalt, t),
		Mark:        lerpColor(a.Mark, b.Mark, t),
		Night:       a.Night + (b.Night-a.Night)*t,
		ScrollStart: a.ScrollStart + (b.ScrollStart-a.ScrollStart)*t,
		ScrollMax:   a.ScrollMax + (b.ScrollMax-a.ScrollMax)*t,
		Duration:    b.Duration,
	}
}

// stageTransition — общая длительность кроссфейда: половина течёт
// в конце старого этапа, половина — в начале нового.
const stageTransition = 10.0

func (g *Game) stageBlend() (int, int, float64) {
	n := len(stageDefs)
	cur := g.stageIdx
	half := stageTransition / 2

	if g.stageTime < half {
		// этап только сменился: дожимаем переход от предыдущего.
		return (cur + n - 1) % n, cur, 0.5 + 0.5*(g.stageTime/half)
	}
	remain := stageDefs[cur].Duration - g.stageTime
	if remain < half {
		return cur, (cur + 1) % n, 0.5 * (1 - remain/half)
	}
	return cur, cur, 0
}

// currentDef — этап с учётом кроссфейда на стыке.
func (g *Game) currentDef() StageDef {
	ai, bi, t := g.stageBlend()
	if ai == bi {
		return stageDefs[ai]
	}
	return blendDefs(stageDefs[ai], stageDefs[bi], t)
}
