package main

import (
	"image/color"
	"math"
	"math/rand"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/vector"
)

// weatherVersion — маркер версии погоды, рисуется внизу экрана.
const weatherVersion = "WX-17"

// ---------- виды погоды ----------

type weatherKind int

const (
	wNone weatherKind = iota
	wRain
	wFog
	wSnow
)

var weatherTitles = [...]string{"", "ДОЖДЬ", "ТУМАН", "СНЕГ"}

// ---------- частицы и декали ----------

type rainDrop struct{ x, y, spd, drift float64 }
type splash struct{ x, y, r, life float64 }

// spray — капля воды, отлетевшая назад из-под колеса.
type spray struct {
	x, y      float64
	vx, vy    float64
	life, max float64
	r         float64
}

// puddle — вытянутая лужа случайной формы: у каждой свои амплитуды
// трёх гармоник края, поэтому контуры все разные.
type puddle struct {
	x, y     float64
	r, rh    float64
	ph       float64
	amp1     float64 // медленная волна края
	amp2     float64 // средняя асимметрия
	amp3     float64 // мелкая рябь (бывает 0 — почти чистый эллипс)
	fq2, fq3 float64 // частоты гармоник — тоже случайные
	a        float64
}

type fogBlob struct {
	x, y     float64
	w, h     float64
	ph       float64
	vx       float64
	parallax float64
}

type wheelTrack struct {
	x, y, a float64
	wet     bool
}

type snowFlake struct{ x, y, spd, ph, r float64 }

type weatherState int

const (
	wStIdle weatherState = iota
	wStActive
	wStFadeOut
)

type WeatherSys struct {
	kind      weatherKind
	lastKind  weatherKind
	state     weatherState
	timer     float64
	intensity float64
	labelT    float64

	RoadLeft, RoadRight float64

	drops    []rainDrop
	splashes []splash
	sprays   []spray
	puddles  []puddle
	flakes   []snowFlake
	blobs    []fogBlob
	tracks   []wheelTrack

	puddleAcc float64
	sprayAcc  float64
	trackAcc  float64
	snowDepth float64
	fogT      float64
	scrollY   float64
}

func NewWeather() *WeatherSys {
	w := &WeatherSys{
		RoadLeft:  float64(roadX0 + 4),
		RoadRight: float64(roadX1 - 4),
	}
	for i := 0; i < 140; i++ {
		w.drops = append(w.drops, rainDrop{
			x: rand.Float64() * (screenWidth + 120), y: rand.Float64() * screenHeight,
			spd: 900 + rand.Float64()*350, drift: -60 - rand.Float64()*40,
		})
	}
	for i := 0; i < 110; i++ {
		w.flakes = append(w.flakes, snowFlake{
			x: rand.Float64() * screenWidth, y: rand.Float64() * screenHeight,
			spd: 75 + rand.Float64()*75, ph: rand.Float64() * math.Pi * 2,
			r: 1.2 + rand.Float64()*1.6,
		})
	}
	// 14 крупных клочьев тумана
	for i := 0; i < 14; i++ {
		w.blobs = append(w.blobs, fogBlob{
			x:        rand.Float64() * screenWidth,
			y:        rand.Float64() * (screenHeight + 300),
			w:        280 + rand.Float64()*220, // 280..500
			h:        90 + rand.Float64()*90,   // 90..180
			ph:       rand.Float64() * 6.28,
			vx:       (rand.Float64() - 0.5) * 16,
			parallax: 0.4 + rand.Float64()*0.35,
		})
	}
	w.timer = 10 + rand.Float64()*10
	return w
}

func (w *WeatherSys) Reset() {
	w.puddles = w.puddles[:0]
	w.splashes = w.splashes[:0]
	w.sprays = w.sprays[:0]
	w.tracks = w.tracks[:0]
	w.snowDepth = 0
	w.scrollY = 0
	for i := range w.blobs {
		w.blobs[i].y = rand.Float64() * (screenHeight + 300)
	}
	w.intensity, w.kind, w.state = 0, wNone, wStIdle
	w.timer = 8 + rand.Float64()*10
}

// Cycle — ручное переключение погоды по кругу: дождь → туман → снег → нет.
func (w *WeatherSys) Cycle() {
	next := map[weatherKind]weatherKind{
		wNone: wRain, wRain: wFog, wFog: wSnow, wSnow: wNone,
	}
	k := next[w.kind]
	if k == wNone {
		w.state = wStFadeOut
		return
	}
	w.kind, w.lastKind = k, k
	w.state = wStActive
	w.timer = 3600
	w.labelT = 2
}

