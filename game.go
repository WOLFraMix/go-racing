package main

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	_ "image/png"
	"math"
	"math/bits"
	"math/rand"
	"os"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/inpututil"
	"github.com/hajimehoshi/ebiten/v2/vector"
)

// ---------- константы экрана и дороги ----------

const (
	screenWidth  = 520
	screenHeight = 1000

	roadX0    = 40
	roadX1    = 460
	laneCount = 6
	laneWidth = (roadX1 - roadX0) / laneCount // 70

	playerYBase = screenHeight - 210

	// проезжаемость: маршрут от игрока до верха дороги существует всегда.
	// Проверяется BFS-ом каждый кадр; спавн и перестроения не имеют
	// права маршрут ломать.
	passBand    = 40.0   // высота клетки сетки проезда
	passTop     = -420.0 // верх проверки: выше экрана, с запасом на stagger
	comfortZone = 160.0  // зона комфорта вокруг игрока

	passLanesMin = 2 // в каждой строке дороги свободны минимум две полосы

	// сколько машин максимум вскрывается за один кадр страховки
	repairSteps = 8

	// случайный вертикальный сдвиг спавна: плетни не стартуют синхронно
	spawnStagger = 220.0
)

// spawnTuning — параметры спавна, зависящие от сложности.
// Плотность задаётся расстоянием между машинами (px дороги),
// а не секундами: она не зависит от скорости скролла.
type spawnTuning struct {
	platoonGap float64 // минимальный зазор до верхней машины полосы
	rowClear   float64 // запрет «строк» рядом с точкой спавна
	interval   float64 // базовое расстояние дороги между спавнами
	span       float64 // случайная надбавка к нему
}

var diffSpawn = [3]spawnTuning{
	{platoonGap: 230, rowClear: 110, interval: 200, span: 120}, // EASY: машина каждые 200–320 px
	{platoonGap: 175, rowClear: 95, interval: 120, span: 80},   // MEDIUM: каждые 120–200 px
	{platoonGap: 135, rowClear: 80, interval: 70, span: 50},    // HARD: каждые 70–120 px
}

// множитель плотности спавна по сложности
var diffMult = [...]float64{1.0, 1.5, 2.0}
var diffNames = [...]string{"EASY", "MEDIUM", "HARD"}

type State int

const (
	StateMenu State = iota
	StatePlaying
	StatePaused
	StateGameOver
)

// spark — искра взрыва. Своя мини-система, чтобы не зависеть от полей
// Particle из particle.go.
type spark struct {
	X, Y, Vx, Vy    float64
	Life, Max, Size float64
	Clr             color.RGBA
}

type Game struct {
	state State

	player  Car
	enemies []Car
	sparks  []spark

	// оставлено для совместимости с particle.go — здесь не используется
	particles []Particle

	highScore int
	bonus     int
	distance  float64

	scrollSpeed float64
	spawnTimer  float64
	markOffset  float64
	changePause float64 // глобальный кулдаун перестроений, в кадрах

	stageIdx  int
	stageTime float64
	stageLoop int
	banner    float64

	boost         float64
	playerBraking bool
	yOff          float64 // визуальный сдвиг машины по Y

	frame float64

	sirenOn bool // проблесковые маячки (только полицейский скин)

	hornCd float64 // кулдаун гудка, в кадрах

	muted      bool
	skinIdx    int
	startStage int
	difficulty int // 0 — easy, 1 — medium, 2 — hard

	audio *AudioSys

	sprites     map[CarKind]Sprite
	playerSkins []Sprite
	skinNames   []string

	scratch *ebiten.Image // временный холст для кроссфейда декораций

	// сгенерированные варианты скинов врагов: перекраска + детали
	enemyVariants map[CarKind][]Sprite

	// фронт затора для ремонта: заполняется passPath при провале
	wallBand  int
	wallLanes uint8

	smGrass, smAsphalt, smMark color.RGBA
	smNight                    float64

	turbo     float64 // активность турбо 0..1
	turboHeat float64 // нагрев 0..1: 1 — выдохся, остывает при отпускании
	turboLock bool    // перегрелся при зажатом Shift — блокировка до отпускания
}

// ---------- загрузка спрайтов ----------

func loadSprite(path string, fallback color.RGBA) Sprite {
	data, err := os.ReadFile(path)
	if err != nil {
		return Sprite{Fill: fallback}
	}
	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return Sprite{Fill: fallback}
	}
	return Sprite{Img: ebiten.NewImageFromImage(img), Fill: fallback}
}

// ---------- создание и сброс ----------

