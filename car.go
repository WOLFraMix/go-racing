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

	playerSpeed = 6.0

	boostMax  = 2.2
	brakeMax  = 1.6
	boostGain = 0.05
	brakeGain = 0.07

	laneChangeSpeed = 1.3 // плавное перестроение
	safeGap         = 70
	changeCooldown  = 150

	randomLaneChance = 0.001
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
}

// randomSpeed — рандомизация скорости: 80–120% от базовой.
func randomSpeed(base float64) float64 {
	return base * (0.8 + rand.Float64()*0.4)
}

// tryLaneChange — попытка перестроения: случайная сторона, проверка
// свободности, поворотник и кулдаун. Возвращает true, если манёвр начат.
func (g *Game) tryLaneChange(e *Car, selfIdx int) bool {
	dir := 0
	if rand.Intn(2) == 0 {
		if g.laneFreeFor(*e, selfIdx, e.Lane-1) {
			dir = -1
		} else if g.laneFreeFor(*e, selfIdx, e.Lane+1) {
			dir = 1
		}
	} else {
		if g.laneFreeFor(*e, selfIdx, e.Lane+1) {
			dir = 1
		} else if g.laneFreeFor(*e, selfIdx, e.Lane-1) {
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
	case r < 0.12:
		kind = KindSport
		points = 2
		base = 2.1 + rand.Float64()*0.6
	case r < 0.26:
		kind = KindTruck
		w = float64(truckW)
		h = float64(truckH)
		points = 3
		base = 0.4 + rand.Float64()*0.4
	case r < 0.38:
		kind = KindMinibus
		points = 2
		base = 0.7 + rand.Float64()*0.3
	case r < 0.48:
		kind = KindBus
		h = float64(busH)
		points = 3
		base = 0.5 + rand.Float64()*0.3
	case r < 0.55:
		kind = KindAmbulance
		points = 4
		base = 1.9 + rand.Float64()*0.5
	case r < 0.67:
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

// updateEnemies — ИИ, перемещение и страховка от наездов. Все константы —
// «за кадр при 60 FPS», умножаются на dt.
func (g *Game) updateEnemies(eff, dt float64) {
	for i := range g.enemies {
		e := &g.enemies[i]

		// движение при перестроении
		if e.TargetLane != e.Lane {
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
			e.Braking = false
		} else if e.BlinkTimer > 0 {
			e.BlinkTimer -= dt
			if e.BlinkTimer <= 0 {
				e.Blink = 0
			}
		}
		if e.Cooldown > 0 {
			e.Cooldown -= dt
		}

		if e.TargetLane != e.Lane || e.Cooldown > 0 {
			continue
		}

		leaderIdx, gap, leaderIsPlayer := g.leaderInfo(*e)

		var leadSpeed float64
		if leaderIdx >= 0 {
			leadSpeed = g.enemies[leaderIdx].OwnSpeed
		} else if leaderIsPlayer {
			leadSpeed = eff
		}

		closeCall := (leaderIdx >= 0 || leaderIsPlayer) &&
			e.OwnSpeed > leadSpeed+0.05 &&
			gap < safeGap

		if closeCall {
			if !g.tryLaneChange(e, i) {
				e.Braking = true
				e.OwnSpeed -= 0.05 * dt
				if e.OwnSpeed < leadSpeed {
					e.OwnSpeed = leadSpeed
				}
				if e.OwnSpeed < 0.2 {
					e.OwnSpeed = 0.2
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
			if rand.Float64() < randomLaneChance*dt {
				g.tryLaneChange(e, i)
			}
		}
	}

	// перемещение вниз
	for i := range g.enemies {
		e := &g.enemies[i]
		rel := eff - e.OwnSpeed
		if rel < 0.4 {
			rel = 0.4
		}
		e.Y += rel * dt
	}

	// страховка от наездов
	for i := range g.enemies {
		for j := range g.enemies {
			if i == j {
				continue
			}
			a, b := g.enemies[i], g.enemies[j]
			if a.Y <= b.Y || !xOverlap(a, b) {
				continue
			}
			if a.Y < b.Y+b.H {
				g.enemies[i].Y = b.Y + b.H
				g.enemies[i].OwnSpeed = math.Min(g.enemies[i].OwnSpeed, b.OwnSpeed)
				g.enemies[i].Braking = true
			}
		}
	}
}
