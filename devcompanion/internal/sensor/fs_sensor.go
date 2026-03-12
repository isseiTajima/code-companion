package sensor

import (
	"context"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"sakura-kodama/internal/types"
	"github.com/fsnotify/fsnotify"
)

type FSSensor struct {
	watchDir string
}

func NewFSSensor(watchDir string) *FSSensor {
	return &FSSensor{watchDir: watchDir}
}

func (s *FSSensor) Name() string {
	return "FSSensor"
}

func (s *FSSensor) Run(ctx context.Context, signals chan<- types.Signal) error {
	log.Printf("[SENSOR] Starting FSSensor on %s", s.watchDir)
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return err
	}
	defer watcher.Close()

	// 監視ディレクトリの追加
	_ = s.addWatchRecursive(watcher, s.watchDir)

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case event, ok := <-watcher.Events:
			if !ok {
				return nil
			}
			log.Printf("[DEBUG] FSSensor event: %v", event)
			
			// 新しいディレクトリが作成されたら監視対象に追加
			if event.Op&fsnotify.Create == fsnotify.Create {
				info, err := os.Stat(event.Name)
				if err == nil && info.IsDir() {
					_ = s.addWatchRecursive(watcher, event.Name)
				}
			}

			base := filepath.Base(event.Name)
			msg := ""
			switch {
			case strings.HasSuffix(base, "_test.go"):
				msg = "go test"
			case strings.HasSuffix(base, ".go"):
				msg = "generate"
			case base == "Makefile" || strings.HasSuffix(base, ".sh"):
				msg = "lint"
			}

			if event.Op&fsnotify.Write == fsnotify.Write || event.Op&fsnotify.Create == fsnotify.Create {
				signals <- types.Signal{
					Type:      types.SigFileModified,
					Source:    types.SourceFS,
					Value:     event.Name,
					Message:   msg,
					Timestamp: types.TimeToStr(time.Now()),
				}
			}
		case err, ok := <-watcher.Errors:
			if !ok {
				return nil
			}
			_ = err
		}
	}
}

func (s *FSSensor) addWatchRecursive(w *fsnotify.Watcher, root string) error {
	return filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if entry.IsDir() {
			if strings.Contains(path, "node_modules") || strings.Contains(path, ".git/objects") {
				return filepath.SkipDir
			}
			_ = w.Add(path)
		}
		return nil
	})
}
