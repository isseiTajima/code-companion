package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"time"

	"sakura-kodama/internal/behavior"
	"sakura-kodama/internal/context"
	"sakura-kodama/internal/session"
	"sakura-kodama/internal/types"
)

func main() {
	logPath := flag.String("f", "", "Path to signal log file (.jsonl)")
	flag.Parse()

	if *logPath == "" {
		fmt.Println("Usage: contextviewer -f <path_to_jsonl>")
		os.Exit(1)
	}

	f, err := os.Open(*logPath)
	if err != nil {
		log.Fatalf("failed to open file: %v", err)
	}
	defer f.Close()

	// エンジン初期化
	est := contextengine.NewEstimator()
	beh := behavior.NewInferrer(5 * time.Minute)
	sess := session.NewTracker()

	fmt.Printf("%-25s | %-20s | %-15s | %-15s | %-5s | %s\n", "Timestamp", "Signal", "Context", "Session", "Conf", "Reason")
	fmt.Println("-----------------------------------------------------------------------------------------------------------------------------")

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		var sig types.Signal
		if err := json.Unmarshal([]byte(line), &sig); err != nil {
			continue
		}

		// Replay logic
		decision := est.ProcessSignal(sig)
		beh.AddSignal(sig)
		b := beh.Infer()
		ts := types.StrToTime(sig.Timestamp)
		s := sess.Update(b, ts)

		reason := ""
		if len(decision.Reasons) > 0 {
			reason = decision.Reasons[0]
		}

		fmt.Printf("%-25s | %-20s | %-15s | %-15s | %.2f | %s\n",
			ts.Format("2006-01-02 15:04:05"),
			string(sig.Type),
			string(decision.State),
			string(s.Mode),
			decision.Confidence,
			reason,
		)
	}

	if err := scanner.Err(); err != nil {
		log.Fatalf("scan error: %v", err)
	}
}
