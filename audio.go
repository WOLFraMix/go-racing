package main

import (
	"bytes"
	"math"
	"math/rand"

	"github.com/hajimehoshi/ebiten/v2/audio"
)

// sampleRate — частота аудиоконтекста Ebitengine всегда 44100.
const sampleRate = 44100

const musicVol = 0.25

var trackNames = [...]string{"SYNTHWAVE", "CHILL", "RUSH", "RETRO", "LOFI"}

// AudioSys — синтез звука без аудиофайлов.
type AudioSys struct {
	ctx    *audio.Context
	engine []*audio.Player // 6 слоёв «двигателя» разной высоты тона

	crashBuf []byte

	lastGear   int
	shiftTimer float64

	rel, load float64 // сглаженные «обороты» и нагрузка
	time      float64

	hornBuf []byte

	siren *audio.Player

	music *audio.Player // активная дорожка

	tracks    []*audio.Player
	musicBufs [][]byte
	trackIdx  int
	muted     bool
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
		a.hornBuf = genHorn(sampleRate)

		a.musicBufs = [][]byte{
			genMusic(sampleRate),
			genMusicChill(sampleRate),
			genMusicDrive(sampleRate),
			genMusicRetro(sampleRate),
			genMusicLofi(sampleRate),
		}

		for i, mb := range a.musicBufs {
			loop := audio.NewInfiniteLoop(bytes.NewReader(mb), int64(len(mb)))
			mp, err := a.ctx.NewPlayer(loop)
			if err != nil {
				continue
			}
			mp.SetVolume(0)
			if i == a.trackIdx {
				mp.SetVolume(musicVol)
				mp.Play()
			}
			a.tracks = append(a.tracks, mp)
		}
		if len(a.tracks) > 0 {
			a.music = a.tracks[0]
		}
	}
	return a
}

// SetMusic — играет активная дорожка, mute только глушит её.
func (a *AudioSys) SetMusic(muted bool) {
	a.muted = muted
	if a.music == nil {
		return
	}
	if muted {
		a.music.SetVolume(0)
	} else {
		a.music.SetVolume(musicVol)
	}
}

// SetTrack — переключение на дорожку idx (0..2) с сохранением мьюта.
func (a *AudioSys) SetTrack(idx int, muted bool) {
	a.muted = muted
	if idx < 0 || idx >= len(trackNames) || idx >= len(a.tracks) {
		return
	}
	if idx == a.trackIdx {
		return
	}
	if a.trackIdx < len(a.tracks) {
		a.tracks[a.trackIdx].SetVolume(0)
	}
	a.trackIdx = idx
	a.music = a.tracks[idx]
	if muted {
		a.music.SetVolume(0)
	} else {
		a.music.SetVolume(musicVol)
	}
	a.music.Play()
}

// Track — номер активной дорожки для HUD.
func (a *AudioSys) Track() int { return a.trackIdx }

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

