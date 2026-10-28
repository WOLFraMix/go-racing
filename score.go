package main

import (
	"encoding/json"
	"os"
)

type highScoreFile struct {
	HighScore int `json:"high_score"`
}

type settingsFile struct {
	Skin  int  `json:"skin"`
	Muted bool `json:"muted"`
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

func (g *Game) readSettings() {
	data, err := os.ReadFile("settings.json")
	if err != nil {
		return
	}
	var st settingsFile
	if err := json.Unmarshal(data, &st); err == nil {
		g.skinIdx = st.Skin
		g.muted = st.Muted
	}
}

func (g *Game) saveSettings() {
	data, _ := json.Marshal(settingsFile{Skin: g.skinIdx, Muted: g.muted})
	_ = os.WriteFile("settings.json", data, 0o644)
}
