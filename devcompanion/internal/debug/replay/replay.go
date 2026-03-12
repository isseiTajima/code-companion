package replay

import (
	"bufio"
	"context"
	"encoding/json"
	"os"
	"time"

	"sakura-kodama/internal/types"
)

// Mode は再生モード。
type Mode int

const (
	ModeRealTime Mode = iota
	ModeFast
)

// Replayer はシグナルログを再生する。
type Replayer struct {
	path string
	mode Mode
}

// New は Replayer を作成する。
func New(path string, mode Mode) *Replayer {
	return &Replayer{
		path: path,
		mode: mode,
	}
}

// Run はシグナルをチャネルに送信する。
func (r *Replayer) Run(ctx context.Context, out chan<- types.Signal) error {
	f, err := os.Open(r.path)
	if err != nil {
		return err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	var lastTime time.Time

	for scanner.Scan() {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		line := scanner.Text()
		if line == "" {
			continue
		}

		var sig types.Signal
		if err := json.Unmarshal([]byte(line), &sig); err != nil {
			continue // パースエラーはスキップ
		}

		sigTime := types.StrToTime(sig.Timestamp)
		if r.mode == ModeRealTime && !lastTime.IsZero() {
			diff := sigTime.Sub(lastTime)
			if diff > 0 {
				select {
				case <-time.After(diff):
				case <-ctx.Done():
					return ctx.Err()
				}
			}
		}

		lastTime = sigTime
		// 現在時刻に書き換える（コンテキストエンジンが正しく動作するため）
		sig.Timestamp = types.TimeToStr(time.Now())

		select {
		case out <- sig:
		case <-ctx.Done():
			return ctx.Err()
		}
	}

	return scanner.Err()
}