func NewGame() *Game {
	g := &Game{
		state: StateMenu,
		audio: NewAudio(),
		sprites: map[CarKind]Sprite{
			KindSedan:       loadSprite("assets/car_sedan.png", color.RGBA{R: 170, G: 60, B: 55, A: 255}),
			KindSport:       loadSprite("assets/car_sport.png", color.RGBA{R: 220, G: 170, B: 40, A: 255}),
			KindSUV:         loadSprite("assets/car_suv.png", color.RGBA{R: 55, G: 120, B: 75, A: 255}),
			KindBus:         loadSprite("assets/bus.png", color.RGBA{R: 235, G: 185, B: 40, A: 255}),
			KindMinibus:     loadSprite("assets/minibus.png", color.RGBA{R: 220, G: 225, B: 230, A: 255}),
			KindTruck:       loadSprite("assets/truck.png", color.RGBA{R: 130, G: 135, B: 145, A: 255}),
			KindAmbulance:   loadSprite("assets/ambulance.png", color.RGBA{R: 240, G: 244, B: 248, A: 255}),
			KindPoliceSedan: loadSprite("assets/police_sedan.png", color.RGBA{R: 40, G: 80, B: 170, A: 255}),
		},
	}
	for _, m := range skinMeta {
		g.playerSkins = append(g.playerSkins, loadSprite("assets/"+m.file, m.fallback))
		g.skinNames = append(g.skinNames, m.name)
	}

	// паки скинов врагов — обычные файлы в assets/skins/<тип>/
	g.enemyVariants = map[CarKind][]Sprite{
		KindSedan:   loadSkinPack("assets/skins/sedan", g.sprites[KindSedan]),
		KindSport:   loadSkinPack("assets/skins/sport", g.sprites[KindSport]),
		KindSUV:     loadSkinPack("assets/skins/suv", g.sprites[KindSUV]),
		KindMinibus: loadSkinPack("assets/skins/minibus", g.sprites[KindMinibus]),
		KindBus:     loadSkinPack("assets/skins/bus", g.sprites[KindBus]),
		KindTruck:   loadSkinPack("assets/skins/truck", g.sprites[KindTruck]),
	}
	// скорая и полицейский седан — единственные скины, без паков вариантов
	g.enemyVariants[KindAmbulance] = []Sprite{g.sprites[KindAmbulance]}
	g.enemyVariants[KindPoliceSedan] = []Sprite{g.sprites[KindPoliceSedan]}

	g.readSettings()
	g.readHighScore()
	if g.difficulty < 0 || g.difficulty >= len(diffMult) {
		g.difficulty = 0
	}
	if g.skinIdx < 0 || g.skinIdx >= len(g.playerSkins) {
		g.skinIdx = 0
	}
	if g.startStage < 0 || g.startStage >= len(stageDefs) {
		g.startStage = 0
	}
	g.reset(g.startStage)
	return g
}

func (g *Game) reset(startStage int) {
	g.player = Car{
		X:    laneCenterX(3, carW),
		Y:    float64(playerYBase),
		W:    carW,
		H:    carH,
		Kind: KindPlayer,
	}
	g.stageIdx = startStage % len(stageDefs)
	d := stageDefs[g.stageIdx]
	g.smGrass, g.smAsphalt, g.smMark = d.Grass, d.Asphalt, d.Mark
	g.smNight = d.Night
	g.enemies = nil
	g.sparks = nil
	g.boost = 0
	g.playerBraking = false
	g.yOff = 0
	g.spawnTimer = 0.8
	g.bonus = 0
	g.distance = 0
	g.markOffset = 0
	g.stageTime = stageTransition / 2
	g.stageLoop = 0
	g.banner = 2.0
	g.turbo = 0
	g.turboHeat = 0
	g.turboLock = false
	g.sirenOn = false
	g.audio.SetSiren(false, g.muted)
	g.scrollSpeed = stageDefs[g.stageIdx].ScrollStart
	g.readHighScore()
}

func (g *Game) curScore() int {
	return g.bonus
}

// ---------- Update: диспетчер состояний ----------

func (g *Game) Update() error {
	switch g.state {
	case StateMenu:
		g.audio.SilenceEngine() // в меню мотор молчит
		g.audio.SetSiren(false, g.muted)
		return g.updateMenu()
	case StatePlaying:
		g.updatePlaying()
	case StatePaused:
		g.audio.SilenceEngine()
		g.audio.SetSiren(false, g.muted)
		g.updatePaused()
	case StateGameOver:
		g.audio.SilenceEngine()
		g.audio.SetSiren(false, g.muted)
		g.updateGameOver()
	}
	return nil
}

func (g *Game) Draw(screen *ebiten.Image) {
	switch g.state {
	case StateMenu:
		g.drawMenu(screen)
	default:
		g.drawWorld(screen)
		g.drawHUD(screen)
		if g.banner > 0 {
			g.drawBanner(screen)
		}
		if g.state == StatePaused {
			g.drawPause(screen)
		}
		if g.state == StateGameOver {
			g.drawGameOver(screen)
		}
	}
}

func (g *Game) Layout(outsideWidth, outsideHeight int) (int, int) {
	return screenWidth, screenHeight
}

// ---------- гарантия проезда: BFS по сетке ----------

// blocker — машина как препятствие: полоса по центру кузова,
// вертикальный отрезок [y0, y1].
type blocker struct {
	lane   int
	y0, y1 float64
}

