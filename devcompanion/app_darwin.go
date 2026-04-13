//go:build darwin
package main

/*
#cgo LDFLAGS: -framework Cocoa
#include <stdbool.h>
#include <stdlib.h>

void SetupNativeTray(const char* iconPath, const char* settingsLabel, const char* quitLabel);
void SetWindowFullInteractive(bool full);
void UpdateInteractiveRectsNative(double* rects, int count);
void ShowTalkButtonPanel(bool isTop, const char* title);
void HideTalkButtonPanel();
void RepositionTalkButtonPanel(bool isTop);
*/
import "C"
import (
	"os"
	"path/filepath"
	"strings"
	"unsafe"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

var globalAppInstance *App

//export goOnTraySettingsClicked
func goOnTraySettingsClicked() {
	if globalAppInstance != nil && globalAppInstance.ctx != nil {
		go func() {
			runtime.EventsEmit(globalAppInstance.ctx, "open-settings")
			runtime.WindowShow(globalAppInstance.ctx)
		}()
	}
}

//export goOnTrayQuitClicked
func goOnTrayQuitClicked() {
	if globalAppInstance != nil && globalAppInstance.ctx != nil {
		go func() {
			runtime.Quit(globalAppInstance.ctx)
		}()
	}
}

//export goOnTalkButtonClicked
func goOnTalkButtonClicked() {
	if globalAppInstance != nil && globalAppInstance.engine != nil {
		go globalAppInstance.engine.OnUserClick()
	}
}

func (a *App) isTopWindowPosition() bool {
	cfg := a.currentConfig()
	return !strings.HasPrefix(string(cfg.WindowPosition), "bottom")
}

func (a *App) talkButtonTitle() string {
	cfg := a.currentConfig()
	if cfg.Language == "en" {
		return "💬 Talk"
	}
	return "💬 話して"
}

func (a *App) showTalkButton() {
	title := C.CString(a.talkButtonTitle())
	defer C.free(unsafe.Pointer(title))
	C.ShowTalkButtonPanel(C.bool(a.isTopWindowPosition()), title)
}

func (a *App) hideTalkButton() {
	C.HideTalkButtonPanel()
}

func (a *App) repositionTalkButton() {
	C.RepositionTalkButtonPanel(C.bool(a.isTopWindowPosition()))
}

// setInteractiveShapeNative: 設定/オンボ時はウィンドウ全体をインタラクティブに切り替える。
// 通常時はフロントエンドから届いた rects に基づきObjC側で管理。
func (a *App) setInteractiveShapeNative(rects []float64, fullWindow bool) {
	if fullWindow || len(rects) == 0 {
		C.SetWindowFullInteractive(C.bool(fullWindow))
		return
	}
	cArr := make([]C.double, len(rects))
	for i, v := range rects {
		cArr[i] = C.double(v)
	}
	C.UpdateInteractiveRectsNative(&cArr[0], C.int(len(rects)/4))
}

func (a *App) setupNativeTray() {
	globalAppInstance = a

	iconPath := ""
	if len(a.icon) > 0 {
		tmp := filepath.Join(os.TempDir(), "sakura-tray-icon.png")
		if err := os.WriteFile(tmp, a.icon, 0644); err == nil {
			iconPath = tmp
		}
	}

	cfg := a.currentConfig()
	settingsLabel := "設定を開く"
	quitLabel := "終了"
	if cfg.Language == "en" {
		settingsLabel = "Open Settings"
		quitLabel = "Quit"
	}

	cPath := C.CString(iconPath)
	cSettings := C.CString(settingsLabel)
	cQuit := C.CString(quitLabel)
	defer C.free(unsafe.Pointer(cPath))
	defer C.free(unsafe.Pointer(cSettings))
	defer C.free(unsafe.Pointer(cQuit))

	C.SetupNativeTray(cPath, cSettings, cQuit)
}
