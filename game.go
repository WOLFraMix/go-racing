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

	// зазор между верхом новой машины и верхней машиной полосы
	spawnPlatoonGap = 220.0

	// случайный вертикальный сдвиг спавна: плетни не стартуют синхронно
	spawnStagger = 220.0

	// запрет «строк»: в соседних полосах от точки спавна не должно быть
	// машин ближе этого расстояния по вертикали
	rowClear = 100.0
)

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
			KindSedan:     loadSprite("assets/car_sedan.png", color.RGBA{R: 170, G: 60, B: 55, A: 255}),
			KindSport:     loadSprite("assets/car_sport.png", color.RGBA{R: 220, G: 170, B: 40, A: 255}),
			KindSUV:       loadSprite("assets/car_suv.png", color.RGBA{R: 55, G: 120, B: 75, A: 255}),
			KindBus:       loadSprite("assets/bus.png", color.RGBA{R: 235, G: 185, B: 40, A: 255}),
			KindMinibus:   loadSprite("assets/minibus.png", color.RGBA{R: 220, G: 225, B: 230, A: 255}),
			KindTruck:     loadSprite("assets/truck.png", color.RGBA{R: 130, G: 135, B: 145, A: 255}),
			KindAmbulance: loadSprite("assets/ambulance.png", color.RGBA{R: 240, G: 244, B: 248, A: 255}),
		},
	}
	for _, m := range skinMeta {
		g.playerSkins = append(g.playerSkins, loadSprite("assets/"+m.file, m.fallback))
		g.skinNames = append(g.skinNames, m.name)
	}

	// паки скинов врагов — обычные файлы в assets/skins/<тип>/
	g.enemyVariants = map[CarKind][]Sprite{
		KindSedan:     loadSkinPack("assets/skins/sedan", g.sprites[KindSedan]),
		KindSport:     loadSkinPack("assets/skins/sport", g.sprites[KindSport]),
		KindSUV:       loadSkinPack("assets/skins/suv", g.sprites[KindSUV]),
		KindMinibus:   loadSkinPack("assets/skins/minibus", g.sprites[KindMinibus]),
		KindBus:       loadSkinPack("assets/skins/bus", g.sprites[KindBus]),
		KindTruck:     loadSkinPack("assets/skins/truck", g.sprites[KindTruck]),
		KindAmbulance: loadSkinPack("assets/skins/ambulance", g.sprites[KindAmbulance]),
	}

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
		return g.updateMenu()
	case StatePlaying:
		g.updatePlaying()
	case StatePaused:
		g.audio.SilenceEngine()
		g.updatePaused()
	case StateGameOver:
		g.audio.SilenceEngine()
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
	bs := make([]blocker, 0, len(g.enemies)+1)
	for i := range g.enemies {
		e := &g.enemies[i]
		bs = append(bs, blocker{laneOfX(e.X + e.W/2), e.Y, e.Y + e.H})
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

	// 1) форс-перестроение в физически свободную соседнюю полосу
	if e.TargetLane == e.Lane {
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
	for i := range g.enemies {
		o := &g.enemies[i]
		dl := laneOfX(o.X+o.W/2) - l
		if dl < 0 {
			dl = -dl
		}
		if dl > 1 {
			continue
		}
		if o.Y < e.Y+e.H+rowClear && e.Y < o.Y+o.H+rowClear {
			return false
		}
	}
	p := g.player
	dl := laneOfX(p.X+p.W/2) - l
	if dl < 0 {
		dl = -dl
	}
	if dl <= 1 && p.Y < e.Y+e.H+rowClear && e.Y < p.Y+p.H+rowClear {
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

	// «клевок» машины: тормоз — назад, газ — вперёд.
	// Сдвиг входит в логическую позицию g.player.Y: спрайт и хитбокс
	// всегда в одном месте, расхождение невозможно по построению.
	targetYOff := 0.0
	if g.playerBraking {
		targetYOff = 24.0
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

	// страховочный фильтр: дорога всегда перетекает медленно,
	// даже если выше по цепочке где-то ступенька
	k := math.Min(1, 2.0*dt)

	g.smGrass = lerpColor(g.smGrass, def.Grass, k)
	g.smAsphalt = lerpColor(g.smAsphalt, def.Asphalt, k)
	g.smMark = lerpColor(g.smMark, def.Mark, k)
	g.smNight += (def.Night - g.smNight) * k

	// плавный разгон внутри этапа
	prog := math.Min(1, g.stageTime/def.Duration)
	target := def.ScrollStart + (def.ScrollMax-def.ScrollStart)*prog
	g.scrollSpeed += (target - g.scrollSpeed) * math.Min(1, 2*dt)
	eff := g.scrollSpeed * (1 + 0.35*g.boost)
	if g.playerBraking {
		eff *= 0.6
	}

	// мир едет
	mark := eff * dt
	g.markOffset += mark
	g.distance += mark / 30

	// звук мотора
	g.audio.UpdateEngine(eff/100, g.boost, dt, g.playerBraking, g.muted)

	// глобальный кулдаун перестроений: не чаще одного манёвра на дорогу
	if g.changePause > 0 {
		g.changePause -= 60 * dt
	}

	// спавн врагов
	g.spawnTimer -= dt
	if g.spawnTimer <= 0 {
		if g.spawnEnemy() {
			g.spawnTimer = math.Max(0.25, (120+rand.Float64()*120)/(math.Max(eff, 1)*diffMult[g.difficulty]))
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

		if hit(g.player, *e) {
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

// spawnEnemy — спавн в любую полосу, где есть зазор за плетнем,
// но только если маршрут проезда и зона комфорта сохраняются
// (spawnSafe). Полосы выбираются в случайном порядке — «вечных
// свободных полос» не существует. Возвращает true, если заспавнено.
func (g *Game) spawnEnemy() bool {
	var cand []int
	for l := 0; l < laneCount; l++ {
		if g.topmostYInLane(l) > spawnPlatoonGap {
			cand = append(cand, l)
		}
	}
	rand.Shuffle(len(cand), func(i, j int) { cand[i], cand[j] = cand[j], cand[i] })

	for _, l := range cand {
		e := RandomEnemy(l)
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
	}
	ps := g.playerSkins[g.skinIdx]
	drawCar(screen, playerDraw, ps.Img)
	drawBrakeLights(screen, playerDraw, g.playerBraking)

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
	eff := g.scrollSpeed * (1 + 0.35*g.boost)
	if g.playerBraking {
		eff *= 0.6
	}

	gold := color.RGBA{R: 255, G: 210, B: 60, A: 255}
	cyan := color.RGBA{R: 120, G: 220, B: 255, A: 255}
	grey := color.RGBA{R: 200, G: 200, B: 210, A: 255}
	green := color.RGBA{R: 120, G: 255, B: 120, A: 255}

	drawText(screen, fmt.Sprintf("A: %d %d %d N: %.2f",
		g.smAsphalt.R, g.smAsphalt.G, g.smAsphalt.B, g.smNight), 12, 118, 14, grey)

	drawText(screen, fmt.Sprintf("Score: %d", g.curScore()), 12, 10, 20, gold)
	drawText(screen, fmt.Sprintf("Best: %d", g.highScore), 12, 38, 20, cyan)
	drawText(screen, fmt.Sprintf("Speed: %d km/h", int(eff*0.3)), 12, 66, 18, green)
	drawText(screen, fmt.Sprintf("Dist: %d m", int(g.distance)), 12, 92, 18, grey)
	drawTextCentered(screen, def.Name, 14, 18, grey)
	if g.muted {
		drawTextCentered(screen, "MUTED (M)", 42, 16, grey)
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