// genMusicChill — расслабленный луп: тёплый пэд, мягкий бас,
// деликатная перкуссия, редкие «пианинные» ноты. Fmaj7–Am7–Dm7–G7.
func genMusicChill(sr int) []byte {
	const bpm = 82.0
	beat := 60.0 / bpm
	bar := 4 * beat
	dur := 4 * bar
	n := int(dur * float64(sr))
	s := make([]float64, n)

	qfreq := func(f float64) float64 { return math.Round(f*dur) / dur }
	note := func(m float64) float64 {
		return qfreq(440 * math.Pow(2, (m-69)/12))
	}

	chords := [][]float64{
		{53, 57, 60, 64}, // Fmaj7
		{57, 60, 64, 67}, // Am7
		{50, 53, 57, 60}, // Dm7
		{55, 59, 62, 65}, // G7
	}
	bassRoot := []float64{29, 33, 26, 31}

	padPh := [4]float64{}
	padPh2 := [4]float64{}
	var bassPh, kickPh float64
	var prevW float64

	for i := 0; i < n; i++ {
		t := float64(i) / float64(sr)
		ci := int(t/bar) % 4
		bi := int(t / beat)
		bt := math.Mod(t, beat)
		chordT := math.Mod(t, bar)

		v := 0.0

		// тёплый пэд: медленная атака, две расстроенные волны
		pEnv := math.Min(1, chordT/0.8) * math.Min(1, (bar-chordT)/0.8) * 0.12
		for ni, m := range chords[ci] {
			f := note(m)
			padPh[ni] += 2 * math.Pi * f / float64(sr)
			padPh2[ni] += 2 * math.Pi * qfreq(f*1.003) / float64(sr)
			v += pEnv * (math.Sin(padPh[ni]) + 0.5*math.Sin(padPh2[ni]))
		}

		// мягкий длинный бас на 1-ю и 3-ю доли
		if bi%2 == 0 {
			env := clamp01(bt/0.015) * clamp01((beat*0.9-bt)/0.2)
			p := bassPh
			bassPh += 2 * math.Pi * note(bassRoot[ci]) / float64(sr)
			v += 0.14 * env * (math.Sin(p) + 0.25*math.Sin(2*p))
		}

		// редкие «пианинные» ноты на сильные доли
		if bi%2 == 0 {
			m := chords[ci][(bi/2)%len(chords[ci])] + 12
			env := math.Exp(-bt / 0.35)
			fq := note(m)
			ph := 2 * math.Pi * fq * bt
			v += 0.07 * env * (math.Sin(ph) + 0.2*math.Sin(2*ph))
		}

		// мягкая бочка
		if bt < 0.25 {
			env := math.Exp(-bt / 0.08)
			fk := 100.0 - 60.0*bt
			kickPh += 2 * math.Pi * fk / float64(sr)
			v += 0.20 * env * math.Sin(kickPh)
		}

		// «щёточные» хэты на восьмые — тихо и мягко
		ht := math.Mod(t, beat/2)
		if ht < 0.04 {
			env := math.Exp(-ht / 0.018)
			w := rand.Float64()*2 - 1
			bright := w - prevW*0.85
			prevW = w
			v += 0.028 * env * bright
		}

		s[i] = v * 0.5
	}

	// круговое эхо — стык остаётся бесшовным
	d := int(float64(sr) * beat * 0.5)
	orig := make([]float64, n)
	copy(orig, s)
	for i := 0; i < n; i++ {
		s[i] += 0.20 * orig[(i-d+n)%n]
	}
	return toPCM(s)
}

// genMusicDrive — энергичный луп: бочка на каждую долю, драйвовый бас
// с сатурацией, оффбит-стабы и лид-рифф. Am–G–F–E.
func genMusicDrive(sr int) []byte {
	const bpm = 132.0
	beat := 60.0 / bpm
	bar := 4 * beat
	dur := 4 * bar
	n := int(dur * float64(sr))
	s := make([]float64, n)

	qfreq := func(f float64) float64 { return math.Round(f*dur) / dur }
	note := func(m float64) float64 {
		return qfreq(440 * math.Pow(2, (m-69)/12))
	}

	chords := [][]float64{
		{57, 60, 64}, // Am
		{55, 59, 62}, // G
		{53, 57, 60}, // F
		{52, 56, 59}, // E
	}
	bassRoot := []float64{33, 31, 29, 28}

	var bassPh, kickPh, leadPh float64
	var prevW float64
	lastEi := -1
	leadF := note(57 + 12)

	for i := 0; i < n; i++ {
		t := float64(i) / float64(sr)
		ci := int(t/bar) % 4
		bt := math.Mod(t, beat)
		ei := int(t / (beat / 2)) // номер восьмой
		et := math.Mod(t, beat/2)

		v := 0.0

		// бочка на каждую долю — плотная, с сатурацией
		if bt < 0.2 {
			env := math.Exp(-bt / 0.045)
			fk := 105.0 - 60.0*bt
			kickPh += 2 * math.Pi * fk / float64(sr)
			v += 0.30 * env * math.Tanh(1.4*math.Sin(kickPh))
		}

		// драйвовый бас восьмыми с паттерном «1 … 3-4»
		if ei%4 == 0 || ei%4 == 2 || ei%4 == 3 {
			env := clamp01(et/0.004) * clamp01((beat/2*0.85-et)/0.05)
			p := bassPh
			bassPh += 2 * math.Pi * note(bassRoot[ci]) / float64(sr)
			w := math.Tanh(1.2 * (math.Sin(p) + 0.4*math.Sin(2*p) + 0.2*math.Sin(3*p)))
			v += 0.16 * env * w
		}

		// оффбит-стабы аккорда — «двигатель» грува
		if et < beat/4 {
			env := math.Exp(-et / 0.06)
			for _, m := range chords[ci] {
				ph := 2 * math.Pi * note(m) * et
				v += 0.05 * env * (math.Sin(ph) + 0.3*math.Sin(2*ph))
			}
		}

		// лид-рифф восьмыми на чётных тактах, октава прыгает
		if ei != lastEi {
			lastEi = ei
			step := []float64{0, 2, 1, 2, 0, 2, 1, 2}[ei%8]
			m := chords[ci][int(step)] + 12
			if (ei/4)%2 == 1 {
				m += 12
			}
			leadF = note(m)
		}
		lEnv := clamp01(et/0.004) * clamp01((beat/2*0.8-et)/0.06)
		leadPh += 2 * math.Pi * leadF / float64(sr)
		w := math.Tanh(0.9 * (math.Sin(leadPh) + 0.25*math.Sin(2*leadPh)))
		v += 0.07 * lEnv * w

		// хэты: открытый на оффбите, тики на шестнадцатые
		ht := math.Mod(t, beat/2)
		if ht < 0.03 {
			env := math.Exp(-ht / 0.01)
			w2 := rand.Float64()*2 - 1
			bright := w2 - prevW*0.8
			prevW = w2
			v += 0.055 * env * bright
		}

		s[i] = v * 0.5
	}

	d := int(float64(sr) * beat * 0.375)
	orig := make([]float64, n)
	copy(orig, s)
	for i := 0; i < n; i++ {
		s[i] += 0.18 * orig[(i-d+n)%n]
	}
	return toPCM(s)
}

