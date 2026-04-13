package weather

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"
)

const maxHistoryDays = 31

// DailyRecord は1日の天気記録。
type DailyRecord struct {
	Date  string `json:"date"`   // "2006-01-02"
	TempC int    `json:"temp_c"`
	Desc  string `json:"desc"`
}

type weatherHistory struct {
	Records []DailyRecord `json:"records"`
}

func historyPath(dataDir string) string {
	return filepath.Join(dataDir, "weather_history.json")
}

func loadHistory(dataDir string) []DailyRecord {
	data, err := os.ReadFile(historyPath(dataDir))
	if err != nil {
		return nil
	}
	var h weatherHistory
	if err := json.Unmarshal(data, &h); err != nil {
		return nil
	}
	return h.Records
}

func saveHistory(dataDir string, records []DailyRecord) {
	_ = os.MkdirAll(dataDir, 0o755)
	data, err := json.MarshalIndent(weatherHistory{Records: records}, "", "  ")
	if err != nil {
		return
	}
	_ = os.WriteFile(historyPath(dataDir), data, 0o644)
}

// recordToday は今日の気温記録をupsertし、31日超の古い記録を削除して保存する。
func recordToday(dataDir string, info *Info) {
	today := time.Now().Format("2006-01-02")
	records := loadHistory(dataDir)

	found := false
	for i, r := range records {
		if r.Date == today {
			records[i].TempC = info.TempC
			records[i].Desc = info.Desc
			found = true
			break
		}
	}
	if !found {
		records = append(records, DailyRecord{Date: today, TempC: info.TempC, Desc: info.Desc})
	}

	sort.Slice(records, func(i, j int) bool {
		return records[i].Date < records[j].Date
	})
	if len(records) > maxHistoryDays {
		records = records[len(records)-maxHistoryDays:]
	}

	saveHistory(dataDir, records)
}

// deltaContext は現在の気温と過去記録を比較した差分テキストを返す。
// 有意な差がない場合や履歴がない場合は空文字を返す。
func deltaContext(dataDir string, currentTempC int) string {
	records := loadHistory(dataDir)
	if len(records) == 0 {
		return ""
	}

	yesterday := time.Now().AddDate(0, 0, -1).Format("2006-01-02")
	weekAgo := time.Now().AddDate(0, 0, -7).Format("2006-01-02")

	var yesterdayRec, weekAgoRec *DailyRecord
	for i := range records {
		switch records[i].Date {
		case yesterday:
			yesterdayRec = &records[i]
		case weekAgo:
			weekAgoRec = &records[i]
		}
	}

	if yesterdayRec != nil {
		return tempDiff("昨日", currentTempC, yesterdayRec.TempC)
	}
	if weekAgoRec != nil {
		return tempDiff("先週", currentTempC, weekAgoRec.TempC)
	}
	return ""
}

// tempDiff は基準ラベルと気温差からテキストを生成する。
// 差が±1°C以内の場合も "ほぼ同じ" として返す。
func tempDiff(label string, current, base int) string {
	diff := current - base
	switch {
	case diff >= 5:
		return fmt.Sprintf("%sより%d°C暖かい", label, diff)
	case diff >= 2:
		return fmt.Sprintf("%sより少し暖かい", label)
	case diff >= -1:
		return fmt.Sprintf("%sとほぼ同じ気温", label)
	case diff >= -4:
		return fmt.Sprintf("%sより少し寒い", label)
	default:
		return fmt.Sprintf("%sより%d°C寒い", label, -diff)
	}
}
