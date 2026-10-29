package main

import (
	"math"
	"math/rand"
)

const (
	carW = laneWidth - 14
	carH = 76
	busH = 100

	truckW = laneWidth - 16
	truckH = 130

	laneChangeSpeed = 1.3 // плавное перестроение
	safeGap         = 70
	changeCooldown  = 150

	randomLaneChance = 0.001
)

const (
	lookAhead   = 20.0 // запас дистанции в кадрах пути: чем больше, тем раньше тормозит
	followDecel = 0.08 // мягкое торможение за кадр у цели дистанции
	hardDecel   = 0.35 // экстренное торможение за кадр, когда вплотную
)

type CarKind int

const (
	KindPlayer CarKind = iota
	KindSedan
	KindSport
	KindSUV
	KindMinibus
	KindBus
	KindAmbulance
	KindTruck
)

type Car struct {
	X, Y, W, H float64
	Kind       CarKind
	OwnSpeed   float64
	BaseOwn    float64
	Lane       int
	TargetLane int
	Blink      int
	BlinkTimer float64
	Cooldown   float64
	Braking    bool
	NearMiss   bool
	Points     int
	Skin       int // индекс варианта скина среди сгенерированных
}

// randomSpeed — рандомизация скорости: 80–120% от базовой.
func randomSpeed(base float64) float64 {
	return base * (0.8 + rand.Float64()*0.4)
}

// canChangeInto — манёвр в полосу target допустим, только если даже
// во время перестроения (машина числится в ОБЕИХ полосах) маршрут
// проезда сохраняется, а зона комфорта вокруг игрока не сужается.
func (g *Game) canChangeInto(e Car, target int) bool {
	if target < 0 || target >= laneCount {
		return false
	}
	return g.spawnSafe(
		blocker{lane: e.Lane, y0: e.Y, y1: e.Y + e.H},
		blocker{lane: target, y0: e.Y, y1: e.Y + e.H},
	)
}

func (g *Game) tryLaneChange(e *Car, selfIdx int) bool {
	dir := 0
	if rand.Intn(2) == 0 {
		if g.laneFreeFor(*e, selfIdx, e.Lane-1) && g.canChangeInto(*e, e.Lane-1) {
			dir = -1
		} else if g.laneFreeFor(*e, selfIdx, e.Lane+1) && g.canChangeInto(*e, e.Lane+1) {
			dir = 1
		}
	} else {
		if g.laneFreeFor(*e, selfIdx, e.Lane+1) && g.canChangeInto(*e, e.Lane+1) {
			dir = 1
		} else if g.laneFreeFor(*e, selfIdx, e.Lane-1) && g.canChangeInto(*e, e.Lane-1) {
			dir = -1
		}
	}
	if dir == 0 {
		return false
	}
	e.TargetLane = e.Lane + dir
	e.Blink = dir
	e.Cooldown = changeCooldown
	return true
}

func RandomEnemy(lane int) Car {
	kind := KindSedan
	w, h := float64(carW), float64(carH)
	points := 1
	base := 1.0 + rand.Float64()*0.7

	switch r := rand.Float64(); {
	case r < 0.02:
		kind = KindAmbulance // редкая гостья — 2%
		points = 4
		base = 1.9 + rand.Float64()*0.5
	case r < 0.14:
		kind = KindSport
		points = 2
		base = 2.1 + rand.Float64()*0.6
	case r < 0.27:
		kind = KindTruck
		w = float64(truckW)
		h = float64(truckH)
		points = 3
		base = 0.4 + rand.Float64()*0.4
	case r < 0.39:
		kind = KindMinibus
		points = 2
		base = 0.7 + rand.Float64()*0.3
	case r < 0.49:
		kind = KindBus
		h = float64(busH)
		points = 3
		base = 0.5 + rand.Float64()*0.3
	case r < 0.62:
		kind = KindSUV
	}

	own := randomSpeed(base)
	return Car{
		X: laneCenterX(lane, w), Y: -h, W: w, H: h,
		Kind: kind, OwnSpeed: own, BaseOwn: own,
		Lane: lane, TargetLane: lane, Points: points,
	}
}