// genMusicRetro — сухой ретро-поп 80-х: плотный ритм-секшен,
// октавный бас, короткие аккордные стабы и простой лид без эха.
// Am–F–G–Am, всё квантовано — стык лупа бесшовный.
func genMusicRetro(sr int) []byte {
	const bpm = 122.0
	beat := 60.0 / bpm
	bar := 4 * beat
	dur := 4 * bar
	n := int(dur * float64(sr))
	s := make([]float64, n)

	qfreq := func(f float64) float64 { return math.Round(f*dur) / dur }
	note := func(m float64) float64 {
		return qfreq(440 * math.Pow(2, (m-69)/12))
	}

	chords := [][]float64{
		{57, 60, 64}, // Am
		{53, 57, 60}, // F
		{55, 59, 62}, // G
		{57, 60, 64}, // Am
	}
	bassRoot := []float64{33, 29, 31, 33}

	var bassPh, kickPh float64
	var prevW float64
	lastEi := -1
	leadF := note(69)
	leadPh := 0.0

	for i := 0; i < n; i++ {
		t := float64(i) / float64(sr)
		ci := int(t/bar) % 4
		bt := math.Mod(t, beat)
		ei := int(t / (beat / 2))
		et := math.Mod(t, beat/2)

		v := 0.0

		// бас октавами восьмыми — сухой, с лёгким драйвом
		env := clamp01(et/0.004) * clamp01((beat/2*0.9-et)/0.04)
		p := bassPh
		bassPh += 2 * math.Pi * note(bassRoot[ci]) / float64(sr)
		w := math.Tanh(1.2*math.Sin(p) + 0.35*math.Sin(2*p))
		if (ei/2)%2 == 1 {
			w += 0.35 * math.Sin(p*2) // октава вверх на «ан»-долях
		}
		v += 0.17 * env * w

		// бочка: пухлый, плотный, на каждую долю
		if bt < 0.22 {
			kickPh += 2 * math.Pi * (95.0 - 55.0*bt) / float64(sr)
			v += 0.28 * math.Exp(-bt/0.06) * math.Sin(kickPh)
		}

		// снейр на 2 и 4: короткий сухой щелчок без гейта и хвостов
		if ei%4 == 2 && et < 0.14 {
			w2 := rand.Float64()*2 - 1
			bright := w2 - prevW*0.6
			prevW = w2
			v += 0.13 * math.Exp(-et/0.03) * (bright + 0.4*math.Sin(2*math.Pi*200*t))
		}

		// хэты на восьмые: короткие, негромкие
		if et < 0.03 {
			w2 := rand.Float64()*2 - 1
			bright := w2 - prevW*0.8
			prevW = w2
			v += 0.035 * math.Exp(-et/0.012) * bright
		}

		// аккордные стабы: короткие «туд-туд» на 1-ю и «ан» второй восьмой
		if et < beat/4 {
			env2 := math.Exp(-et / 0.05)
			for _, m := range chords[ci] {
				ph := 2 * math.Pi * note(m) * et
				v += 0.055 * env2 * (math.Sin(ph) + 0.3*math.Sin(2*ph))
			}
		}

		// лид: простая фигурация восьмыми, без вибрато и верхних прыжков
		if ei != lastEi {
			lastEi = ei
			seq := []int{0, 2, 1, 0, 2, 1, 0, 1}
			m := chords[ci][seq[ei%len(seq)]] + 12
			leadF = note(m)
		}
		leadPh += 2 * math.Pi * leadF / float64(sr)
		lv := math.Tanh(0.8 * (math.Sin(leadPh) + 0.2*math.Sin(2*leadPh)))
		v += 0.07 * clamp01(et/0.005) * clamp01((beat/2*0.75-et)/0.05) * lv

		s[i] = v * 0.5
	}

	// только короткое эхо на пол-восьмой — чуть объёма, без «далека»
	d := int(float64(sr) * beat * 0.125)
	orig := make([]float64, n)
	copy(orig, s)
	for i := 0; i < n; i++ {
		s[i] += 0.10 * orig[(i-d+n)%n]
	}
	return toPCM(s)
}

