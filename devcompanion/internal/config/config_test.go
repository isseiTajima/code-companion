package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadConfig_ReturnsDefaultWhenNoFile(t *testing.T) {
	// ユーザーホームを一時ディレクトリに変更してテスト
	tmpHome := t.TempDir()
	origHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpHome)
	defer os.Setenv("HOME", origHome)

	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}

	if cfg.Name != "さくら" {
		t.Errorf("expected default name さくら, got %s", cfg.Name)
	}
}

func TestLoadConfig_LoadsYaml(t *testing.T) {
	tmpHome := t.TempDir()
	origHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpHome)
	defer os.Setenv("HOME", origHome)

	path, _ := DefaultConfigPath()
	dir := filepath.Dir(path)
	_ = os.MkdirAll(dir, 0755)

	yamlData := `
idle_timeout: 123
persona_style: energetic
name: CustomName
`
	_ = os.WriteFile(path, []byte(yamlData), 0644)

	cfg, err := LoadConfig()

	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}

	if cfg.IdleTimeout != 123 {
		t.Errorf("expected 123, got %d", cfg.IdleTimeout)
	}
	if cfg.PersonaStyle != "energetic" {
		t.Errorf("expected energetic, got %s", cfg.PersonaStyle)
	}
	if cfg.Name != "CustomName" {
		t.Errorf("expected CustomName, got %s", cfg.Name)
	}
}