// collectBlockers — все машины на экране + игрок.
func (g *Game) collectBlockers() []blocker {
	bs := make([]blocker, 0, len(g.enemies)+2)
	for i := range g.enemies {
		e := &g.enemies[i]
		l := laneOfX(e.X + e.W/2)
		bs = append(bs, blocker{lane: l, y0: e.Y, y1: e.Y + e.H})
		// маневрирующая машина физически занимает и целевую полосу
		if e.TargetLane != l && e.TargetLane >= 0 && e.TargetLane < laneCount {
			bs = append(bs, blocker{lane: e.TargetLane, y0: e.Y, y1: e.Y + e.H})
		}
	}
	p := &g.player
	bs = append(bs, blocker{laneOfX(p.X + p.W/2), p.Y, p.Y + p.H})
	return bs
}

// passPath — гарантия проезда на ДВУХ полосах. Сетка: полоса × строка
// высотой passBand. Правила:
//  1. в каждой строке свободны минимум passLanesMin полос;
//  2. свободные полосы соседних строк связаны: из занятой игроком
//     области до верха дороги существует непрерывный маршрут по
//     свободным клеткам (вперёд или по диагонали — манёвр).
//
// При провале запоминает фронт затора для ремонта.
func (g *Game) passPath(extra ...blocker) bool {
	bs := g.collectBlockers()
	bs = append(bs, extra...)

	nB := int((g.player.Y+g.player.H+40.0-passTop)/passBand) + 1
	occ := make([]uint8, nB) // бит l — полоса l занята в строке i
	for _, b := range bs {
		i0 := int((b.y0 - passTop) / passBand)
		i1 := int((b.y1 - passTop) / passBand)
		if i0 < 0 {
			i0 = 0
		}
		if i1 > nB-1 {
			i1 = nB - 1
		}
		for i := i0; i <= i1; i++ {
			occ[i] |= 1 << uint(b.lane)
		}
	}

	// правило 1: в каждой строке минимум две свободные полосы
	free := make([]uint8, nB)
	for i := 0; i < nB; i++ {
		free[i] = ^occ[i] & 0x3F
		if bits.OnesCount8(free[i]) < passLanesMin {
			g.wallBand = i
			g.wallLanes = occ[i]
			return false
		}
	}

	// правило 2: связность свободных полос от строки игрока до верха.
	// mask — полосы, достижимые из текущего положения игрока.
	pl := laneOfX(g.player.X + g.player.W/2)
	pi := int((g.player.Y + g.player.H/2 - passTop) / passBand)
	if pi > nB-1 {
		pi = nB - 1
	}
	mask := uint8(1 << uint(pl))
	for i := pi - 1; i >= 0; i-- {
		// из полосы l можно уйти в l-1, l, l+1 — манёвр
		mask |= mask<<1 | mask>>1
		mask &= free[i] & 0x3F
		if mask == 0 {
			g.wallBand = i
			g.wallLanes = occ[i]
			return false
		}
	}
	return true
}

// ensurePassable — ежекадровая страховка: если гарантия «две свободные
// полосы + связность» нарушена (плетни сползлись), вскрываем затор
// сразу, за один кадр — до того, как он доедет до игрока.
func (g *Game) ensurePassable() {
	for i := 0; i < repairSteps; i++ {
		if g.passPath() {
			return
		}
		if !g.repairWall() {
			return
		}
	}
}

// repairWall — вскрывает затор на фронте wallBand: выбирает машину-
// преграду возле фронта и либо принудительно перестраивает её (если
// соседняя полоса физически свободна), либо даёт ускорение, чтобы она
// уехала с линии и открыла щель. Возвращает true, если мера применена.
func (g *Game) repairWall() bool {
	bandY0 := passTop + float64(g.wallBand-1)*passBand
	bandY1 := bandY0 + 2*passBand
	lm := g.wallLanes
	lm = (lm | lm<<1 | lm>>1) & 0x3F // фронт + соседние полосы

	var cands []*Car
	for i := range g.enemies {
		e := &g.enemies[i]
		if e.Y+e.H < bandY0 || e.Y > bandY1 {
			continue
		}
		if lm&(1<<uint(laneOfX(e.X+e.W/2))) != 0 {
			cands = append(cands, e)
		}
	}
	if len(cands) == 0 {
		return false
	}

	e := cands[rand.Intn(len(cands))]

	// 1) форс-перестроение — только если сейчас никто не маневрирует.
	// Иначе затор раскрываем ускорением: аварийный путь не имеет права
	// плодить одновременные манёвры, как и обычный ИИ
	if !g.changeBusy() && e.TargetLane == e.Lane {
		for _, nl := range [2]int{e.Lane - 1, e.Lane + 1} {
			if nl >= 0 && nl < laneCount && g.laneFreeFor(*e, -1, nl) {
				e.TargetLane = nl
				e.Blink = nl - e.Lane
				e.Cooldown = changeCooldown
				return true
			}
		}
	}

	// 2) ускорение: машина уезжает вперёд, щель открывается
	fast := 2.4 + rand.Float64()*0.5
	if e.BaseOwn < fast {
		e.BaseOwn = fast
	}
	if e.OwnSpeed < e.BaseOwn {
		e.OwnSpeed = e.BaseOwn
	}
	e.BoostUntil = int(g.frame) + 90
	return true
}

// changeBusy — верно, пока хоть одна машина в потоке маневрирует.
// Пока кто-то перестраивается, новые манёвры не начинаются.
func (g *Game) changeBusy() bool {
	for i := range g.enemies {
		if e := &g.enemies[i]; e.TargetLane != e.Lane {
			return true
		}
	}
	return false
}