// genMusicLofi — лоу-фай: виниловый шум, мягкий джазовый прогрессинг,
// раскачка тайминга, «пыльные» барабаны. Am9–Dm9–G7–Cmaj7.
func genMusicLofi(sr int) []byte {
	const bpm = 74.0
	beat := 60.0 / bpm
	bar := 4 * beat
	dur := 4 * bar
	n := int(dur * float64(sr))
	s := make([]float64, n)

	qfreq := func(f float64) float64 { return math.Round(f*dur) / dur }
	note := func(m float64) float64 {
		return qfreq(440 * math.Pow(2, (m-69)/12))
	}

	chords := [][]float64{
		{57, 60, 64, 67}, // Am9-подобное: A C E G
		{50, 53, 57, 60}, // Dm7
		{55, 59, 62, 65}, // G7
		{48, 52, 55, 59}, // Cmaj7
	}
	bassRoot := []float64{33, 26, 31, 24}

	padPh := [4]float64{}
	var bassPh, kickPh float64
	var prevW float64

	// виниловый шелест: медленный шум с редкими щелчками
	noise := make([]float64, n)
	last := 0.0
	for i := 0; i < n; i++ {
		w := rand.Float64()*2 - 1
		last = last*0.995 + w*0.005
		noise[i] = last
		if rand.Float64() < 1.0/float64(sr)*22 { // ~22 щелчка в секунду
			noise[i] += (rand.Float64()*2 - 1) * 0.35
		}
	}

	for i := 0; i < n; i++ {
		t := float64(i) / float64(sr)
		ci := int(t/bar) % 4
		bi := int(t / beat)
		bt := math.Mod(t, beat)
		chordT := math.Mod(t, bar)

		// лоу-фай раскачка: время чуть «гуляет» swing'ом
		swing := 0.02 * math.Sin(2*math.Pi*float64(bi%2)/2.0)
		tt := t + swing

		v := 0.0

		// мягкий пэд, почти без атак, тёплые верха срезаны простой фильтрацией
		pEnv := math.Min(1, chordT/0.6) * math.Min(1, (bar-chordT)/0.6) * 0.10
		for ni, m := range chords[ci] {
			p := padPh[ni]
			padPh[ni] += 2 * math.Pi * note(m) / float64(sr)
			v += pEnv * (math.Sin(p) + 0.15*math.Sin(2*p))
		}

		// ленивый бас: нота на 1 и «и» третьей доли
		if bi == 0 || bi == 2 {
			env := clamp01(bt/0.02) * clamp01((beat*1.1-bt)/0.3)
			p := bassPh
			bassPh += 2 * math.Pi * note(bassRoot[ci]) / float64(sr)
			v += 0.13 * env * (math.Tanh(0.9*math.Sin(p)) + 0.2*math.Sin(2*p))
		}

		// «пыльная» бочка на 1 и 3
		if bi%2 == 0 && bt < 0.3 {
			kickPh += 2 * math.Pi * (90.0 - 40.0*bt) / float64(sr)
			v += 0.22 * math.Exp(-bt/0.09) * math.Sin(kickPh)
		}

		// снейр: мягкий, с шумом и низким телом
		if bi%2 == 1 && bt < 0.2 {
			env := math.Exp(-bt / 0.06)
			w := rand.Float64()*2 - 1
			v += 0.07 * env * (0.8*w + 0.5*math.Sin(2*math.Pi*170*tt))
		}

		// редкие хэты: только на «и»
		if bi%2 == 1 {
			ht := math.Mod(t, beat)
			if ht < 0.05 {
				w := rand.Float64()*2 - 1
				bright := w - prevW*0.85
				prevW = w
				v += 0.03 * math.Exp(-ht/0.02) * bright
			}
		}

		s[i] = v*0.55 + 0.05*noise[i] // винил всегда на фоне
	}

	// лоу-пасс «в один полюс» — приглушить верха, эффект старой записи
	y := 0.0
	for i := 0; i < n; i++ {
		y += 0.22 * (s[i] - y)
		s[i] = y
	}

	d := int(float64(sr) * beat * 0.5)
	orig := make([]float64, n)
	copy(orig, s)
	for i := 0; i < n; i++ {
		s[i] += 0.18 * orig[(i-d+n)%n]
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
