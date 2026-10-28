package main

import "github.com/hajimehoshi/ebiten/v2"

func main() {
	ebiten.SetWindowSize(screenWidth, screenHeight+40) // +40 под рамку окна ОС
	ebiten.SetWindowTitle("Go Racing Mini — Ebitengine")

	g := NewGame()
	if err := ebiten.RunGame(g); err != nil {
		panic(err)
	}
}
