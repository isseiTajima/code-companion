package llm

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// SpeechReviewItem はレビュー対象の1セリフ。
type SpeechReviewItem struct {
	Speech      string `json:"speech"`
	Personality string `json:"personality"`
	Category    string `json:"category"`
	Lang        string `json:"lang"`
	Source      string `json:"source"`
}

// SpeechRatingRecord は1件の評価記録。
type SpeechRatingRecord struct {
	TS          string `json:"ts"`
	Speech      string `json:"speech"`
	Rating      int    `json:"rating"` // 1-10
	Comment     string `json:"comment,omitempty"`
	Personality string `json:"personality,omitempty"`
	Category    string `json:"category,omitempty"`
	Lang        string `json:"lang,omitempty"`
}

func ratingsPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(home, ".sakura-kodama")
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", err
	}
	return filepath.Join(dir, "ratings.jsonl"), nil
}

// LoadRatedSpeeches は既評価セリフのセットを返す（重複スキップ用）。
func LoadRatedSpeeches() (map[string]struct{}, error) {
	path, err := ratingsPath()
	if err != nil {
		return nil, err
	}
	f, err := os.Open(path)
	if os.IsNotExist(err) {
		return map[string]struct{}{}, nil
	}
	if err != nil {
		return nil, err
	}
	defer f.Close()

	rated := make(map[string]struct{})
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		var r SpeechRatingRecord
		if err := json.Unmarshal(sc.Bytes(), &r); err != nil {
			continue
		}
		rated[normalizeKey(r.Speech)] = struct{}{}
	}
	return rated, sc.Err()
}

// LoadUnratedSpeeches は過去 days 日分のauditログから未評価セリフを返す。
// since が非ゼロの場合はそれ以降のタイムスタンプのものだけを対象にする。
// 同一テキストは1件にまとめる。
func LoadUnratedSpeeches(days int, since ...time.Time) ([]SpeechReviewItem, error) {
	rated, err := LoadRatedSpeeches()
	if err != nil {
		return nil, err
	}

	var sinceTime time.Time
	if len(since) > 0 {
		sinceTime = since[0]
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	auditDir := filepath.Join(home, ".sakura-kodama", "audit")

	seen := make(map[string]struct{})
	var items []SpeechReviewItem

	now := time.Now()
	for i := 0; i < days; i++ {
		day := now.AddDate(0, 0, -i).Format("20060102")
		path := filepath.Join(auditDir, fmt.Sprintf("speech_%s.jsonl", day))
		f, err := os.Open(path)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			continue
		}

		sc := bufio.NewScanner(f)
		sc.Buffer(make([]byte, 1024*1024), 1024*1024)
		for sc.Scan() {
			var rec SpeechRecord
			if err := json.Unmarshal(sc.Bytes(), &rec); err != nil {
				continue
			}
			if rec.Type != RecordTypeSpeech || rec.Speech == "" || rec.Source == "fallback" {
				continue
			}
			if !sinceTime.IsZero() {
				ts, err := time.Parse(time.RFC3339, rec.Timestamp)
				if err != nil || ts.Before(sinceTime) {
					continue
				}
			}
			key := normalizeKey(rec.Speech)
			if _, done := rated[key]; done {
				continue
			}
			if _, dup := seen[key]; dup {
				continue
			}
			seen[key] = struct{}{}
			items = append(items, SpeechReviewItem{
				Speech:      rec.Speech,
				Personality: rec.Personality,
				Category:    rec.Category,
				Lang:        rec.Language,
				Source:      rec.Source,
			})
		}
		f.Close()
	}
	return items, nil
}

// SaveRating は1件の評価をratings.jsonlに追記する。
func SaveRating(item SpeechReviewItem, rating int, comment string) error {
	path, err := ratingsPath()
	if err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer f.Close()

	rec := SpeechRatingRecord{
		TS:          time.Now().Format(time.RFC3339),
		Speech:      item.Speech,
		Rating:      rating,
		Comment:     comment,
		Personality: item.Personality,
		Category:    item.Category,
		Lang:        item.Lang,
	}
	return json.NewEncoder(f).Encode(rec)
}

func normalizeKey(s string) string {
	return strings.TrimSpace(strings.ToLower(s))
}
