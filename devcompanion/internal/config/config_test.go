package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// --- Load: ファイル未存在時のデフォルト生成 ---

func TestLoad_FileNotExist_ReturnsDefault(t *testing.T) {
	// Given: 存在しないパス
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "config.json")

	// When: Load を呼び出す
	cfg, err := Load(path)

	// Then: エラーなし・デフォルト値が返る
	if err != nil {
		t.Fatalf("want nil error, got %v", err)
	}
	if cfg == nil {
		t.Fatal("want non-nil config, got nil")
	}
}

func TestLoad_FileNotExist_SavesDefaultFile(t *testing.T) {
	// Given: 存在しないパス
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "config.json")

	// When: Load を呼び出す
	_, err := Load(path)
	if err != nil {
		t.Fatalf("want nil error, got %v", err)
	}

	// Then: デフォルト設定ファイルが作成される
	if _, statErr := os.Stat(path); os.IsNotExist(statErr) {
		t.Error("want config file created, but it does not exist")
	}
}

func TestLoad_FileNotExist_DefaultName(t *testing.T) {
	// Given: 存在しないパス
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "config.json")

	// When: Load を呼び出す
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("want nil error, got %v", err)
	}

	// Then: デフォルトの Name が "チビちゃん"
	if cfg.Name != "チビちゃん" {
		t.Errorf("want Name=%q, got %q", "チビちゃん", cfg.Name)
	}
}

func TestLoad_FileNotExist_DefaultTone(t *testing.T) {
	// Given: 存在しないパス
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "config.json")

	// When: Load を呼び出す
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("want nil error, got %v", err)
	}

	// Then: デフォルトの Tone が "genki"
	if cfg.Tone != "genki" {
		t.Errorf("want Tone=%q, got %q", "genki", cfg.Tone)
	}
}

func TestLoad_FileNotExist_DefaultModel(t *testing.T) {
	// Given: 存在しないパス
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "config.json")

	// When: Load を呼び出す
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("want nil error, got %v", err)
	}

	// Then: デフォルトの Model が "gemma3:4b"
	if cfg.Model != "gemma3:4b" {
		t.Errorf("want Model=%q, got %q", "gemma3:4b", cfg.Model)
	}
}

func TestLoad_FileNotExist_DefaultMuteIsFalse(t *testing.T) {
	// Given: 存在しないパス
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "config.json")

	// When: Load を呼び出す
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("want nil error, got %v", err)
	}

	// Then: Mute はデフォルト false
	if cfg.Mute {
		t.Error("want Mute=false by default, got true")
	}
}

// --- Load: 正常ファイルの読み込み ---

func TestLoad_ValidFile_LoadsCorrectly(t *testing.T) {
	// Given: 有効な JSON 設定ファイル
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "config.json")

	expected := Config{
		Name:          "テスト太郎",
		Tone:          "calm",
		EncourageFreq: "high",
		Monologue:     false,
		AlwaysOnTop:   false,
		Mute:          true,
		Model:         "qwen2.5:3b",
	}

	data, err := json.Marshal(expected)
	if err != nil {
		t.Fatalf("failed to marshal test config: %v", err)
	}
	if err := os.WriteFile(path, data, 0644); err != nil {
		t.Fatalf("failed to write test config: %v", err)
	}

	// When: Load を呼び出す
	cfg, err := Load(path)

	// Then: エラーなし・期待値と一致
	if err != nil {
		t.Fatalf("want nil error, got %v", err)
	}
	if cfg.Name != expected.Name {
		t.Errorf("Name: want %q, got %q", expected.Name, cfg.Name)
	}
	if cfg.Tone != expected.Tone {
		t.Errorf("Tone: want %q, got %q", expected.Tone, cfg.Tone)
	}
	if cfg.Mute != expected.Mute {
		t.Errorf("Mute: want %v, got %v", expected.Mute, cfg.Mute)
	}
	if cfg.Model != expected.Model {
		t.Errorf("Model: want %q, got %q", expected.Model, cfg.Model)
	}
}

// --- Load: 不正 JSON のエラーハンドリング ---

func TestLoad_InvalidJSON_ReturnsError(t *testing.T) {
	// Given: 不正な JSON ファイル
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "config.json")

	if err := os.WriteFile(path, []byte("{invalid json}"), 0644); err != nil {
		t.Fatalf("failed to write invalid config: %v", err)
	}

	// When: Load を呼び出す
	cfg, err := Load(path)

	// Then: エラーが返る・nil config
	if err == nil {
		t.Error("want error for invalid JSON, got nil")
	}
	if cfg != nil {
		t.Errorf("want nil config for invalid JSON, got %+v", cfg)
	}
}

func TestLoad_EmptyFile_ReturnsError(t *testing.T) {
	// Given: 空のファイル（有効な JSON ではない）
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "config.json")

	if err := os.WriteFile(path, []byte(""), 0644); err != nil {
		t.Fatalf("failed to write empty file: %v", err)
	}

	// When: Load を呼び出す
	cfg, err := Load(path)

	// Then: エラーが返る
	if err == nil {
		t.Error("want error for empty file, got nil")
	}
	if cfg != nil {
		t.Errorf("want nil config for empty file, got %+v", cfg)
	}
}

// --- Save: JSON 書き込み ---

