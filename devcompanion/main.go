package main

import (
	"embed"
	"log"
	"path/filepath"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/menu"
	"github.com/wailsapp/wails/v2/pkg/menu/keys"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/mac"
	wailsRuntime "github.com/wailsapp/wails/v2/pkg/runtime"

	"sakura-kodama/internal/config"
	"sakura-kodama/internal/llm"
	"sakura-kodama/internal/monitor"
	"sakura-kodama/internal/observer"
	"sakura-kodama/internal/profile"
	"sakura-kodama/internal/ws"
)

//go:embed all:frontend/dist
var assets embed.FS

//go:embed frontend/src/assets/chara.png
var icon []byte

func main() {
	// 1. Config 読み込み
	appCfg, err := config.LoadConfig()
	if err != nil {
		log.Printf("Warning: failed to load config: %v", err)
	}

	width, height := ScaledDimensions(appCfg.Scale)

	// 2. モジュール初期化
	mon, err := monitor.New(appCfg, ".")
	if err != nil {
		log.Fatalf("monitor init: %v", err)
	}

	cfgPath, _ := config.DefaultConfigPath()
	profilePath := filepath.Join(filepath.Dir(cfgPath), "dev_profile.json")
	profileStore, _ := profile.NewProfileStore(profilePath)
	if profileStore == nil {
		profileStore = &profile.ProfileStore{} 
	}

	devObserver, _ := observer.NewDevObserver(".")
	wsServer := ws.NewServer()
	go func() {
		if err := wsServer.Start(); err != nil {
			log.Printf("WS server error: %v", err)
		}
	}()
	speechGen := llm.NewSpeechGenerator(&appCfg.Config)
	
	app := NewApp(&appCfg.Config, speechGen, wsServer, profileStore, assets, icon, devObserver)
	app.SetMonitor(mon)

	// 3. アプリケーションメニュー（左上）の作成
	appMenu := menu.NewMenu()
	appMenu.Append(menu.AppMenu())
	settingsMenu := appMenu.AddSubmenu("設定")
	settingsMenu.AddText("設定を開く", keys.CmdOrCtrl(","), func(_ *menu.CallbackData) {
		wailsRuntime.EventsEmit(app.ctx, "open-settings")
	})
	appMenu.Append(menu.EditMenu())
	appMenu.Append(menu.WindowMenu())

	// 4. Wails 実行
	if err := wails.Run(&options.App{
		Title:            "Sakura Kodama",
		Width:            width,
		Height:           height,
		Frameless:        true,
		DisableResize:    true,
		BackgroundColour: &options.RGBA{R: 0, G: 0, B: 0, A: 0}, // 完全に透明
		AlwaysOnTop:      appCfg.AlwaysOnTop,
		OnStartup:        app.startup,
		Bind:             []interface{}{app},
		Menu:             appMenu,
		Assets:           assets,
		Mac: &mac.Options{
			TitleBar: &mac.TitleBar{
				TitlebarAppearsTransparent: true,
				HideTitle:                  true,
				HideTitleBar:               true,
			},
			WebviewIsTransparent: true,
			WindowIsTranslucent:  false,
		},
	}); err != nil {
		log.Fatalf("wails run: %v", err)
	}
}
