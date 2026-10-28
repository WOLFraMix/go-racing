package main

import (
	"bytes"
	"math"
	"math/rand"

	"github.com/hajimehoshi/ebiten/v2/audio"
)

// sampleRate — частота аудиоконтекста Ebitengine всегда 44100.
const sampleRate = 44100

// AudioSys — синтез звука без аудиофайлов.
type AudioSys struct {
	ctx    *audio.Context
	engine []*audio.Player // 4 слоя «двигателя» с разной высотой тона

	squealBuf []byte
	crashBuf  []byte
	tickBuf   []byte
	tick      *audio.Player
}

func NewAudio() *AudioSys {
	a := &AudioSys{ctx: audio.NewContext(sampleRate)}

	for _, f := range []float64{55, 78, 102, 130} {
		buf := genEngineLoop(f, sampleRate)
		loop := audio.NewInfiniteLoop(bytes.NewReader(buf), int64(len(buf)))
		p, err := a.ctx.NewPlayer(loop)
		if err != nil {
			continue
		}
		p.SetVolume(0)
		p.Play()
		a.engine = append(a.engine, p)
	}
	a.squealBuf = genSqueal(sampleRate)
	a.crashBuf = genCrash(sampleRate)
	a.tickBuf = genTick(sampleRate)
	a.tick, _ = a.ctx.NewPlayer(bytes.NewReader(a.tickBuf))
	return a
}

// UpdateEngine — громкость слоёв зависит от скорости: слои перекрываются,
// и тон двигателя непрерывно «растёт» с разгоном.
func (a *AudioSys) UpdateEngine(eff float64, muted bool) {
	if len(a.engine) == 0 {
		return
	}
	f := 1.0
	if muted {
		f = 0
	}
	ratio := (eff - 1.2) / 7.2
	if ratio < 0 {
		ratio = 0
	}
	if ratio > 1 {
		ratio = 1
	}
	pos := ratio * float64(len(a.engine)-1)
	for i, p := range a.engine {
		v := 1 - math.Abs(float64(i)-pos)
		if v < 0 {
			v = 0
		}
		p.SetVolume((0.02 + 0.10*v) * f)
	}
}

func (a *AudioSys) SilenceEngine() {
	for _, p := range a.engine {
		p.SetVolume(0)
	}
}

func (a *AudioSys) PlayTick(vol float64, muted bool) {
	if muted || a.tick == nil {
		return
	}
	_ = a.tick.Rewind()
	a.tick.SetVolume(vol)
	a.tick.Play()
}

func (a *AudioSys) PlaySqueal(vol float64, muted bool) {
	if muted {
		return
	}
	p, err := a.ctx.NewPlayer(bytes.NewReader(a.squealBuf))
	if err != nil {
		return
	}
	p.SetVolume(vol)
	p.Play()
}

func (a *AudioSys) PlayCrash(vol float64, muted bool) {
	if muted {
		return
	}
	p, err := a.ctx.NewPlayer(bytes.NewReader(a.crashBuf))
	if err != nil {
		return
	}
	p.SetVolume(vol)
	p.Play()
}

// ---------- генерация PCM ----------

// toPCM — float [-1..1] → 16-bit LE стерео (Ebitengine требует 2 канала).
func toPCM(samples []float64) []byte {
	buf := make([]byte, len(samples)*4)
	for i, s := range samples {
		if s > 1 {
			s = 1
		} else if s < -1 {
			s = -1
		}
		v := int16(s * 32000)
		buf[i*4] = byte(v)
		buf[i*4+1] = byte(v >> 8)
		buf[i*4+2] = byte(v)
		buf[i*4+3] = byte(v >> 8)
	}
	return buf
}

// genEngineLoop — ровно целое число периодов на буфер → бесшовный луп.
func genEngineLoop(freq float64, sr int) []byte {
	const dur = 1.0
	cycles := int(freq * dur)
	n := int(float64(sr) * dur)
	s := make([]float64, n)
	for i := 0; i < n; i++ {
		t := float64(i) / float64(sr)
		ph := 2 * math.Pi * float64(cycles) * t
		v := math.Sin(ph) + 0.5*math.Sin(2*ph) + 0.25*math.Sin(3*ph)
		rough := 0.75 + 0.25*math.Sin(2*math.Pi*28*t)
		noiseWin := math.Pow(math.Sin(math.Pi*t), 0.25)
		v += 0.12 * noiseWin * (rand.Float64()*2 - 1)
		s[i] = v * rough * 0.4
	}
	return toPCM(s)
}

// genSqueal — визг шин: ниспадающий вой с вибрато и шумом.
func genSqueal(sr int) []byte {
	n := sr / 2
	s := make([]float64, n)
	ph := 0.0
	for i := 0; i < n; i++ {
		t := float64(i) / float64(sr)
		f := (1500 - 600*t) * (1 + 0.02*math.Sin(2*math.Pi*11*t))
		ph += 2 * math.Pi * f / float64(sr)
		env := math.Min(1, t*40) * math.Exp(-3*t)
		s[i] = (0.7*math.Sin(ph) + 0.3*(rand.Float64()*2-1)) * env * 0.5
	}
	return toPCM(s)
}

// genCrash — удар: шумовой всплеск + низкочастотный гул с затуханием.
func genCrash(sr int) []byte {
	n := sr * 4 / 5
	s := make([]float64, n)
	low := 0.0
	for i := 0; i < n; i++ {
		t := float64(i) / float64(sr)
		w := rand.Float64()*2 - 1
		low = low*0.96 + w*0.04
		env := math.Exp(-4.5*t) + 2*math.Exp(-40*t)
		s[i] = (0.8*w + 2.5*low) * env * 0.6
	}
	return toPCM(s)
}

// genTick — короткий «тик» поворотника.
func genTick(sr int) []byte {
	n := sr * 40 / 1000
	s := make([]float64, n)
	for i := 0; i < n; i++ {
		t := float64(i) / float64(sr)
		s[i] = math.Sin(2*math.Pi*1100*t) * math.Exp(-t*120) * 0.8
	}
	return toPCM(s)
}