func TestSave_WritesValidJSON(t *testing.T) {
	// Given: 設定値と書き込みパス
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "config.json")

	cfg := &Config{
		Name:          "テスト花子",
		Tone:          "polite",
		EncourageFreq: "mid",
		Monologue:     true,
		AlwaysOnTop:   true,
		Mute:          false,
		Model:         "gemma3:4b",
	}

	// When: Save を呼び出す
	err := Save(cfg, path)

	// Then: エラーなし
	if err != nil {
		t.Fatalf("want nil error, got %v", err)
	}

	// ファイルが存在する
	data, readErr := os.ReadFile(path)
	if readErr != nil {
		t.Fatalf("want readable file, got %v", readErr)
	}

	// 有効な JSON として parse できる
	var loaded Config
	if jsonErr := json.Unmarshal(data, &loaded); jsonErr != nil {
		t.Fatalf("want valid JSON, got %v", jsonErr)
	}

	// 値が一致する
	if loaded.Name != cfg.Name {
		t.Errorf("Name: want %q, got %q", cfg.Name, loaded.Name)
	}
	if loaded.Tone != cfg.Tone {
		t.Errorf("Tone: want %q, got %q", cfg.Tone, loaded.Tone)
	}
	if loaded.Mute != cfg.Mute {
		t.Errorf("Mute: want %v, got %v", cfg.Mute, loaded.Mute)
	}
}

func TestSave_CreatesDirectoryIfNeeded(t *testing.T) {
	// Given: 存在しないサブディレクトリへのパス
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "subdir", "nested", "config.json")

	cfg := &Config{
		Name:  "テスト",
		Tone:  "genki",
		Model: "gemma3:4b",
	}

	// When: Save を呼び出す
	err := Save(cfg, path)

	// Then: エラーなし・ディレクトリが作成されている
	if err != nil {
		t.Fatalf("want nil error, got %v", err)
	}
	if _, statErr := os.Stat(path); os.IsNotExist(statErr) {
		t.Error("want config file created in nested dir, but it does not exist")
	}
}

func TestSave_OverwritesExistingFile(t *testing.T) {
	// Given: 既存のファイル
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "config.json")

	// 最初の保存
	original := &Config{Name: "最初", Tone: "genki", Model: "gemma3:4b"}
	if err := Save(original, path); err != nil {
		t.Fatalf("first Save failed: %v", err)
	}

	// When: 別の設定で上書き
	updated := &Config{Name: "更新後", Tone: "calm", Model: "qwen2.5:3b"}
	if err := Save(updated, path); err != nil {
		t.Fatalf("second Save failed: %v", err)
	}

	// Then: 上書きされた値が読み込まれる
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load after overwrite failed: %v", err)
	}
	if cfg.Name != "更新後" {
		t.Errorf("want Name=%q after overwrite, got %q", "更新後", cfg.Name)
	}
	if cfg.Model != "qwen2.5:3b" {
		t.Errorf("want Model=%q after overwrite, got %q", "qwen2.5:3b", cfg.Model)
	}
}

// --- DefaultConfig: デフォルト値の検証 ---

func TestDefaultConfig_AllFieldsSet(t *testing.T) {
	// When: デフォルト設定を取得
	cfg := DefaultConfig()

	// Then: nil でない
	if cfg == nil {
		t.Fatal("want non-nil DefaultConfig")
	}

	// Then: 各フィールドが期待値
	if cfg.Name != "チビちゃん" {
		t.Errorf("Name: want %q, got %q", "チビちゃん", cfg.Name)
	}
	if cfg.Tone != "genki" {
		t.Errorf("Tone: want %q, got %q", "genki", cfg.Tone)
	}
	if cfg.EncourageFreq != "mid" {
		t.Errorf("EncourageFreq: want %q, got %q", "mid", cfg.EncourageFreq)
	}
	if !cfg.Monologue {
		t.Error("Monologue: want true, got false")
	}
	if !cfg.AlwaysOnTop {
		t.Error("AlwaysOnTop: want true, got false")
	}
	if cfg.Mute {
		t.Error("Mute: want false, got true")
	}
	if cfg.Model != "gemma3:4b" {
		t.Errorf("Model: want %q, got %q", "gemma3:4b", cfg.Model)
	}
}

// --- Load → Save → Load ラウンドトリップ ---

func TestLoadSave_RoundTrip(t *testing.T) {
	// Given: 設定をファイルに保存
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "config.json")

	original := &Config{
		Name:          "往復テスト",
		Tone:          "tsundere",
		EncourageFreq: "low",
		Monologue:     false,
		AlwaysOnTop:   false,
		Mute:          true,
		Model:         "gemma3:4b",
	}

	if err := Save(original, path); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	// When: Load で読み戻す
	loaded, err := Load(path)

	// Then: 全フィールドが一致する
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if loaded.Name != original.Name {
		t.Errorf("Name: want %q, got %q", original.Name, loaded.Name)
	}
	if loaded.Tone != original.Tone {
		t.Errorf("Tone: want %q, got %q", original.Tone, loaded.Tone)
	}
	if loaded.EncourageFreq != original.EncourageFreq {
		t.Errorf("EncourageFreq: want %q, got %q", original.EncourageFreq, loaded.EncourageFreq)
	}
	if loaded.Monologue != original.Monologue {
		t.Errorf("Monologue: want %v, got %v", original.Monologue, loaded.Monologue)
	}
	if loaded.AlwaysOnTop != original.AlwaysOnTop {
		t.Errorf("AlwaysOnTop: want %v, got %v", original.AlwaysOnTop, loaded.AlwaysOnTop)
	}
	if loaded.Mute != original.Mute {
		t.Errorf("Mute: want %v, got %v", original.Mute, loaded.Mute)
	}
	if loaded.Model != original.Model {
		t.Errorf("Model: want %q, got %q", original.Model, loaded.Model)
	}
}
