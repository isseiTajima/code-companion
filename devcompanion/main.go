package main

import (
	"context"
	"log"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/mac"

	"devcompanion/internal/config"
	"devcompanion/internal/llm"
	"devcompanion/internal/monitor"
	"devcompanion/internal/ws"
)

func main() {
	cfgPath, err := config.DefaultConfigPath()
	if err != nil {
		log.Fatalf("config path: %v", err)
	}
	cfg, err := config.Load(cfgPath)
	if err != nil {
		log.Printf("config load error (using default): %v", err)
		cfg = config.DefaultConfig()
	}

	mon, err := monitor.New(cfg, ".")
	if err != nil {
		log.Fatalf("monitor init: %v", err)
	}

	wsServer := ws.NewServer()
	speechGen := llm.NewSpeechGenerator(cfg.Model)
	app := NewApp(cfg, speechGen, wsServer)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go mon.Run(ctx)
	go func() {
		if err := wsServer.Start(); err != nil {
			log.Printf("websocket server: %v", err)
		}
	}()
	go pipeline(ctx, mon.Events(), speechGen, wsServer, cfg)

	if err := wails.Run(&options.App{
		Title:            "DevCompanion",
		Width:            200,
		Height:           300,
		Frameless:        true,
		BackgroundColour: &options.RGBA{R: 0, G: 0, B: 0, A: 0},
		AlwaysOnTop:      cfg.AlwaysOnTop,
		OnStartup:        app.startup,
		Bind:             []interface{}{app},
		Mac: &mac.Options{
			WebviewIsTransparent: true,
			WindowIsTranslucent:  true,
		},
	}); err != nil {
		log.Fatalf("wails run: %v", err)
	}
}

func pipeline(
	ctx context.Context,
	events <-chan monitor.MonitorEvent,
	speech *llm.SpeechGenerator,
	wsServer *ws.Server,
	cfg *config.Config,
) {
	for {
		select {
		case e := <-events:
			reason := reasonFromEvent(e)
			text := speech.Generate(e, cfg, reason)
			wsServer.Broadcast(ws.Event{
				State:  string(e.State),
				Task:   string(e.Task),
				Mood:   string(e.Mood),
				Speech: text,
			})
		case <-ctx.Done():
			return
		}
	}
}

func reasonFromEvent(e monitor.MonitorEvent) llm.Reason {
	switch e.State {
	case monitor.StateSuccess:
		return llm.ReasonSuccess
	case monitor.StateFail:
		return llm.ReasonFail
	case monitor.StateThinking:
		return llm.ReasonThinkingTick
	default:
		return llm.ReasonThinkingTick
	}
}
