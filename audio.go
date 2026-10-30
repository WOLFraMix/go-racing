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
	musicBuf []byte

	lastGear   int
	shiftTimer float64

	rel, load float64 // сглаженные «обороты» и нагрузка
	time      float64

	hornBuf []byte

	siren *audio.Player

	music *audio.Player
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
	buf := genSiren(sampleRate)
	loop := audio.NewInfiniteLoop(bytes.NewReader(buf), int64(len(buf)))
	if sp, err := a.ctx.NewPlayer(loop); err == nil {
		sp.SetVolume(0)
		sp.Play()
		a.siren = sp
		a.musicBuf = genMusic(sampleRate)
		buf := a.musicBuf
		loop := audio.NewInfiniteLoop(bytes.NewReader(buf), int64(len(buf)))
		if mp, err := a.ctx.NewPlayer(loop); err == nil {
			mp.SetVolume(0.25)
			mp.Play()
			a.music = mp
		}
		a.hornBuf = genHorn(sampleRate)
	}
	return a
}

// SetMusic — фоновая музыка играет всегда, mute только глушит её.
func (a *AudioSys) SetMusic(muted bool) {
	if a.music == nil {
		return
	}
	if muted {
		a.music.SetVolume(0)
	} else {
		a.music.SetVolume(0.25)
	}
}

