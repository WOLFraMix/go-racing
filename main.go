package main

import "github.com/hajimehoshi/ebiten/v2"

func main() {
	ebiten.SetWindowSize(screenWidth, screenHeight+40)
	ebiten.SetWindowTitle("Go Racing Mini — Ebitengine")

	g := NewGame()
	if err := ebiten.RunGame(g); err != nil {
		panic(err)
	}
}