// spawnSafe — спавн или перестроение допустимы, только если маршрут
// проезда сохраняется, а в зоне комфорта вокруг игрока на каждой линии
// остаются свободными минимум две полосы — запас на манёвр.
func (g *Game) spawnSafe(extra ...blocker) bool {
	if !g.passPath(extra...) {
		return false
	}
	bs := g.collectBlockers()
	bs = append(bs, extra...)
	py0 := g.player.Y - comfortZone
	py1 := g.player.Y + g.player.H + comfortZone
	for y := py0; y <= py1; y += passBand {
		free := 0
		for l := 0; l < laneCount; l++ {
			blocked := false
			for _, b := range bs {
				if b.lane == l && b.y0 < y && b.y1 > y {
					blocked = true
					break
				}
			}
			if !blocked {
				free++
			}
		}
		if free < 2 {
			return false
		}
	}
	return true
}

// antiRow — запрет «строк»: в своей и соседних полосах от точки
// спавна не должно быть машин ближе rowClear по вертикали.
// Ряд из машин на одной высоте не образуется даже при плотном спавне.
func (g *Game) antiRow(e Car) bool {
	l := laneOfX(e.X + e.W/2)
	rc := diffSpawn[g.difficulty].rowClear
	for i := range g.enemies {
		o := &g.enemies[i]
		dl := laneOfX(o.X+o.W/2) - l
		if dl < 0 {
			dl = -dl
		}
		if dl > 1 {
			continue
		}
		if o.Y < e.Y+e.H+rc && e.Y < o.Y+o.H+rc {
			return false
		}
	}
	p := g.player
	dl := laneOfX(p.X+p.W/2) - l
	if dl < 0 {
		dl = -dl
	}
	if dl <= 1 && p.Y < e.Y+e.H+rc && e.Y < p.Y+p.H+rc {
		return false
	}
	return true
}

// ---------- игровая логика ----------

