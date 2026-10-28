package main

import "image/color"

type StageDef struct {
	Name        string
	Frames      float64 // длительность в игровых кадрах (60 FPS units)
	ScrollStart float64
	ScrollEnd   float64
	Night       bool
	Asphalt     color.RGBA
	Grass       color.RGBA
	SpawnEvery  float64 // базовый интервал спавна
}

var stageDefs = []StageDef{
	{
		Name: "CITY", Frames: 60 * 35, ScrollStart: 3.0, ScrollEnd: 4.3,
		Asphalt:    color.RGBA{R: 70, G: 72, B: 76, A: 255},
		Grass:      color.RGBA{R: 96, G: 128, B: 70, A: 255},
		SpawnEvery: 160,
	},
	{
		Name: "HIGHWAY", Frames: 60 * 35, ScrollStart: 4.4, ScrollEnd: 5.6,
		Asphalt:    color.RGBA{R: 58, G: 60, B: 66, A: 255},
		Grass:      color.RGBA{R: 110, G: 120, B: 84, A: 255},
		SpawnEvery: 150,
	},
	{
		Name: "NIGHT", Frames: 60 * 35, ScrollStart: 4.2, ScrollEnd: 6.2, Night: true,
		Asphalt:    color.RGBA{R: 40, G: 42, B: 52, A: 255},
		Grass:      color.RGBA{R: 46, G: 60, B: 44, A: 255},
		SpawnEvery: 150,
	},
}
