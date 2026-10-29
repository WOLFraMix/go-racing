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
	engine []*audio.Player // 6 слоёв «двигателя» разной высоты тона

	crashBuf []byte

	lastGear   int
	shiftTimer float64

	rel, load float64 // сглаженные «обороты» и нагрузка
	time      float64
}

func NewAudio() *AudioSys {
	a := &AudioSys{ctx: audio.NewContext(sampleRate)}

	for _, f := range []float64{58, 74, 94, 120, 152, 194} {
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
	a.crashBuf = genCrash(sampleRate)
	return a
}

// UpdateEngine — обороты догоняют скорость с инерцией, нагрузка и тон
// плавно меняются от газа и торможения, передачи переключаются с провалом.
func (a *AudioSys) UpdateEngine(eff, boost, dt float64, braking, muted bool) {
	if len(a.engine) == 0 {
		return
	}
	f := 1.0
	if muted {
		f = 0
	}

	// целевые обороты от фактической скорости
	targetRel := (eff - 1.2) / 6.5
	if targetRel < 0 {
		targetRel = 0
	}
	if targetRel > 1 {
		targetRel = 1
	}
	// инерция: звук «набирает» и «сбрасывает» обороты с задержкой,
	// а не перескакивает мгновенно за скоростью
	a.rel += (targetRel - a.rel) * math.Min(1, 2.2*dt)

	// нагрузка: газ — громко, тормоз — тихо, переходы плавные
	targetLoad := 0.32
	if boost > 0.05 {
		targetLoad = 0.85
	}
	if braking {
		targetLoad = 0.12
	}
	a.load += (targetLoad - a.load) * math.Min(1, 6*dt)

	a.time += dt

	// передачи
	const gears = 4
	gearF := a.rel * (gears - 1)
	gear := int(gearF)
	frac := gearF - float64(gear)
	if gear != a.lastGear && gear > 0 && gear < gears {
		a.shiftTimer = 0.14
	}
	a.lastGear = gear
	if a.shiftTimer > 0 {
		a.shiftTimer -= dt
	}

	pos := a.rel * float64(len(a.engine)-1)
	for i, p := range a.engine {
		v := 1 - math.Abs(float64(i)-pos)
		if v < 0 {
			v = 0
		}
		// «дыхание» оборотов внутри передачи + лёгкая вибрация
		wobble := 0.82 + 0.30*frac +
			0.05*math.Sin(a.time*11.0) + 0.03*math.Sin(a.time*27.0)
		vol := (0.008 + 0.050*a.load*v) * f * wobble
		if a.shiftTimer > 0 {
			vol *= 0.25
		}
		p.SetVolume(vol)
	}
}

func (a *AudioSys) SilenceEngine() {
	for _, p := range a.engine {
		p.SetVolume(0)
	}
}

func (a *AudioSys) PlayCrash(vol float64, muted bool) {
	if muted {
		return
	}
	p, err := a.ctx.NewPlayer(bytes.NewReader(a.crashBuf))
	if err != nil {
		return
	}
	p.SetVolume(vol * 0.5)
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

// genCrash — мягкий удар: шумовой всплеск + низкочастотный гул.
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