func (g *Game) updatePlaying() {
	dt := 1.0 / 60.0
	g.frame++

	if inpututil.IsKeyJustPressed(ebiten.KeyP) {
		g.state = StatePaused
		return
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyM) {
		g.muted = !g.muted
		g.saveSettings()
	}
	if inpututil.IsKeyJustPressed(ebiten.KeySpace) {
		// сирена — только у машин с маячками на крыше (ПАТРИОТ)
		if beaconSkins[g.skinNames[g.skinIdx]] {
			g.sirenOn = !g.sirenOn
			g.audio.SetSiren(g.sirenOn, g.muted)
		} else {
			// все остальные — короткий гудок с кулдауном
			if g.hornCd <= 0 {
				g.audio.PlayHorn(g.muted)
				g.hornCd = 30 // не чаще двух раз в секунду
			}
		}
	}
	if g.hornCd > 0 {
		g.hornCd -= 60 * dt
	}

	// руль: поворотник в «кадрах», как у врагов
	const steer = 340.0
	if ebiten.IsKeyPressed(ebiten.KeyLeft) || ebiten.IsKeyPressed(ebiten.KeyA) {
		g.player.X -= steer * dt
		g.player.Blink = -1
		g.player.BlinkTimer = 36
	}
	if ebiten.IsKeyPressed(ebiten.KeyRight) || ebiten.IsKeyPressed(ebiten.KeyD) {
		g.player.X += steer * dt
		g.player.Blink = 1
		g.player.BlinkTimer = 36
	}
	if g.player.BlinkTimer > 0 {
		g.player.BlinkTimer -= 60 * dt
	}
	g.player.X = clamp(g.player.X, float64(roadX0+4), float64(roadX1-4-carW))

	// газ и тормоз
	g.playerBraking = ebiten.IsKeyPressed(ebiten.KeyS) || ebiten.IsKeyPressed(ebiten.KeyDown)
	if ebiten.IsKeyPressed(ebiten.KeyW) || ebiten.IsKeyPressed(ebiten.KeyUp) {
		g.boost += (1 - g.boost) * math.Min(1, 5*dt)
	} else {
		g.boost += (0 - g.boost) * math.Min(1, 4*dt)
	}

	// турбо: удержание Shift — тяга с перегревом.
	// Перегрелся — блокируется до отпускания Shift, иначе получился бы
	// пилообразный цикл «остыл на миг — снова включился» = вечное турбо.
	shiftHeld := ebiten.IsKeyPressed(ebiten.KeyShift) ||
		ebiten.IsKeyPressed(ebiten.KeyShiftLeft) ||
		ebiten.IsKeyPressed(ebiten.KeyShiftRight)
	if shiftHeld && !g.turboLock && g.turboHeat < 1.0 {
		g.turbo += (1 - g.turbo) * math.Min(1, 8*dt)
		g.turboHeat += 0.20 * dt // полный бак горит ~5 с
		if g.turboHeat >= 1.0 {
			g.turboLock = true
		}
	} else {
		g.turbo += (0 - g.turbo) * math.Min(1, 6*dt)
		g.turboHeat -= 0.22 * dt // остывает за ~4.5 с
		// разблокировка только после отпускания Shift и заметного остывания
		if !shiftHeld && g.turboHeat < 0.6 {
			g.turboLock = false
		}
	}
	g.turbo = clamp(g.turbo, 0, 1)
	g.turboHeat = clamp(g.turboHeat, 0, 1)

	// «клевок» машины: тормоз — назад, газ — вперёд.
	// Сдвиг входит в логическую позицию g.player.Y: спрайт и хитбокс
	// всегда в одном месте, расхождение невозможно по построению.
	targetYOff := 0.0
	if g.playerBraking {
		targetYOff = 24.0
	} else if g.turbo > 0.5 {
		targetYOff = -20.0
	} else if g.boost > 0.5 {
		targetYOff = -14.0
	}
	g.yOff += (targetYOff - g.yOff) * math.Min(1, 15*dt)
	g.player.Y = float64(playerYBase) + g.yOff

	// смена этапа
	def := stageDefs[g.stageIdx]
	g.stageTime += dt
	if g.stageTime >= def.Duration {
		g.stageTime = 0
		g.stageIdx = (g.stageIdx + 1) % len(stageDefs)
		if g.stageIdx == 0 {
			g.stageLoop++
		}
		g.banner = 2.0
	}
	if g.banner > 0 {
		g.banner -= dt
	}

	def = g.currentDef()

	// переход этапов
	k := math.Min(1, 0.7*dt)

	g.smGrass = lerpColor(g.smGrass, def.Grass, k)
	g.smAsphalt = lerpColor(g.smAsphalt, def.Asphalt, k)
	g.smMark = lerpColor(g.smMark, def.Mark, k)
	g.smNight += (def.Night - g.smNight) * k

	// плавный разгон внутри этапа
	prog := math.Min(1, g.stageTime/def.Duration)
	target := def.ScrollStart + (def.ScrollMax-def.ScrollStart)*prog

	// стык этапов: первые 3 секунды подмешиваем конечную скорость
	// предыдущего этапа — торможение/разгон размазывается, ступеньки нет
	if g.stageTime < 3.0 {
		prev := stageDefs[(g.stageIdx+len(stageDefs)-1)%len(stageDefs)]
		bt := g.stageTime / 3.0
		target = prev.ScrollMax + (target-prev.ScrollMax)*bt
	}
	g.scrollSpeed += (target - g.scrollSpeed) * math.Min(1, 2*dt)
	eff := g.scrollSpeed * (1 + 0.35*g.boost + 0.55*g.turbo)
	if g.playerBraking {
		eff *= 0.6
	}

	// мир едет
	mark := eff * dt
	g.markOffset += mark
	g.distance += mark / 30

	// звук мотора
	g.audio.UpdateEngine(eff/100, math.Max(g.boost, g.turbo), dt, g.playerBraking, g.muted)

	// глобальный кулдаун перестроений: не чаще одного манёвра на дорогу
	if g.changePause > 0 {
		g.changePause -= 60 * dt
	}

	// спавн врагов
	g.spawnTimer -= dt
	if g.spawnTimer <= 0 {
		if g.spawnEnemy() {
			st := diffSpawn[g.difficulty]
			g.spawnTimer = (st.interval + rand.Float64()*st.span) / math.Max(eff, 1)
		} else {
			g.spawnTimer = 0.12
		}
	}

	// ИИ врагов: eff/60 — px за кадр (единицы OwnSpeed), 60*dt — dt в кадрах
	g.updateEnemies(eff/60.0, 60.0*dt)

	// страховка проезда каждый кадр: вскрываем заторы, если путь пропал
	g.ensurePassable()

	// объезд машин, столкновения, вылет за экран
	for i := len(g.enemies) - 1; i >= 0; i-- {
		e := &g.enemies[i]

		// машина проехала мимо игрока — очки за объезд с множителем
		// сложности; вплотную — двойная награда
		if !e.NearMiss && e.Y+e.H > g.player.Y && e.Y < g.player.Y+g.player.H {
			e.NearMiss = true
			g.bonus += e.Points * (g.difficulty + 1)
			dx := math.Abs((e.X + e.W/2) - (g.player.X + g.player.W/2))
			if dx < (e.W+g.player.W)/2+10 {
				g.bonus += e.Points * (g.difficulty + 1)
			}
		}

		if hitRect(g.player, *e) {
			g.crash()
			return
		}

		if e.Y > screenHeight+200 {
			g.enemies = append(g.enemies[:i], g.enemies[i+1:]...)
		}
	}

	// искры взрыва
	for i := len(g.sparks) - 1; i >= 0; i-- {
		p := &g.sparks[i]
		p.X += p.Vx * dt
		p.Y += p.Vy * dt
		p.Vx *= 0.98
		p.Vy = p.Vy*0.98 + 60*dt
		p.Life -= dt
		if p.Life <= 0 {
			g.sparks = append(g.sparks[:i], g.sparks[i+1:]...)
		}
	}
}

