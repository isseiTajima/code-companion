package monitor

import (
	"context"
	"fmt"
	"runtime"
	"testing"
	"time"

	"sakura-kodama/internal/config"
	"sakura-kodama/internal/types"
)

func TestLongRunningStability(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping long running stability test in short mode")
	}

	// モニターのセットアップ
	cfg := config.DefaultAppConfig()
	// テスト用にレコーダーは無効化（I/O負荷を避けるため）
	m, err := New(cfg, ".")
	if err != nil {
		t.Fatalf("failed to create monitor: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// モニターをバックグラウンドで実行
	go m.Run(ctx)

	// 初期リソース状態
	var startMem runtime.MemStats
	runtime.ReadMemStats(&startMem)
	startGoroutines := runtime.NumGoroutine()

	// シグナル生成ループ (1万件)
	const totalSignals = 10000
	
	// 高速再生（1万件を数秒で処理）
	// チャネルバッファが溢れない程度にウェイトを入れる
	const interval = 100 * time.Microsecond 

	fmt.Printf("Starting stability test with %d signals...\n", totalSignals)

	// イベント消費用ゴルーチン
	eventCount := 0
	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			select {
			case <-m.Events():
				eventCount++
			case <-ctx.Done():
				return
			}
		}
	}()

	start := time.Now()
	for i := 0; i < totalSignals; i++ {
		sig := types.Signal{
			Type:      types.SigFileModified,
			Source:    types.SourceFS,
			Value:     fmt.Sprintf("file_%d.go", i),
			Timestamp: types.TimeToStr(time.Now()),
		}
		m.InjectSignal(sig)
		
		if i%1000 == 0 {
			// 定期的にGitコミットなどを混ぜて状態遷移を誘発
			m.InjectSignal(types.Signal{
				Type:      types.SigGitCommit,
				Source:    types.SourceGit,
				Timestamp: types.TimeToStr(time.Now()),
			})
		}
		
		time.Sleep(interval)
	}
	duration := time.Since(start)

	// 少し待って処理完了を期待
	time.Sleep(1 * time.Second)

	// 終了後のリソース状態
	var endMem runtime.MemStats
	runtime.ReadMemStats(&endMem)
	endGoroutines := runtime.NumGoroutine()

	fmt.Printf("Processed %d signals in %v\n", totalSignals, duration)
	fmt.Printf("Events received: %d\n", eventCount)
	fmt.Printf("Goroutines: start=%d, end=%d\n", startGoroutines, endGoroutines)
	fmt.Printf("Memory: start=%d KB, end=%d KB\n", startMem.Alloc/1024, endMem.Alloc/1024)

	// Goroutineリーク判定（許容範囲内か）
	// テストランナー自体のオーバーヘッドもあるため、厳密な一致は難しいが、
	// 著しい増加（例えば +100以上）がないか確認
	if endGoroutines > startGoroutines+50 {
		t.Errorf("potential goroutine leak: increased by %d", endGoroutines-startGoroutines)
	}

	// Memoryリーク判定はGCのタイミングによるため参考程度だが、
	// 異常な増加（例えば100MB以上）がないか確認
	if endMem.Alloc > startMem.Alloc+100*1024*1024 {
		t.Errorf("potential memory leak: increased by %d KB", (endMem.Alloc-startMem.Alloc)/1024)
	}
}
