package main

import (
	"bytes"
	"encoding/binary"
	"image/color"
	"math"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/audio"
	"github.com/hajimehoshi/ebiten/v2/inpututil"
	"github.com/hajimehoshi/ebiten/v2/vector"
)

const blinkPeriod = 0.55 // полный цикл вкл/выкл, ~110 миганий в минуту

// Зоны поворотников в координатах спрайта 106×152.
// Передние — внешние половины зон фар, задние — поверх красных стопов.
type Rect struct{ X, Y, W, H float32 }

var (
	blinkFrontL = Rect{7, 13, 13, 10}
	blinkFrontR = Rect{86, 13, 13, 10}
	blinkRearL  = Rect{8, 124, 18, 10}
	blinkRearR  = Rect{80, 124, 18, 10}
)

type TurnSignals struct {
	Left  bool
	Right bool
	Muted bool

	scale    float32
	audioCtx *audio.Context
	tickWav  []byte
	phase    float64
	on       bool
}

func NewTurnSignals(scale float32) *TurnSignals {
	t := &TurnSignals{scale: scale, tickWav: genTickWav(44100)}
	t.audioCtx = audio.CurrentContext()
	return t
}

// Reset — гасит оба поворотника и фазу мигания.
func (t *TurnSignals) Reset() {
	t.Left, t.Right = false, false
	t.phase, t.on = 0, false
}

func (t *TurnSignals) Update() {
	switch {
	case inpututil.IsKeyJustPressed(ebiten.KeyQ):
		t.Left = !t.Left
		t.playTick()
	case inpututil.IsKeyJustPressed(ebiten.KeyE):
		t.Right = !t.Right
		t.playTick()
	}

	if !t.Left && !t.Right {
		t.phase, t.on = 0, false
		return
	}

	t.phase += 1.0 / float64(ebiten.ActualTPS())
	if t.phase >= blinkPeriod {
		t.phase -= blinkPeriod
		t.on = !t.on
		t.playTick()
	}
}

func (t *TurnSignals) Draw(screen *ebiten.Image, carX, carY float32) {
	if (!t.Left && !t.Right) || !t.on {
		return
	}
	for _, side := range []struct {
		rect Rect
		on   bool
	}{
		{blinkFrontL, t.Left},
		{blinkFrontR, t.Right},
		{blinkRearL, t.Left},
		{blinkRearR, t.Right},
	} {
		if !side.on {
			continue
		}
		r := side.rect
		// мягкий ореол
		vector.FillRect(screen,
			carX+(r.X-2)*t.scale, carY+(r.Y-2)*t.scale,
			(r.W+4)*t.scale, (r.H+4)*t.scale,
			color.RGBA{R: 255, G: 150, B: 30, A: 60}, false)
		// само стекло поворотника
		vector.FillRect(screen,
			carX+r.X*t.scale, carY+r.Y*t.scale,
			r.W*t.scale, r.H*t.scale,
			color.RGBA{R: 255, G: 170, B: 40, A: 210}, false)
	}
}

func (t *TurnSignals) playTick() {
	if t.audioCtx == nil || t.Muted {
		return
	}
	p, err := t.audioCtx.NewPlayer(bytes.NewReader(t.tickWav))
	if err != nil {
		return
	}
	p.SetVolume(0.25)
	p.Play()
}

// Мягкий «тик-так» реле: низкий тёплый тон с плавной атакой и затуханием.
func genTickWav(rate int) []byte {
	const dur = 0.07 // чуть длиннее — звук «бархатнее»
	n := int(float64(rate) * dur)
	samples := make([]int16, n)
	for i := 0; i < n; i++ {
		ts := float64(i) / float64(rate)
		attack := math.Min(1, ts*300) // короткое плавное нарастание
		decay := math.Exp(-ts * 45)   // мягкий длинный спад
		env := attack * decay
		// один низкий тон с лёгкой призвуковой октавой — без резкого верха
		s := math.Sin(2*math.Pi*420*ts)*0.8 + math.Sin(2*math.Pi*840*ts)*0.12
		samples[i] = int16(s * env * 0.10 * 32767) // тихо: 0.10 вместо 0.18
	}
	return wavMono16(samples, rate)
}

func wavMono16(samples []int16, rate int) []byte {
	buf := new(bytes.Buffer)
	buf.WriteString("RIFF")
	binary.Write(buf, binary.LittleEndian, uint32(36+len(samples)*2))
	buf.WriteString("WAVEfmt ")
	binary.Write(buf, binary.LittleEndian, uint32(16))
	binary.Write(buf, binary.LittleEndian, uint16(1))
	binary.Write(buf, binary.LittleEndian, uint16(1))
	binary.Write(buf, binary.LittleEndian, uint32(rate))
	binary.Write(buf, binary.LittleEndian, uint32(rate*2))
	binary.Write(buf, binary.LittleEndian, uint16(2))
	binary.Write(buf, binary.LittleEndian, uint16(16))
	buf.WriteString("data")
	binary.Write(buf, binary.LittleEndian, uint32(len(samples)*2))
	for _, s := range samples {
		binary.Write(buf, binary.LittleEndian, s)
	}
	return buf.Bytes()
}
