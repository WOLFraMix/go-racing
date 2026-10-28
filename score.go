package main

import (
	"encoding/json"
	"os"
)

type highScoreFile struct {
	HighScore int `json:"high_score"`
}

func (g *Game) readHighScore() {
	data, err := os.ReadFile("highscore.json")
	if err != nil {
		return
	}
	var hs highScoreFile
	if err := json.Unmarshal(data, &hs); err == nil {
		g.highScore = hs.HighScore
	}
}

func (g *Game) saveHighScore() {
	data, _ := json.Marshal(highScoreFile{HighScore: g.highScore})
	_ = os.WriteFile("highscore.json", data, 0o644)
}