func (g *Game) crash() {
	g.audio.PlayCrash(0.5, g.muted)
	cx, cy := g.player.X+g.player.W/2, g.player.Y+g.player.H/2
	for i := 0; i < 46; i++ {
		ang := rand.Float64() * 2 * math.Pi
		sp := 60 + rand.Float64()*260
		life := 0.5 + rand.Float64()*0.6
		col := color.RGBA{R: 255, G: uint8(120 + rand.Intn(120)), B: 40, A: 255}
		if rand.Intn(3) == 0 {
			col = color.RGBA{R: 90, G: 90, B: 95, A: 255}
		}
		g.sparks = append(g.sparks, spark{
			X: cx, Y: cy,
			Vx:   math.Cos(ang) * sp,
			Vy:   math.Sin(ang)*sp - 60,
			Life: life, Max: life,
			Size: 2 + rand.Float64()*3,
			Clr:  col,
		})
	}
	if g.curScore() > g.highScore {
		g.highScore = g.curScore()
		g.saveHighScore()
	}
	g.state = StateGameOver
}

func (g *Game) updatePaused() {
	if inpututil.IsKeyJustPressed(ebiten.KeyP) {
		g.state = StatePlaying
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyEscape) {
		g.state = StateMenu
	}
}

func (g *Game) updateGameOver() {
	if inpututil.IsKeyJustPressed(ebiten.KeyR) {
		g.reset(g.startStage)
		g.state = StatePlaying
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyEscape) {
		g.state = StateMenu
	}
}

// topmostYInLane — Y самой верхней машины в полосе (1e9, если пусто).
func (g *Game) topmostYInLane(l int) float64 {
	best := 1e9
	for i := range g.enemies {
		e := &g.enemies[i]
		if laneOfX(e.X+e.W/2) == l && e.Y < best {
			best = e.Y
		}
	}
	return best
}

// hasKind — есть ли на экране хотя бы одна машина этого типа.
func (g *Game) hasKind(k CarKind) bool {
	for i := range g.enemies {
		if g.enemies[i].Kind == k {
			return true
		}
	}
	return false
}

// spawnEnemy — спавн в любую полосу, где есть зазор за плетнем,
// но только если маршрут проезда и зона комфорта сохраняются
// (spawnSafe). Полосы выбираются в случайном порядке — «вечных
// свободных полос» не существует. Возвращает true, если заспавнено.
func (g *Game) spawnEnemy() bool {
	var cand []int
	gap := diffSpawn[g.difficulty].platoonGap
	for l := 0; l < laneCount; l++ {
		if g.topmostYInLane(l) > gap {
			cand = append(cand, l)
		}
	}
	rand.Shuffle(len(cand), func(i, j int) { cand[i], cand[j] = cand[j], cand[i] })

	for _, l := range cand {
		e := RandomEnemy(l)
		if e.Kind == KindPoliceSedan && g.hasKind(KindPoliceSedan) {
			continue
		}
		e.Y -= rand.Float64() * spawnStagger
		if !g.antiRow(e) {
			continue // не сеем строки: рядом по высоте уже кто-то стоит
		}
		if g.spawnSafe(blocker{lane: l, y0: e.Y, y1: e.Y + e.H}) {
			if len(g.enemyVariants[e.Kind]) > 0 {
				e.Skin = rand.Intn(len(g.enemyVariants[e.Kind]))
			}
			g.enemies = append(g.enemies, e)
			return true
		}
	}
	return false
}

// ---------- отрисовка ----------

func (g *Game) drawWorld(screen *ebiten.Image) {
	def := g.currentDef()

	// Кивок (yOff) входит в логическую позицию g.player.Y — спрайт и
	// хитбокс всегда совпадают, отдельного визуального сдвига нет.
	playerDraw := g.player

	vector.FillRect(screen, 0, 0, screenWidth, screenHeight, g.smGrass, false)
	vector.FillRect(screen, roadX0, 0, roadX1-roadX0, screenHeight, g.smAsphalt, false)

	edge := color.RGBA{R: 235, G: 235, B: 240, A: 255}
	vector.FillRect(screen, roadX0+2, 0, 3, screenHeight, edge, false)
	vector.FillRect(screen, roadX1-5, 0, 3, screenHeight, edge, false)

	const dashLen, spacing = 42, 88
	frac := math.Mod(g.markOffset, spacing)
	for j := -1; j <= screenHeight/spacing+1; j++ {
		y := frac + float64(j)*spacing
		for l := 1; l < laneCount; l++ {
			x := roadX0 + laneWidth*l - 2
			vector.FillRect(screen, float32(x), float32(y), 4, dashLen, g.smMark, false)
		}
	}

	g.drawScenery(screen, def)

	// ночь: затемнение, лучи фар и габариты — без порогов, свет
	// разгорается плавно с первых сумерек. Порог здесь давал вспышку
	// ровно на стыке HIGHWAY→DUSK: Night пересекает 0.2 точно на границе
	dark := g.smNight
	if dark > 0.001 {
		vector.FillRect(screen, 0, 0, screenWidth, screenHeight,
			color.RGBA{R: 8, G: 10, B: 30, A: uint8(110 * dark)}, false)
		k := dark * dark // парабола: в сумерках тускло, ночью — в полную
		for _, e := range g.enemies {
			drawHeadlights(screen, e, float32(k))
		}
		drawHeadlights(screen, playerDraw, float32(k))
	}

	for _, e := range g.enemies {
		skin := g.enemyVariants[e.Kind][e.Skin]
		drawCar(screen, e, skin.Img)
		drawBlinker(screen, e, e.Blink, int(g.frame))
		drawBrakeLights(screen, e, e.Braking)
		if e.Kind == KindPoliceSedan || e.Kind == KindAmbulance {
			drawBeacons(screen, e, int(g.frame))
		}
	}
	ps := g.playerSkins[g.skinIdx]
	drawCar(screen, playerDraw, ps.Img)
	drawBrakeLights(screen, playerDraw, g.playerBraking)
	if g.turbo > 0.04 {
		g.drawTurboFlame(screen, playerDraw)
	}
	if g.sirenOn && beaconSkins[g.skinNames[g.skinIdx]] {
		drawBeacons(screen, playerDraw, int(g.frame))
	}

	// габариты и стоп-сигналы — после машин, поверх корпуса
	if dark > 0.001 {
		for _, e := range g.enemies {
			drawTailLights(screen, e, e.Braking)
		}
		drawTailLights(screen, playerDraw, g.playerBraking)
	}

	for _, p := range g.sparks {
		c := p.Clr
		c.A = uint8(clamp(255*p.Life/p.Max, 0, 255))
		vector.FillCircle(screen, float32(p.X), float32(p.Y), float32(p.Size), c, false)
	}
}

