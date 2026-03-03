package config

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
)

// Config はアプリケーションの設定を保持する。
type Config struct {
	Name          string `json:"name"`
	Tone          string `json:"tone"`
	EncourageFreq string `json:"encourage_freq"`
	Monologue     bool   `json:"monologue"`
	AlwaysOnTop   bool   `json:"always_on_top"`
	Mute          bool   `json:"mute"`
	Model         string `json:"model"`
}

// DefaultConfig はデフォルト設定を返す。
func DefaultConfig() *Config {
	return &Config{
		Name:          "チビちゃん",
		Tone:          "genki",
		EncourageFreq: "mid",
		Monologue:     true,
		AlwaysOnTop:   true,
		Mute:          false,
		Model:         "gemma3:4b",
	}
}

// Load は path からJSON設定を読み込む。
// ファイルが存在しない場合はデフォルト値を作成・保存して返す。
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			cfg := DefaultConfig()
			if saveErr := Save(cfg, path); saveErr != nil {
				return nil, saveErr
			}
			return cfg, nil
		}
		return nil, err
	}

	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}

// Save は cfg を path へJSON形式で書き込む。
// 必要なディレクトリが存在しない場合は作成する。
func Save(cfg *Config, path string) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}

// DefaultConfigPath は設定ファイルの標準パスを返す。
func DefaultConfigPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".devcompanion", "config.json"), nil
}