// ---------- геометрия ----------

func laneCenterX(lane int, w float64) float64 {
	return float64(roadX0+lane*laneWidth) + (float64(laneWidth)-w)/2
}

func laneOfX(cx float64) int {
	lane := int((cx - float64(roadX0)) / float64(laneWidth))
	if lane < 0 {
		lane = 0
	}
	if lane >= laneCount {
		lane = laneCount - 1
	}
	return lane
}

func xOverlap(a, b Car) bool {
	const m = 4.0
	return a.X+m < b.X+b.W-m && a.X+a.W-m > b.X+m
}

func hit(a, b Car) bool {
	const m = 5.0
	return a.X+m < b.X+b.W-m &&
		a.X+a.W-m > b.X+m &&
		a.Y+m < b.Y+b.H-m &&
		a.Y+a.H-m > b.Y+m
}

func (g *Game) laneFreeFor(e Car, selfIdx, lane int) bool {
	if lane < 0 || lane >= laneCount {
		return false
	}
	probe := Car{X: laneCenterX(lane, e.W), Y: e.Y, W: e.W, H: e.H}
	const need = 30.0

	for i, o := range g.enemies {
		if i == selfIdx {
			continue
		}
		if !xOverlap(probe, o) {
			continue
		}
		if probe.Y < o.Y+o.H+need && o.Y < probe.Y+probe.H+need {
			return false
		}
	}
	p := Car{X: g.player.X, Y: g.player.Y, W: g.player.W, H: g.player.H}
	if xOverlap(probe, p) && probe.Y < p.Y+p.H+need && p.Y < probe.Y+probe.H+need {
		return false
	}
	return true
}

func (g *Game) leaderInfo(e Car) (idx int, gap float64, isPlayer bool) {
	lane := laneOfX(e.X + e.W/2)
	lx := float64(roadX0 + lane*laneWidth)
	rx := lx + float64(laneWidth)

	bestGap := math.MaxFloat64
	idx = -1

	for i := range g.enemies {
		o := &g.enemies[i]
		cx := o.X + o.W/2
		if cx < lx || cx > rx {
			continue
		}
		tail := o.Y + o.H
		if tail > e.Y-1 {
			continue
		}
		gapHere := e.Y - tail
		if gapHere < bestGap {
			bestGap = gapHere
			idx = i
			isPlayer = false
		}
	}

	if pcx := g.player.X + g.player.W/2; pcx >= lx && pcx <= rx {
		tail := g.player.Y + g.player.H
		if tail <= e.Y-1 {
			gapHere := e.Y - tail
			if gapHere < bestGap {
				bestGap = gapHere
				idx = -1
				isPlayer = true
			}
		}
	}

	if idx == -1 && !isPlayer {
		return -1, 0, false
	}
	return idx, bestGap, isPlayer
}

