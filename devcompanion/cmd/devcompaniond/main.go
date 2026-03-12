package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"sakura-kodama/internal/config"
	contextengine "sakura-kodama/internal/context"
	"sakura-kodama/internal/engine"
	"sakura-kodama/internal/llm"
	"sakura-kodama/internal/monitor"
	"sakura-kodama/internal/observer"
	"sakura-kodama/internal/persona"
	"sakura-kodama/internal/profile"
	ws_transport "sakura-kodama/internal/transport/websocket"
	"sakura-kodama/internal/types"
	"sakura-kodama/internal/ws"
)

func main() {
	log.Println("[SERVER] Initializing Sakura Kodama Server Mode...")

	// 1. Config 読み込み
	appCfg, err := config.LoadConfig()
	if err != nil {
		log.Printf("Warning: failed to load config: %v. Using defaults.", err)
		appCfg = config.DefaultAppConfig()
	}

	// 2. 各モジュールの初期化
	mon, err := monitor.New(appCfg, ".")
	if err != nil {
		log.Fatalf("failed to init monitor: %v", err)
	}

	obs, err := observer.NewDevObserver(".")
	if err != nil {
		log.Printf("Warning: failed to init observer: %v", err)
	}

	cfgPath, _ := config.DefaultConfigPath()
	profilePath := filepath.Join(filepath.Dir(cfgPath), "dev_profile.json")
	ps, err := profile.NewProfileStore(profilePath)
	if err != nil {
		log.Printf("Warning: failed to init profile store: %v", err)
		ps = &profile.ProfileStore{}
	}

	speech := llm.NewSpeechGenerator(&appCfg.Config)
	wsServer := ws.NewServer()

	// 3. Transport Layer (WebSocket Only)
	wsn := ws_transport.NewWebSocketNotifier(wsServer)

	// 4. Engine の構築
	ce := contextengine.NewEstimator()
	pe := persona.NewPersonaEngine(types.StyleSoft)
	eng := engine.New(mon, ce, pe, speech, ps, obs, &appCfg.Config, wsn)

	// 5. 実行開始
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// WebSocket サーバーの起動 (ポート 34567 は internal/ws/server.go で定義)
	go func() {
		log.Printf("[SERVER] WebSocket Server starting on :%d", ws.WSPort)
		if err := wsServer.Start(); err != nil {
			log.Printf("WebSocket server error: %v", err)
		}
	}()

	// 各エンジンの起動
	go mon.Run(ctx)
	go eng.Run(ctx)
	go eng.StartupGreeting(ctx)

	log.Println("[SERVER] Sakura Kodama Server Mode is now running.")
	log.Println("[SERVER] Press Ctrl+C to stop.")

	<-ctx.Done()
	log.Println("[SERVER] Shutting down gracefully...")
}
