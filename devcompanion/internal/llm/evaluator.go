package llm

import (
	"context"
	"fmt"
	"log"
	"strconv"
	"strings"
	"time"
)

const evalKeepCount = 3 // 評価後にプールへ保持する上位件数

// evaluateCandidates はOllamaで候補セリフを評価し、上位evalKeepCount件の0-indexedインデックスを返す。
// 失敗した場合はnil（全件使用のフォールバック）を返す。
func (sg *SpeechGenerator) evaluateCandidates(ctx context.Context, candidates []string, recent []string, language string) []int {
	if len(candidates) <= evalKeepCount {
		idx := make([]int, len(candidates))
		for i := range idx {
			idx[i] = i
		}
		return idx
	}

	ollamaClient, ok := sg.router.ollama.(*OllamaClient)
	if !ok || ollamaClient == nil {
		return nil
	}

	prompt := buildEvalPrompt(candidates, recent, language)

	timeoutCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	text, err := ollamaClient.GenerateRaw(timeoutCtx, prompt)
	if err != nil {
		log.Printf("[EVAL] Evaluator request failed: %v", err)
		return nil
	}

	log.Printf("[EVAL] Raw response: %q", text)
	return parseEvalResponse(text, len(candidates))
}

func buildEvalPrompt(candidates []string, recent []string, language string) string {
	var sb strings.Builder

	if language == "en" {
		fmt.Fprintf(&sb, "Select the best %d lines for an AI companion character.\nPrefer lines that: (A) combine an observation with a genuine personal reaction (e.g. \"You've been at this for a while — honestly impressive\"), OR (B) are short but grounded in observation (e.g. \"Looking good\", \"Steady pace\").\nAvoid: generic filler with no observation behind it (e.g. \"I'm rooting for you\", \"Hope it works out\", \"Keep it up\").\nAlso avoid lines similar to recent ones, and prefer variety in tone.\nOutput ONLY numbers separated by space (e.g. \"1 3\"). No explanation.\n", evalKeepCount)
		if len(recent) > 0 {
			sb.WriteString("\nRecent lines (avoid similarity):\n")
			for _, r := range recent {
				sb.WriteString("- " + r + "\n")
			}
		}
		sb.WriteString("\nCandidates:\n")
		for i, c := range candidates {
			fmt.Fprintf(&sb, "%d. \"%s\"\n", i+1, c)
		}
		fmt.Fprintf(&sb, "\nBest %d numbers:", evalKeepCount)
	} else {
		fmt.Fprintf(&sb, "AIコンパニオンのセリフ候補から良い%d件を選んでください。\n優先①: 観察＋感情の2段構成（例:「さっきからずっと触ってますよね、すごいなって思ってました」「こんな時間までやってるの、普通にすごいです」）\n優先②: 短くても観察ベースの自然な励まし（例:「いいペースですね」「ちゃんと進んでますね」）\n避ける: 観察なし・誰にでも言える定型文（例:「応援しています」「うまくいくといいですね」「頑張ってください」）\nまた直近のセリフと被るものは避け、トーンにバリエーションをつけること。\n番号を空白区切りで答えるだけ（例: \"1 3\"）。説明不要。\n", evalKeepCount)
		if len(recent) > 0 {
			sb.WriteString("\n直近のセリフ（似た表現は避けること）:\n")
			for _, r := range recent {
				sb.WriteString("- " + r + "\n")
			}
		}
		sb.WriteString("\n候補:\n")
		for i, c := range candidates {
			fmt.Fprintf(&sb, "%d. 「%s」\n", i+1, c)
		}
		fmt.Fprintf(&sb, "\n良い%d件の番号のみ:", evalKeepCount)
	}

	return sb.String()
}

func parseEvalResponse(text string, maxIdx int) []int {
	// 最初の行だけを使う（モデルが説明を続けた場合の対策）
	if nl := strings.IndexByte(text, '\n'); nl >= 0 {
		text = text[:nl]
	}
	text = strings.TrimSpace(text)

	seen := make(map[int]bool)
	var result []int
	for _, token := range strings.Fields(text) {
		clean := strings.Trim(token, ".,、。「」()（）[]")
		n, err := strconv.Atoi(clean)
		if err != nil || n < 1 || n > maxIdx || seen[n] {
			continue
		}
		seen[n] = true
		result = append(result, n-1) // 0-indexed に変換
		if len(result) >= evalKeepCount {
			break
		}
	}

	if len(result) == 0 {
		log.Printf("[EVAL] Parse failed for response: %q", text)
		return nil
	}
	return result
}
