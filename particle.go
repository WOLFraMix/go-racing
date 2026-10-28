package main

import (
	"image/color"
	"math"
	"math/rand"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/vector"
)

type Particle struct {
	X, Y, Vx, Vy, Life float64
	Col                color.RGBA
}

func (g *Game) updateParticles() {
	alive := g.particles[:0]
	for _, p := range g.particles {
		p.Life -= 0.02
		p.X += p.Vx
		p.Y += p.Vy
		if p.Life > 0 {
			alive = append(alive, p)
		}
	}
	g.particles = alive
}

func (g *Game) explode(cx, cy float64) {
	for i := 0; i < 40; i++ {
		a := rand.Float64() * 2 * math.Pi
		sp := rand.Float64()*4 + 1
		var col color.RGBA
		switch rand.Intn(3) {
		case 0:
			col = color.RGBA{R: 255, G: 180, B: 40, A: 255}
		case 1:
			col = color.RGBA{R: 255, G: 90, B: 40, A: 255}
		default:
			col = color.RGBA{R: 230, G: 230, B: 230, A: 255}
		}
		g.particles = append(g.particles, Particle{
			X: cx, Y: cy,
			Vx:   math.Cos(a) * sp,
			Vy:   math.Sin(a) * sp,
			Life: rand.Float64()*0.5 + 0.5,
			Col:  col,
		})
	}
}

func (g *Game) drawParticles(dst *ebiten.Image) {
	for _, p := range g.particles {
		a := p.Life
		if a > 1 {
			a = 1
		}
		col := p.Col
		col.A = uint8(255 * a)
		vector.FillCircle(dst, float32(p.X), float32(p.Y), 3, col, false)
	}
}
