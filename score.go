package main

import (
	"encoding/json"
	"os"
)

type highScoreFile struct {
	HighScore int `json:"high_score"`
}

type settingsFile struct {
	Skin       int  `json:"skin"`
	Stage      int  `json:"stage"`
	Muted      bool `json:"muted"`
	Difficulty int  `json:"difficulty"`
}

func (g *Game) readSettings() {
	data, err := os.ReadFile("settings.json")
	if err != nil {
		return
	}
	var st settingsFile
	if err := json.Unmarshal(data, &st); err == nil {
		g.difficulty = st.Difficulty
		g.skinIdx = st.Skin
		g.startStage = st.Stage
		g.muted = st.Muted
	}
}

func (g *Game) saveSettings() {
	data, _ := json.Marshal(settingsFile{Skin: g.skinIdx, Stage: g.startStage, Muted: g.muted, Difficulty: g.difficulty})
	_ = os.WriteFile("settings.json", data, 0o644)
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