func (w *WeatherSys) Update(dt, scrollDelta, carX, carY float64) {
	switch w.state {
	case wStIdle:
		w.timer -= dt
		if w.timer <= 0 {
			kinds := []weatherKind{wRain, wFog, wSnow}
			var k weatherKind
			for {
				k = kinds[rand.Intn(len(kinds))]
				if k != w.lastKind {
					break
				}
			}
			w.kind, w.lastKind = k, k
			w.state = wStActive
			w.timer = 14 + rand.Float64()*10
			w.labelT = 3
		}
	case wStActive:
		w.timer -= dt
		if w.timer <= 0 {
			w.state = wStFadeOut
		}
	}

	target, rate := 0.0, 1.0/2.5
	if w.state == wStActive {
		target = 1
	}
	if w.state == wStFadeOut {
		rate = 1.0 / 3.0
	}
	w.intensity += (target - w.intensity) * math.Min(1, rate*dt)
	if w.state == wStFadeOut && w.intensity < 0.02 {
		w.intensity, w.kind, w.state = 0, wNone, wStIdle
		w.timer = 20 + rand.Float64()*30
	}

	if w.labelT > 0 {
		w.labelT -= dt
	}
	w.fogT += dt
	w.scrollY += scrollDelta

	it := w.intensity

	// ---------- дождь ----------
	if w.kind == wRain && it > 0.02 {
		for i := range w.drops {
			d := &w.drops[i]
			d.y += d.spd * dt
			d.x += d.drift * dt
			if d.y > screenHeight+6 {
				d.y = -25 - rand.Float64()*50
				d.x = rand.Float64()*(screenWidth+120) - 60
				if rand.Float64() < 0.25 {
					w.splashes = append(w.splashes, splash{
						x: d.x, y: screenHeight - 4 - rand.Float64()*18,
						r: 1, life: 0.35,
					})
				}
			}
		}
		w.puddleAcc += dt * it
		if w.puddleAcc > 0.55 {
			w.puddleAcc = 0
			if len(w.puddles) < 18 {
				r := 14 + rand.Float64()*16
				p := puddle{
					x:  w.RoadLeft + r + 6 + rand.Float64()*(w.RoadRight-w.RoadLeft-2*r-12),
					y:  -110,
					r:  r,
					rh: r * (1.2 + rand.Float64()*1.6), // пропорции — от почти
					// круглой до сильно вытянутой
					ph:   rand.Float64() * 6.28,
					amp1: 0.03 + rand.Float64()*0.10,
					amp2: 0.02 + rand.Float64()*0.08,
					amp3: rand.Float64() * 0.05, // у части луж её почти нет
					fq2:  0.5 + rand.Float64()*0.5,
					fq3:  0.8 + rand.Float64()*0.9,
					a:    0.2,
				}
				w.puddles = append(w.puddles, p)
			}
		}
		// брызги из-под колёс: летят назад и вниз от задних колёс
		w.sprayAcc += dt * it
		for w.sprayAcc > 0.03 {
			w.sprayAcc -= 0.03
			for _, wx := range []float64{carX + carW*0.27, carX + carW*0.73} {
				if rand.Float64() < 0.6 {
					w.sprays = append(w.sprays, spray{
						x:    wx + (rand.Float64()-0.5)*6,
						y:    carY + carH - 12 + rand.Float64()*8,
						vx:   -30 - rand.Float64()*70, // назад и вбок
						vy:   60 + rand.Float64()*140, // вниз, за машину
						life: 0.25 + rand.Float64()*0.25,
						max:  0.5,
						r:    0.8 + rand.Float64()*1.2,
					})
				}
			}
		}
	}
	for i := 0; i < len(w.puddles); {
		p := &w.puddles[i]
		p.y += scrollDelta
		if w.kind == wRain {
			p.a = math.Min(1, p.a+dt*0.5)
		} else {
			p.a -= dt / 8
		}
		if p.y > screenHeight+140 || p.a <= 0 {
			w.puddles = append(w.puddles[:i], w.puddles[i+1:]...)
		} else {
			i++
		}
	}
	for i := 0; i < len(w.sprays); {
		s := &w.sprays[i]
		s.life -= dt
		s.x += s.vx * dt
		s.y += s.vy*dt + scrollDelta
		s.vy += 120 * dt
		if s.life <= 0 || s.y > screenHeight+20 {
			w.sprays = append(w.sprays[:i], w.sprays[i+1:]...)
		} else {
			i++
		}
	}

	// ---------- снежный покров ----------
	switch {
	case w.kind == wSnow && it > 0.02:
		w.snowDepth = math.Min(1, w.snowDepth+0.05*dt*it)
	default:
		w.snowDepth = math.Max(0, w.snowDepth-0.03*dt)
	}

	// ---------- снег: снежинки ----------
	if w.kind == wSnow && it > 0.02 {
		for i := range w.flakes {
			f := &w.flakes[i]
			f.y += (f.spd + scrollDelta*30) * dt
			f.ph += dt * 1.7
			f.x += math.Sin(f.ph) * 26 * dt
			if f.y > screenHeight+6 {
				f.y = -8 - rand.Float64()*20
				f.x = rand.Float64() * screenWidth
			}
		}
	}

	// ---------- следы колёс ----------
	if (w.kind == wRain || w.kind == wSnow) && it > 0.4 {
		w.trackAcc += dt
		if w.trackAcc > 0.085 {
			w.trackAcc = 0
			jit := (rand.Float64() - 0.5) * 2.5
			wet := w.kind == wRain
			w.tracks = append(w.tracks,
				wheelTrack{x: carX + carW*0.27 + jit, y: carY + carH - 10, a: 1, wet: wet},
				wheelTrack{x: carX + carW*0.73 + jit, y: carY + carH - 10, a: 1, wet: wet})
		}
	}
	for i := 0; i < len(w.tracks); {
		t := &w.tracks[i]
		t.y += scrollDelta
		if t.wet {
			t.a -= dt / 2.6
		} else {
			t.a -= dt / 6
		}
		if t.y > screenHeight+40 || t.a <= 0 {
			w.tracks = append(w.tracks[:i], w.tracks[i+1:]...)
		} else {
			i++
		}
	}
	if len(w.tracks) > 220 {
		w.tracks = w.tracks[len(w.tracks)-220:]
	}

	// ---------- туман ----------
	if w.kind == wFog && it > 0.02 {
		for i := range w.blobs {
			b := &w.blobs[i]
			b.y += scrollDelta*b.parallax - 5*dt
			b.x += b.vx * dt
			b.ph += dt * 0.3
			if b.y < -280 {
				b.y = screenHeight + 270
				b.x = rand.Float64() * screenWidth
			}
			if b.y > screenHeight+290 {
				b.y = -270
				b.x = rand.Float64() * screenWidth
			}
			if b.x < -560 {
				b.x = screenWidth + 550
			}
			if b.x > screenWidth+560 {
				b.x = -550
			}
		}
	}

	// ---------- всплески ----------
	for i := 0; i < len(w.splashes); {
		s := &w.splashes[i]
		s.life -= dt
		s.r += dt * 40
		s.y += scrollDelta
		if s.life <= 0 {
			w.splashes = append(w.splashes[:i], w.splashes[i+1:]...)
		} else {
			i++
		}
	}
}