func (g *Game) drawHUD(screen *ebiten.Image) {
	def := g.currentDef()
	eff := g.scrollSpeed * (1 + 0.35*g.boost + 0.55*g.turbo)
	if g.playerBraking {
		eff *= 0.6
	}

	gold := color.RGBA{R: 255, G: 210, B: 60, A: 255}
	cyan := color.RGBA{R: 120, G: 220, B: 255, A: 255}
	grey := color.RGBA{R: 200, G: 200, B: 210, A: 255}
	green := color.RGBA{R: 120, G: 255, B: 120, A: 255}

	drawText(screen, fmt.Sprintf("Score: %d", g.curScore()), 12, 10, 20, gold)
	drawText(screen, fmt.Sprintf("Best: %d", g.highScore), 12, 38, 20, cyan)
	drawText(screen, fmt.Sprintf("Speed: %d km/h", int(eff*0.3)), 12, 66, 18, green)
	drawText(screen, fmt.Sprintf("Dist: %d m", int(g.distance)), 12, 92, 18, grey)
	drawTextCentered(screen, def.Name, 14, 18, grey)
	if g.muted {
		drawTextCentered(screen, "MUTED (M)", 42, 16, grey)
	}

	// шкала турбо: справа напротив скорости; полная — перегрев
	if g.turboHeat > 0.01 || g.turbo > 0.01 {
		bx := screenWidth - 156
		bw := 144
		by := 68

		vector.FillRect(screen, float32(bx), float32(by), float32(bw), 14,
			color.RGBA{R: 20, G: 22, B: 30, A: 200}, false)

		// заполнение = остаток ресурса (1 - нагрев), голубая полоса
		avail := bw - 4
		vector.FillRect(screen, float32(bx+2), float32(by+2),
			float32(int(float64(avail)*(1-g.turboHeat))), 10,
			color.RGBA{R: 80, G: 170, B: 255, A: 255}, false)

		// почти перегрелся — рамка мигает красным
		if g.turboHeat > 0.85 {
			if int(g.frame/8)%2 == 0 {
				vector.FillRect(screen, float32(bx), float32(by), float32(bw), 14,
					color.RGBA{R: 255, G: 70, B: 60, A: 120}, false)
			}
			drawTextCenteredX(screen, "TURBO HOT", bx+bw/2, by+1, 10,
				color.RGBA{R: 255, G: 120, B: 110, A: 255})
		}
	}
}

func (g *Game) drawBanner(screen *ebiten.Image) {
	a := g.banner / 2
	if a > 1 {
		a = 1
	}
	clr := color.RGBA{R: 255, G: 210, B: 60, A: uint8(255 * a)}
	drawTextCentered(screen, stageDefs[g.stageIdx].Name, 160, 44, clr)
}

// beaconSkins — скины с маячками на крыше (координаты балки: 24..82 × 64..80
// на холсте спрайта 106×152).
var beaconSkins = map[string]bool{
	"PATRIOT POLICE": true,
}

// beaconRect — прямоугольник балки маячков в мировых координатах.
// У скинов разные балки, координаты — в единицах спрайта 106×152.
func beaconRect(c Car) (bx, by, bw, bh float64) {
	sx, sy := c.W/106.0, c.H/152.0
	if c.Kind == KindPoliceSedan || c.Kind == KindAmbulance {
		return c.X + 30*sx, c.Y + 64*sy, 46 * sx, 16 * sy
	}
	return c.X + 24*sx, c.Y + 64*sy, 58 * sx, 16 * sy
}

