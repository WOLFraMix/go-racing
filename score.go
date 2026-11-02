package main

import (
	"encoding/json"
	"log"
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
	data, err := json.Marshal(settingsFile{Skin: g.skinIdx, Stage: g.startStage, Muted: g.muted, Difficulty: g.difficulty})
	if err != nil {
		log.Printf("go-racing: marshal settings: %v", err)
		return
	}
	if err := os.WriteFile("settings.json", data, 0o644); err != nil {
		log.Printf("go-racing: save settings: %v", err)
	}
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
	data, err := json.Marshal(highScoreFile{HighScore: g.highScore})
	if err != nil {
		log.Printf("go-racing: marshal highscore: %v", err)
		return
	}
	if err := os.WriteFile("highscore.json", data, 0o644); err != nil {
		log.Printf("go-racing: save highscore: %v", err)
	}
}