// ---------- отрисовка ----------

// fillStrips — фигура из горизонтальных полосок с ПОСТОЯННОЙ плотностью;
// мягкость даёт только сужение формы к краям.
func fillStrips(scr *ebiten.Image, cx, cy, halfH, step float64,
	profile func(dy float64) float64, a float64, clr color.NRGBA) {
	c := clr
	c.A = uint8(clamp(a, 0, 1) * 255)
	for dy := -halfH; dy <= halfH; dy += step {
		hw := profile(dy)
		if hw <= 0.5 {
			continue
		}
		vector.FillRect(scr, float32(cx-hw), float32(cy+dy),
			float32(hw*2), float32(step), c, false)
	}
}

// puddleShape — вытянутый эллипс с тремя гармониками случайной силы:
// формы у луж разные, но края остаются скруглёнными и размытыми.
func (w *WeatherSys) puddleShape(scr *ebiten.Image, p *puddle,
	cx, cy, r, rh, a float64, clr color.NRGBA) {
	fillStrips(scr, cx, cy, rh, 3, func(dy float64) float64 {
		q := 1 - (dy*dy)/(rh*rh)
		if q <= 0.008 {
			return 0
		}
		hw := r * math.Sqrt(q)
		hw *= 1 + p.amp1*math.Sin(p.ph+dy*0.35)
		hw *= 1 + p.amp2*math.Sin(p.ph*p.fq2*4+dy*0.7)
		hw *= 1 + p.amp3*math.Sin(p.ph*p.fq3*7+dy*1.6)
		hw *= math.Pow(q, 0.3) // размытие к торцам
		hw = math.Min(hw, math.Min(cx-w.RoadLeft, w.RoadRight-cx))
		return hw
	}, a, clr)
}