// drawTurboFlame — синее пламя из выхлопных патрубков при турбо.
// Два факела у заднего бампера: белое ядро, синий ореол, тёмно-синий
// хвост. Дрожание — по кадру, длина растёт с активностью турбо.
func (g *Game) drawTurboFlame(screen *ebiten.Image, c Car) {
	sx := c.W / 106.0
	sy := c.H / 152.0
	cx := c.X + c.W/2
	y0 := c.Y + c.H - 4*sy

	flick := 0.8 + 0.2*math.Sin(float64(g.frame)*1.9)
	jit := math.Sin(float64(g.frame)*13.0) * 2.0 * sx
	fade := g.turbo * g.turbo // пламя разгоняется вместе с тягой

	// один патрубок — правый, чуть правее центра бампера
	x := cx + 16*sx
	L := (34*flick + 12) * sx * fade

	// хвост: тёмно-синий
	vector.FillCircle(screen, float32(x+jit), float32(y0+L*0.85), float32(5.5*sx*fade),
		color.RGBA{R: 30, G: 60, B: 200, A: 90}, false)
	// середина: ярко-синий
	vector.FillCircle(screen, float32(x), float32(y0+L*0.5), float32(5*sx*fade),
		color.RGBA{R: 60, G: 130, B: 255, A: 170}, false)
	// ядро у патрубка: белое с голубым
	vector.FillCircle(screen, float32(x), float32(y0+4*sy), float32(4*sx),
		color.RGBA{R: 235, G: 245, B: 255, A: uint8(220 * fade)}, false)
}

// drawBeacons — мигание маячков поверх спрайта + свечение вокруг.
// Координаты балки с спрайта переводятся в рект машины пропорционально.
func drawBeacons(screen *ebiten.Image, c Car, frame int) {
	sx, sy := c.W/106.0, c.H/152.0
	bx, by, bw, bh := beaconRect(c)

	ph := (frame / 12) % 2 // такт мигания: ~0.2 с на фазу
	redA, blueA := uint8(50), uint8(50)
	if ph == 0 {
		redA = 230
	} else {
		blueA = 230
	}

	cxL := bx + bw*0.25 // центры половин балки
	cxR := bx + bw*0.75
	cy := by + bh/2

	// ---------- свечение: компактные ореолы в пределах крыши ----------
	// активная сторона светит ярче, пассивная — тлеет
	ra, ba := uint8(28), uint8(28)
	if ph == 0 {
		ra = 75
	} else {
		ba = 75
	}
	for _, layer := range [3]struct {
		r    float64
		mulA float64
	}{
		{22 * sx, 0.45},
		{16 * sx, 0.75},
		{12 * sx, 1.0},
	} {
		ga := uint8(float64(ra) * layer.mulA)
		gb := uint8(float64(ba) * layer.mulA)
		vector.FillCircle(screen, float32(cxL), float32(cy), float32(layer.r),
			color.RGBA{R: 255, G: 60, B: 50, A: ga}, false)
		vector.FillCircle(screen, float32(cxR), float32(cy), float32(layer.r),
			color.RGBA{R: 60, G: 110, B: 255, A: gb}, false)
	}

	// ---------- сами маячки ----------
	vector.FillRect(screen, float32(bx), float32(by), float32(bw/2), float32(bh),
		color.RGBA{R: 255, G: 40, B: 40, A: redA}, false)
	vector.FillRect(screen, float32(bx+bw/2), float32(by), float32(bw/2), float32(bh),
		color.RGBA{R: 40, G: 90, B: 255, A: blueA}, false)

	// блик-вспышка на активной половине
	if ph == 0 {
		vector.FillRect(screen, float32(bx+2*sx), float32(by+2*sy),
			float32(bw/2-4*sx), float32(3*sy),
			color.RGBA{R: 255, G: 210, B: 200, A: 200}, false)
	} else {
		vector.FillRect(screen, float32(bx+bw/2+2*sx), float32(by+2*sy),
			float32(bw/2-4*sx), float32(3*sy),
			color.RGBA{R: 200, G: 225, B: 255, A: 200}, false)
	}
}

func (g *Game) drawPause(screen *ebiten.Image) {
	vector.FillRect(screen, 0, 0, screenWidth, screenHeight,
		color.RGBA{R: 0, G: 0, B: 0, A: 130}, false)
	drawTextCentered(screen, "PAUSE", screenHeight/2-50, 40,
		color.RGBA{R: 255, G: 210, B: 60, A: 255})
	drawTextCentered(screen, "P — продолжить, ESC — в меню", screenHeight/2+16, 18, color.White)
}

func (g *Game) drawGameOver(screen *ebiten.Image) {
	vector.FillRect(screen, 0, 0, screenWidth, screenHeight,
		color.RGBA{R: 60, G: 10, B: 10, A: 150}, false)
	drawTextCentered(screen, "CRASH!", screenHeight/2-140, 48,
		color.RGBA{R: 255, G: 80, B: 60, A: 255})
	drawTextCentered(screen, fmt.Sprintf("Score: %d", g.curScore()), screenHeight/2-60, 26,
		color.RGBA{R: 255, G: 210, B: 60, A: 255})
	drawTextCentered(screen, fmt.Sprintf("Best: %d", g.highScore), screenHeight/2-16, 22,
		color.RGBA{R: 120, G: 220, B: 255, A: 255})
	drawTextCentered(screen, "R — заново, ESC — меню", screenHeight/2+40, 18, color.White)
}

// ---------- утилиты ----------

func clamp(v, lo, hi float64) float64 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}