// SetSiren — сирена играется, только если включена и не замьючена.
func (a *AudioSys) SetSiren(on, muted bool) {
	if a.siren == nil {
		return
	}
	if on && !muted {
		a.siren.SetVolume(0.10)
	} else {
		a.siren.SetVolume(0)
	}
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

// PlayHorn — двойное «би-би»: два тона ~370 Гц по 0.12 с с паузой.
// Одноразовый буфер, как у crash; отдельный плеер на каждое нажатие.
func (a *AudioSys) PlayHorn(muted bool) {
	if muted {
		return
	}
	p, err := a.ctx.NewPlayer(bytes.NewReader(a.hornBuf))
	if err != nil {
		return
	}
	p.SetVolume(0.06)
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
	p.SetVolume(vol * 0.05)
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

// genMusic — лёгкий synthwave-луп, всё синтезировано с нуля.
// Прогрессия Am–F–C–G: пэд из мягких синусов, восьмые баса,
// арпеджио шестнадцатыми с эхом и деликатная перкуссия.
// Все тона квантованы на целое число периодов на длину лупа —
// стык бесшовный, как у engine-лупов.
func genMusic(sr int) []byte {
	const bpm = 100.0
	beat := 60.0 / bpm // 0.6 с
	bar := 4 * beat    // 2.4 с — один аккорд
	dur := 4 * bar     // 9.6 с — весь луп
	n := int(dur * float64(sr))
	s := make([]float64, n)

	// квантование частоты: периодов на луп должно быть целое число
	qfreq := func(f float64) float64 { return math.Round(f*dur) / dur }
	note := func(midi float64) float64 {
		return qfreq(440 * math.Pow(2, (midi-69)/12))
	}

	chords := [][]float64{
		{57, 60, 64}, // Am: A3 C4 E4
		{53, 57, 60}, // F:  F3 A3 C4
		{48, 52, 55}, // C:  C3 E3 G3
		{55, 59, 62}, // G:  G3 B3 D4
	}
	bassRoot := []float64{33, 29, 24, 31} // A1 F1 C1 G1
	arpOrder := [4]int{0, 1, 2, 1}        // фигурация арпеджио

	padPh := [3]float64{}
	padPh2 := [3]float64{}
	var bassPh, kickPh float64
	var prevW float64

	for i := 0; i < n; i++ {
		t := float64(i) / float64(sr)
		ci := int(t/bar) % 4
		bi := int(t / beat)        // номер доли
		bt := math.Mod(t, beat)    // время внутри доли
		chordT := math.Mod(t, bar) // время внутри аккорда

		v := 0.0

		// пэд: два слегка расстроенных синуса на ноту, перекрёстное затухание
		pEnv := math.Min(1, chordT/0.4) * math.Min(1, (bar-chordT)/0.4) * 0.10
		for ni, m := range chords[ci] {
			f := note(m)
			p := padPh[ni]
			padPh[ni] += 2 * math.Pi * f / float64(sr)
			p2 := padPh2[ni]
			padPh2[ni] += 2 * math.Pi * qfreq(f*1.004) / float64(sr)
			v += pEnv * (math.Sin(p) + 0.6*math.Sin(p2))
		}

		// бас: восьмые с драйвовым паттерном «1 … 4» внутри доли
		ei := int(t / (beat / 2)) // номер восьмой
		if ei%4 == 0 || ei%4 == 3 {
			slotT := math.Mod(t, beat/2)
			bl := beat / 2 * 0.8
			bg := clamp01(slotT/0.005) * clamp01((bl-slotT)/0.05)
			f := note(bassRoot[ci])
			p := bassPh
			bassPh += 2 * math.Pi * f / float64(sr)
			v += 0.15 * bg * (math.Sin(p) + 0.3*math.Sin(2*p))
		}

		// арпеджио: шестнадцатые, октава прыгает каждый такт фигурации
		asi := int(t / (beat / 4)) // номер шестнадцатой
		slotT := math.Mod(t, beat/4)
		aEnv := clamp01(slotT/0.004) * clamp01(((beat/4)*0.85-slotT)/0.04)
		m := chords[ci][arpOrder[asi%4]] + 12
		if (asi/4)%2 == 1 {
			m += 12
		}
		fa := 440 * math.Pow(2, (m-69)/12)
		var phA float64
		for k := 0; k < asi%4+1; k++ { // фаза от начала ноты
			phA += 2 * math.Pi * fa / float64(sr)
		}
		v += 0.065 * aEnv * (math.Sin(phA) + 0.15*math.Sin(2*phA))
		_ = phA

		// бочка на каждую долю: синус со скольжением высоты вниз
		if bt < 0.2 {
			env := math.Exp(-bt / 0.05)
			fk := 80.0 - 175.0*bt
			p := kickPh
			kickPh += 2 * math.Pi * fk / float64(sr)
			v += 0.28 * env * math.Sin(p)
		}
		// снейр на 2-ю и 4-ю доли: шум + короткий тон
		if bi%2 == 1 && bt < 0.16 {
			env := math.Exp(-bt / 0.045)
			w := rand.Float64()*2 - 1
			v += 0.10 * env * (0.7*w + 0.4*math.Sin(2*math.Pi*190*t))
		}
		// хэты на восьмые: «осветлённый» шум (разность соседних сэмплов)
		ht := math.Mod(t, beat/2)
		if ht < 0.035 {
			env := math.Exp(-ht / 0.012)
			w := rand.Float64()*2 - 1
			bright := w - prevW*0.8
			prevW = w
			v += 0.05 * env * bright
		}

		s[i] = v * 0.5
	}

	// лёгкое эхо на всё: копия с задержкой 3/8 доли, завёрнутая по кругу —
	// за счёт этого стык лупа остаётся бесшовным и у хвоста эха
	d := int(float64(sr) * beat * 0.375)
	orig := make([]float64, n)
	copy(orig, s)
	for i := 0; i < n; i++ {
		s[i] += 0.22 * orig[(i-d+n)%n]
	}
	return toPCM(s)
}

// genHorn — один длинный «БИИИИИП»: чистый тон с мягким подъёмом
// и спадом. Один тон, а не аккорд — аккорд двух близких частот
// даёт биения и дребезжит, нам нужен ровный сигнал.
func genHorn(sr int) []byte {
	const (
		f       = 440.0 // тон гудка, Гц
		beep    = 0.45  // длина сигнала, с
		attack  = 0.05  // мягкий подъём, с
		release = 0.10  // мягкий спад, с
	)
	n := int(float64(sr) * beep)
	s := make([]float64, n)
	ph := 0.0
	w := 2 * math.Pi * f / float64(sr)
	for i := 0; i < n; i++ {
		t := float64(i) / float64(sr)
		ph += w

		// плавный вход и выход — никакого щелчка на краях
		env := clamp01(t/attack) * clamp01((beep-t)/release)

		// почти чистый тон, лишь капля второй гармоники для тела
		v := math.Sin(ph) + 0.08*math.Sin(2*ph)
		v = math.Tanh(0.8 * v)
		s[i] = v * env * 0.42
	}
	return toPCM(s)
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

// genSiren — «вау»-сирена: тон плавно завывает между 620 и 850 Гц.
// Целое число периодов ЛЧМ на длину буфера → бесшовный луп.
func genSiren(sr int) []byte {
	const T = 2.0 // период одного завывания, сек
	n := int(float64(sr) * T)
	s := make([]float64, n)
	ph := 0.0
	for i := 0; i < n; i++ {
		t := float64(i) / float64(sr)
		f := 620.0 + 230.0*math.Sin(2*math.Pi*t/T)
		ph += 2 * math.Pi * f / float64(sr) // накопленная фаза — иначе свип «щёлкнет» на стыке
		v := 0.55*math.Sin(ph) + 0.25*math.Sin(2*ph)
		env := 0.85 + 0.15*math.Sin(2*math.Pi*t/T+math.Pi) // чуть громче на пике тона
		s[i] = v * env * 0.35
	}
	return toPCM(s)
}