// drawPuddle — ореол влажности, тело и светлое «зеркало» воды.
func (w *WeatherSys) drawPuddle(scr *ebiten.Image, p *puddle) {
	wet := color.NRGBA{R: 30, G: 38, B: 62, A: 255}
	sky := color.NRGBA{R: 116, G: 128, B: 158, A: 255}

	// ореол влажности: размывает границу с асфальтом
	w.puddleShape(scr, p, p.x, p.y, p.r*1.25, p.rh*1.15, 0.16*p.a, wet)
	// тело лужи
	w.puddleShape(scr, p, p.x, p.y, p.r, p.rh, 0.35*p.a, wet)
	// зеркало воды внутри
	w.puddleShape(scr, p, p.x, p.y, p.r*0.68, p.rh*0.74, 0.50*p.a, sky)
}

// drawSpray — капля брызг: маленькая светлая точка.
func (w *WeatherSys) drawSpray(scr *ebiten.Image, s *spray) {
	a := clamp(s.life/s.max, 0, 1) * 0.55
	c := color.NRGBA{R: 205, G: 220, B: 244, A: uint8(a * 255)}
	sz := s.r
	vector.FillRect(scr, float32(s.x-sz), float32(s.y-sz),
		float32(sz*2), float32(sz*2), c, false)
}

// drawTrack — след колеса: мягкая вытянутая полоска.
func (w *WeatherSys) drawTrack(scr *ebiten.Image, t *wheelTrack) {
	clr := color.NRGBA{R: 30, G: 40, B: 64, A: 255}
	aa := 0.13 * t.a
	if !t.wet {
		clr = color.NRGBA{R: 242, G: 246, B: 252, A: 255}
		aa = 0.30 * t.a
	}
	fillStrips(scr, t.x, t.y, 13, 2, func(dy float64) float64 {
		q := 1 - (dy*dy)/(13*13)
		if q <= 0.05 {
			return 0
		}
		return 3.5 * math.Pow(q, 0.3)
	}, aa, clr)
}

// drawSnowCover — покров: плотный белый слой, сугробы у краёв и полосы
// вдоль линий разметки. Плотность поднята — снег заметно белее.
func (w *WeatherSys) drawSnowCover(scr *ebiten.Image, d float64) {
	if d <= 0.01 {
		return
	}
	vector.FillRect(scr, roadX0, 0, roadX1-roadX0, screenHeight,
		color.NRGBA{R: 243, G: 247, B: 253, A: uint8(75 * d)}, false)

	bank := color.NRGBA{R: 247, G: 250, B: 254, A: uint8(clamp(0.85*d, 0, 1) * 255)}
	wisp := color.NRGBA{R: 245, G: 249, B: 254, A: uint8(clamp(0.34*d, 0, 1) * 255)}
	const step = 5.0
	for y := 0.0; y < screenHeight+step; y += step {
		wy := y - w.scrollY // мировая координата — едет вместе с дорогой

		// сугробы у краёв дороги
		wob := 3.0*math.Sin(wy*0.06) + 1.8*math.Sin(wy*0.17)
		bw := 8 + 14*d + wob
		vector.FillRect(scr, float32(roadX0), float32(y), float32(bw), step, bank, false)
		vector.FillRect(scr, float32(roadX1-bw), float32(y), float32(bw), step, bank, false)

		// лёгкие снежные полосы вдоль линий разметки
		for l := 1; l < laneCount; l++ {
			x := float64(roadX0 + laneWidth*l)
			ph := float64(l) * 1.9
			ww := (1.5 + 4.0*d) *
				(1 + 0.4*math.Sin(wy*0.045+ph) + 0.2*math.Sin(wy*0.13+ph*2))
			if ww < 0.6 {
				continue
			}
			vector.FillRect(scr, float32(x-ww), float32(y),
				float32(ww*2), step, wisp, false)
		}
	}
}

// DrawUnder — слой ПОД машинами: покров, следы колёс и лужи.
func (w *WeatherSys) DrawUnder(screen *ebiten.Image) {
	w.drawSnowCover(screen, w.snowDepth)
	for i := range w.tracks {
		w.drawTrack(screen, &w.tracks[i])
	}
	for i := range w.puddles {
		w.drawPuddle(screen, &w.puddles[i])
	}
}

