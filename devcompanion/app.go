package main

import (
	"context"

	"devcompanion/internal/config"
	"devcompanion/internal/llm"
	"devcompanion/internal/ws"
)

// App はWailsバインディングを公開するアプリケーション構造体。
type App struct {
	ctx    context.Context
	speech *llm.SpeechGenerator
	ws     *ws.Server
	cfg    *config.Config
}

// NewApp は App を初期化する。
func NewApp(cfg *config.Config, speech *llm.SpeechGenerator, wsServer *ws.Server) *App {
	return &App{
		speech: speech,
		ws:     wsServer,
		cfg:    cfg,
	}
}

// startup はWailsランタイムからアプリ起動時に呼ばれる。
func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
}

// LoadConfig は現在の設定を返す（Wailsバインディング）。
func (a *App) LoadConfig() config.Config {
	return *a.cfg
}

// SaveConfig は設定を保存する（Wailsバインディング）。
func (a *App) SaveConfig(cfg config.Config) error {
	a.cfg = &cfg
	path, err := config.DefaultConfigPath()
	if err != nil {
		return err
	}
	return config.Save(&cfg, path)
}

// OnCharaClick はキャラクリック時にセリフを生成してWebSocketへ送信する（Wailsバインディング）。
func (a *App) OnCharaClick() {
	speech := a.speech.OnUserClick(a.cfg)
	a.ws.Broadcast(ws.Event{
		State:  string(a.cfg.Tone),
		Speech: speech,
	})
}
