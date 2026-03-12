package memory

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"sakura-kodama/internal/config"
)

type WorkMemory struct {
	Project      string
	RecentAreas  []string
	LastActivity string
}

func (m *WorkMemory) String() string {
	if m.Project == "" && len(m.RecentAreas) == 0 && m.LastActivity == "" {
		return ""
	}
	var sb strings.Builder
	sb.WriteString("Recent Work Memory:\n")
	if m.Project != "" {
		sb.WriteString(fmt.Sprintf("Project: %s\n", m.Project))
	}
	if len(m.RecentAreas) > 0 {
		sb.WriteString(fmt.Sprintf("Frequent Work Areas: %s\n", strings.Join(m.RecentAreas, ", ")))
	}
	if m.LastActivity != "" {
		sb.WriteString(fmt.Sprintf("Last Activity: %s\n", m.LastActivity))
	}
	return sb.String()
}

// BuildMemory は直近の DEVELOPER_LOG.jsonl から作業メモリを構築する
func BuildMemory() (*WorkMemory, error) {
	cfgPath, err := config.DefaultConfigPath()
	if err != nil {
		return nil, err
	}
	logPath := filepath.Join(filepath.Dir(cfgPath), "DEVELOPER_LOG.jsonl")
	
	file, err := os.Open(logPath)
	if err != nil {
		if os.IsNotExist(err) {
			return &WorkMemory{}, nil
		}
		return nil, err
	}
	defer file.Close()

	// 直近 50 行程度を解析
	var lines []string
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
		if len(lines) > 50 {
			lines = lines[1:]
		}
	}

	mem := &WorkMemory{
		Project: "Sakura Kodama", // デフォルト。将来的には git remote などから取得
	}

	areaCounts := make(map[string]int)

	for _, line := range lines {
		var entry map[string]interface{}
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			continue
		}

		// コンテキストから作業領域を推測
		if ctx, ok := entry["context"].(map[string]interface{}); ok {
			task, _ := ctx["task"].(string)
			if area := guessArea(task); area != "" {
				areaCounts[area]++
			}
		}
		
		// 最後の有意義な発言から活動を記録
		if speech, ok := entry["speech"].(string); ok && speech != "" {
			mem.LastActivity = speech
		}
	}

	// 頻出エリア Top 2
	mem.RecentAreas = getTopAreas(areaCounts, 2)
	
	// LastActivity をセリフではなく「何をしていたか」に要約（簡易版）
	// 本来は解析が必要だが、ここでは「前回話した内容」として扱う
	if len(mem.LastActivity) > 30 {
		runes := []rune(mem.LastActivity)
		mem.LastActivity = string(runes[:30]) + "..."
	}

	return mem, nil
}

func guessArea(task string) string {
	t := strings.ToLower(task)
	switch {
	case strings.Contains(t, "ui") || strings.Contains(t, "svelte") || strings.Contains(t, "css"):
		return "UI"
	case strings.Contains(t, "animation") || strings.Contains(t, "chara"):
		return "Sakura animation"
	case strings.Contains(t, "llm") || strings.Contains(t, "prompt") || strings.Contains(t, "speech"):
		return "Sakura backend"
	case strings.Contains(t, "build") || strings.Contains(t, "make"):
		return "Build system"
	case strings.Contains(t, "doc") || strings.Contains(t, "readme"):
		return "Documentation"
	default:
		return ""
	}
}

func getTopAreas(counts map[string]int, n int) []string {
	var areas []string
	for i := 0; i < n; i++ {
		max := -1
		top := ""
		for area, count := range counts {
			if count > max {
				// すでに選ばれたものはスキップ
				alreadySelected := false
				for _, a := range areas {
					if a == area {
						alreadySelected = true
						break
					}
				}
				if !alreadySelected {
					max = count
					top = area
				}
			}
		}
		if top != "" {
			areas = append(areas, top)
		}
	}
	return areas
}