// Draw — слой ПОВЕРХ сцены: осадки и дымка.
func (w *WeatherSys) Draw(screen *ebiten.Image) {
	it := w.intensity
	if it <= 0.005 {
		return
	}
	switch w.kind {
	case wRain:
		w.drawRain(screen, it)
	case wFog:
		w.drawFog(screen, it)
	case wSnow:
		w.drawSnow(screen, it)
	}
	if w.labelT > 0 && w.kind != wNone {
		if w.labelT > 1 || int(w.labelT*6)%2 == 0 {
			drawTextCentered(screen, weatherTitles[w.kind], 96, 18,
				color.NRGBA{R: 255, G: 210, B: 60, A: 255})
		}
	}
	drawText(screen, weatherVersion, screenWidth-56, screenHeight-26, 12,
		color.NRGBA{R: 255, G: 200, B: 40, A: 255})
}

func (w *WeatherSys) drawRain(scr *ebiten.Image, it float64) {
	vector.FillRect(scr, 0, 0, screenWidth, screenHeight,
		color.NRGBA{R: 18, G: 24, B: 42, A: uint8(45 * it)}, false)

	n := int(float64(len(w.drops)) * it)
	dropC := color.NRGBA{R: 190, G: 210, B: 235, A: 110}
	for i := 0; i < n; i++ {
		d := &w.drops[i]
		vector.StrokeLine(scr,
			float32(d.x), float32(d.y),
			float32(d.x+d.drift*0.018), float32(d.y+16), 1.4, dropC, false)
	}

	for i := range w.splashes {
		s := &w.splashes[i]
		decay := s.life / 0.35
		clr := color.NRGBA{R: 205, G: 220, B: 244, A: 255}
		fillStrips(scr, s.x, s.y, s.r, 2, func(dy float64) float64 {
			q := 1 - (dy*dy)/(s.r*s.r)
			if q <= 0.05 {
				return 0
			}
			return s.r * math.Sqrt(q)
		}, 0.3*decay, clr)
	}

	// брызги из-под колёс
	for i := range w.sprays {
		w.drawSpray(scr, &w.sprays[i])
	}
}

// drawFog — дымка: ровная вуаль ~19% + очень крупные мягкие клочья.
func (w *WeatherSys) drawFog(scr *ebiten.Image, it float64) {
	vector.FillRect(scr, 0, 0, screenWidth, screenHeight,
		color.NRGBA{R: 209, G: 215, B: 227, A: uint8(48 * it)}, false)

	fogC := color.NRGBA{R: 211, G: 217, B: 229, A: 255}
	for i := range w.blobs {
		b := &w.blobs[i]
		peak := 0.10 * it
		fillStrips(scr, b.x, b.y, b.h, 5, func(dy float64) float64 {
			q := 1 - (dy*dy)/(b.h*b.h)
			if q <= 0.01 {
				return 0
			}
			hw := b.w * math.Pow(q, 0.55)
			hw *= 1 + 0.2*math.Sin(b.ph+dy*0.04)
			return hw
		}, peak, fogC)
		fillStrips(scr, b.x+b.w*0.45, b.y+b.h*0.35, b.h*0.6, 5,
			func(dy float64) float64 {
				hh := b.h * 0.6
				q := 1 - (dy*dy)/(hh*hh)
				if q <= 0.01 {
					return 0
				}
				hw := b.w * 0.55 * math.Pow(q, 0.55)
				hw *= 1 + 0.15*math.Sin(b.ph+2.1+dy*0.07)
				return hw
			}, peak*0.75, fogC)
		fillStrips(scr, b.x-b.w*0.4, b.y+b.h*0.5, b.h*0.5, 5,
			func(dy float64) float64 {
				hh := b.h * 0.5
				q := 1 - (dy*dy)/(hh*hh)
				if q <= 0.01 {
					return 0
				}
				hw := b.w * 0.4 * math.Pow(q, 0.55)
				hw *= 1 + 0.15*math.Sin(b.ph*1.6+1.0+dy*0.09)
				return hw
			}, peak*0.55, fogC)
	}
}

// drawSnow — падающие снежинки, теперь непрозрачные белые точки.
func (w *WeatherSys) drawSnow(scr *ebiten.Image, it float64) {
	n := int(float64(len(w.flakes)) * it)
	flakeC := color.NRGBA{R: 252, G: 253, B: 255, A: 255}
	for i := 0; i < n; i++ {
		f := &w.flakes[i]
		s := f.r
		vector.FillRect(scr, float32(f.x-s), float32(f.y-s),
			float32(s*2), float32(s*2), flakeC, false)
	}
}