func (g *Game) updateEnemies(eff, dt float64) {
	for i := range g.enemies {
		e := &g.enemies[i]

		if e.TargetLane != e.Lane {
			// боковое перестроение: двигаем X, скорость не трогаем
			tx := laneCenterX(e.TargetLane, e.W)
			dx := tx - e.X
			step := laneChangeSpeed * dt
			if math.Abs(dx) <= step {
				e.X = tx
				e.Lane = e.TargetLane
				e.BlinkTimer = 24
				e.Cooldown = changeCooldown
			} else if dx > 0 {
				e.X += step
			} else {
				e.X -= step
			}
		} else if e.BlinkTimer > 0 {
			e.BlinkTimer -= dt
			if e.BlinkTimer <= 0 {
				e.Blink = 0
			}
		}
		if e.Cooldown > 0 {
			e.Cooldown -= dt
		}

		// в самом боковом манёвре скорость не управляем,
		// но в кулдауне после него — управляем ОБЯЗАТЕЛЬНО:
		// раньше здесь стоял continue, из-за которого машина
		// после перестроения летела вперёд без тормозов
		if e.TargetLane != e.Lane {
			continue
		}

		leaderIdx, gap, leaderIsPlayer := g.leaderInfo(*e)

		if leaderIdx >= 0 || leaderIsPlayer {
			leadSpeed := 0.0
			if leaderIdx >= 0 {
				leadSpeed = g.enemies[leaderIdx].OwnSpeed
			} else {
				leadSpeed = eff
			}

			// желаемая дистанция: safeGap + запас на скорость сближения.
			// dc > 0 — догоняем лидера: тормозим ЗАРАНЕЕ, а не на пороге.
			dc := e.OwnSpeed - leadSpeed
			desired := safeGap + dc*lookAhead
			if desired < safeGap {
				desired = safeGap
			}

			if gap < desired {
				// манёвр возможен только вне кулдауна; иначе — тормозим
				if e.Cooldown > 0 || !g.tryLaneChange(e, i) {
					e.Braking = true
					// не быстрее лидера, пока дистанция меньше желаемой
					if e.OwnSpeed > leadSpeed {
						e.OwnSpeed = leadSpeed
					}
					// при малом запасе пути гасим сближение на оставшемся
					// расстоянии: need — тормозной путь dc, который нужно
					// успеть погасить до safeGap
					if gap < safeGap*2 {
						margin := gap - safeGap
						need := 0.0
						if margin > 1 {
							need = dc * dc / margin
						}
						decel := followDecel + need
						if decel > hardDecel {
							decel = hardDecel
						}
						e.OwnSpeed -= decel * dt
					}
					if e.OwnSpeed < 0.05 {
						e.OwnSpeed = 0.05
					}
				} else {
					e.Braking = false
				}
			} else {
				e.Braking = false
				if e.OwnSpeed < e.BaseOwn {
					e.OwnSpeed += 0.02 * dt
					if e.OwnSpeed > e.BaseOwn {
						e.OwnSpeed = e.BaseOwn
					}
				}
				if e.Cooldown <= 0 && rand.Float64() < randomLaneChance*dt {
					g.tryLaneChange(e, i)
				}
			}
		} else {
			// впереди никого — едем как раньше
			e.Braking = false
			if e.OwnSpeed < e.BaseOwn {
				e.OwnSpeed += 0.02 * dt
				if e.OwnSpeed > e.BaseOwn {
					e.OwnSpeed = e.BaseOwn
				}
			}
			if rand.Float64() < randomLaneChance*dt {
				g.tryLaneChange(e, i)
			}
		}
	}

	// перемещение вниз.
	// Пол 0.4 действует только когда машина НЕ тормозит за лидером:
	// тормозящей машине пол запрещает стоять на месте —
	// именно он и вползал её в переднюю.
	for i := range g.enemies {
		e := &g.enemies[i]
		rel := eff - e.OwnSpeed
		if rel < 0.4 && !e.Braking {
			rel = 0.4
		}
		e.Y += rel * dt
	}

	// страховка от наездов — последняя линия обороны.
	// При нормальном следовании срабатывать не должна.
	for i := range g.enemies {
		a := &g.enemies[i]
		for j := range g.enemies {
			if i == j {
				continue
			}
			b := &g.enemies[j]
			if a.Y > b.Y || !xOverlap(*a, *b) {
				continue
			}
			if a.Y+a.H > b.Y {
				a.Y = b.Y - a.H
				if a.OwnSpeed > b.OwnSpeed {
					a.OwnSpeed = b.OwnSpeed
				}
				a.Braking = true
			}
		}
	}
}

// HitRect — единственный источник правды о прямоугольнике машины.
// X, Y — левый верхний угол (как их использует drawCar).
func (c Car) HitRect() (x, y, w, h float64) {
	return c.X, c.Y, c.W, c.H
}
