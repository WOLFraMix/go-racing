package main

import (
	"math"
	"os"
	"path/filepath"
	"strings"
)

// loadSkinPack — читает все PNG из папки пака как варианты скина.
// Папки нет или она пуста — единственный вариант: базовый спрайт.
// Хочешь добавить скин — положи PNG в папку. Хочешь убрать — удали.
// Размер картинки лучше держать как у базового спрайта: её растянут
// под размеры машины.
func loadSkinPack(dir string, base Sprite) []Sprite {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return []Sprite{base}
	}
	var list []Sprite
	for _, en := range entries {
		if en.IsDir() || !strings.EqualFold(filepath.Ext(en.Name()), ".png") {
			continue
		}
		list = append(list, loadSprite(filepath.Join(dir, en.Name()), base.Fill))
	}
	if len(list) == 0 {
		return []Sprite{base}
	}
	return list
}

// clamp01 — общая утилита, используется в scenery.go
func clamp01(v float64) float64 { return math.Max(0, math.Min(1, v)) }
