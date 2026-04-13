package profile

import (
	"sakura-kodama/internal/types"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"sync"
	"time"
)

// Relationship はユーザーとサクラの親密度モデル。
type Relationship struct {
	Level                   int    `json:"relationship_level"`        // 0-100
	Trust                   int    `json:"trust"`                     // 0-100
	EncouragementPreference string `json:"encouragement_preference"` // "gentle"|"strict"
}

// NewsInterests はユーザーのニュース関心を記録する。
type NewsInterests struct {
	CategoryScores    map[string]float64 `json:"category_scores"`    // "tech":0.8, "game":0.3
	LikedHeadlines    []string           `json:"liked_headlines"`    // 最大20件
	DislikedHeadlines []string           `json:"disliked_headlines"` // 最大10件
	ShownHeadlines    map[string]int     `json:"shown_headlines"`    // 見出し → 表示回数（最大200エントリ）
}

// DevProfile はセリフ生成に渡す開発者プロファイル。
type DevProfile struct {
	NightCoder        bool                                 `json:"night_coder"`
	CommitFrequency   string                               `json:"commit_frequency"` // "low"|"medium"|"high"
	BuildFailRate     string                               `json:"build_fail_rate"`  // "low"|"medium"|"high"
	LastActive        string                               `json:"last_active"`      // ISO 8601
	Relationship      Relationship                         `json:"relationship"`
	Personality       types.UserPersonality                `json:"personality"`
	Evolution         map[types.TraitID]types.TraitProgress `json:"evolution"`
	Memories          []types.ProjectMoment                `json:"memories"`
	PersonalMemories  []types.PersonalMemory               `json:"personal_memories"`
	NewsInterests     NewsInterests                        `json:"news_interests"`
}

// fileData はファイルに永続化する統計データと DevProfile を合わせた構造体。
type fileData struct {
	DevProfile
	CommitCount   int `json:"commit_count"`
	BuildSuccess  int `json:"build_success"`
	BuildFail     int `json:"build_fail"`
	NightActivity int `json:"night_activity"` // 深夜帯のイベント数
	TotalActivity int `json:"total_activity"` // 全イベント数
	AnswerCount   int `json:"answer_count"`   // 質問への回答数（Trust 強化）
}

// ProfileStore は開発者プロファイルを管理する。
type ProfileStore struct {
	mu   sync.Mutex
	path string
	data fileData
}

// NewProfileStore は ProfileStore を初期化する。
func NewProfileStore(path string) (*ProfileStore, error) {
	ps := &ProfileStore{path: path}
	ps.data.Personality.Traits = make(map[types.TraitID]float64)
	ps.data.Evolution = make(map[types.TraitID]types.TraitProgress)
	ps.data.Memories = make([]types.ProjectMoment, 0)
	ps.data.PersonalMemories = make([]types.PersonalMemory, 0)

	raw, err := os.ReadFile(path)
	if err == nil {
		if parseErr := json.Unmarshal(raw, &ps.data); parseErr != nil {
			return nil, fmt.Errorf("parse profile: %w", parseErr)
		}
	} else if !os.IsNotExist(err) {
		return nil, fmt.Errorf("read profile: %w", err)
	}

	// 必須フィールドの初期化
	if ps.data.Personality.Traits == nil {
		ps.data.Personality.Traits = make(map[types.TraitID]float64)
	}
	if ps.data.Evolution == nil {
		ps.data.Evolution = make(map[types.TraitID]types.TraitProgress)
	}
	if ps.data.Memories == nil {
		ps.data.Memories = make([]types.ProjectMoment, 0)
	}
	if ps.data.PersonalMemories == nil {
		ps.data.PersonalMemories = make([]types.PersonalMemory, 0)
	}

	ps.data.DevProfile = computeProfile(ps.data)
	return ps, nil
}

// RecordCommit はコミットを記録してファイルに書き込む。
func (ps *ProfileStore) RecordCommit() {
	ps.mu.Lock()
	defer ps.mu.Unlock()
	ps.data.CommitCount++
	ps.data.DevProfile = computeProfile(ps.data)
	_ = ps.save()
}

// RecordBuildSuccess はビルド成功を記録してファイルに書き込む。
func (ps *ProfileStore) RecordBuildSuccess() {
	ps.mu.Lock()
	defer ps.mu.Unlock()
	ps.data.BuildSuccess++
	ps.data.DevProfile = computeProfile(ps.data)
	_ = ps.save()
}

// RecordBuildFail はビルド失敗を記録してファイルに書き込む。
func (ps *ProfileStore) RecordBuildFail() {
	ps.mu.Lock()
	defer ps.mu.Unlock()
	ps.data.BuildFail++
	ps.data.DevProfile = computeProfile(ps.data)
	_ = ps.save()
}

// RecordActivity はアクティビティを記録する。
func (ps *ProfileStore) RecordActivity(now time.Time) {
	ps.mu.Lock()
	defer ps.mu.Unlock()
	ps.data.TotalActivity++
	if isNightHour(now.Hour()) {
		ps.data.NightActivity++
	}
	ps.data.LastActive = now.UTC().Format(time.RFC3339)
	ps.data.DevProfile = computeProfile(ps.data)
}

// RecordTraitUpdate はユーザーの回答に基づいて特性を更新する。
// RecordTraitAsked は質問を発火した時点で LastAsked を記録する。
// ユーザーがスキップしても12時間クールダウンが適用される。
func (ps *ProfileStore) RecordTraitAsked(trait types.TraitID) {
	ps.mu.Lock()
	defer ps.mu.Unlock()
	prog := ps.data.Evolution[trait]
	prog.LastAsked = types.TimeToStr(time.Now())
	ps.data.Evolution[trait] = prog
	_ = ps.save()
}

func (ps *ProfileStore) RecordTraitUpdate(trait types.TraitID, value float64, answer string) {
	ps.mu.Lock()
	defer ps.mu.Unlock()

	current := ps.data.Personality.Traits[trait]
	if current == 0 {
		ps.data.Personality.Traits[trait] = value
	} else {
		ps.data.Personality.Traits[trait] = (current + value) / 2.0
	}

	prog := ps.data.Evolution[trait]

	// 矛盾検出: 前の回答と新しい回答が大きく異なる場合は信頼度を下げる
	if current != 0 && math.Abs(current-value) > 0.4 {
		prog.Confidence = math.Max(0.1, prog.Confidence-0.1)
		fmt.Printf("[LEARNING] Contradiction detected for %s (prev=%.2f, new=%.2f) — confidence adjusted to %.2f\n",
			trait, current, value, prog.Confidence)
	} else {
		prog.Confidence += 0.2
	}
	if prog.Confidence > 1.0 {
		prog.Confidence = 1.0
	}
	if prog.Confidence >= 0.8 {
		prog.CurrentStage = 2
	} else if prog.Confidence >= 0.4 {
		prog.CurrentStage = 1
	}
	prog.LastAnswer = answer
	prog.LastUpdated = types.TimeToStr(time.Now())
	// 回答履歴を AskedTopics に追積（最大5件）
	if answer != "" && answer != "対象なし" {
		prog.AskedTopics = append(prog.AskedTopics, answer)
		if len(prog.AskedTopics) > 5 {
			prog.AskedTopics = prog.AskedTopics[len(prog.AskedTopics)-5:]
		}
	}
	ps.data.Evolution[trait] = prog

	// 質問に答えてくれたことで AnswerCount を加算（Trust 計算に反映）
	if answer != "" && answer != "対象なし" {
		ps.data.AnswerCount++
	}

	ps.data.DevProfile = computeProfile(ps.data)
	_ = ps.save()
}

// RecordPersonalMemory はユーザーの個人情報・会話内容を記録する（最大100件）。
func (ps *ProfileStore) RecordPersonalMemory(mem types.PersonalMemory) {
	ps.mu.Lock()
	defer ps.mu.Unlock()
	ps.data.PersonalMemories = append(ps.data.PersonalMemories, mem)
	if len(ps.data.PersonalMemories) > 100 {
		ps.data.PersonalMemories = ps.data.PersonalMemories[1:]
	}
	ps.data.DevProfile = computeProfile(ps.data)
	_ = ps.save()
}

// RecordMoment はプロジェクトの重要な瞬間を記録する。
func (ps *ProfileStore) RecordMoment(moment types.ProjectMoment) {
	ps.mu.Lock()
	defer ps.mu.Unlock()
	ps.data.Memories = append(ps.data.Memories, moment)
	if len(ps.data.Memories) > 50 {
		ps.data.Memories = ps.data.Memories[1:]
	}
	ps.data.DevProfile = computeProfile(ps.data)
	_ = ps.save()
}

// Get は現在の DevProfile を返す。
func (ps *ProfileStore) Get() DevProfile {
	ps.mu.Lock()
	defer ps.mu.Unlock()
	return ps.data.DevProfile
}

// Stop は最終データを保存する。
func (ps *ProfileStore) Stop() error {
	ps.mu.Lock()
	defer ps.mu.Unlock()
	return ps.save()
}

func (ps *ProfileStore) save() error {
	b, err := json.MarshalIndent(ps.data, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(ps.path, b, 0644)
}

func computeProfile(d fileData) DevProfile {
	return DevProfile{
		CommitFrequency:  commitFrequency(d.CommitCount),
		BuildFailRate:    buildFailRate(d.BuildSuccess, d.BuildFail),
		NightCoder:       nightCoder(d.NightActivity, d.TotalActivity),
		LastActive:       d.LastActive,
		Relationship:     computeRelationship(d),
		Personality:      d.Personality,
		Evolution:        d.Evolution,
		Memories:         d.Memories,
		PersonalMemories: d.PersonalMemories,
		NewsInterests:    d.NewsInterests,
	}
}

// RecordNewsInterest はニュースへの関心フィードバックを記録する。
// tags: フィードカテゴリ ("tech", "game", "anime" など)
// headlines: 表示された見出し文字列（複数行も可）
func (ps *ProfileStore) RecordNewsInterest(headlines string, tags []string, interested bool) {
	ps.mu.Lock()
	defer ps.mu.Unlock()

	ni := &ps.data.NewsInterests
	if ni.CategoryScores == nil {
		ni.CategoryScores = make(map[string]float64)
	}

	// カテゴリスコア更新（±0.15、[-1, 1] クランプ）
	delta := 0.15
	if !interested {
		delta = -0.15
	}
	for _, tag := range tags {
		score := ni.CategoryScores[tag] + delta
		if score > 1.0 {
			score = 1.0
		} else if score < -1.0 {
			score = -1.0
		}
		ni.CategoryScores[tag] = score
	}

	// 見出し履歴を追加
	if interested {
		ni.LikedHeadlines = appendCapped(ni.LikedHeadlines, headlines, 20)
	} else {
		ni.DislikedHeadlines = appendCapped(ni.DislikedHeadlines, headlines, 10)
	}

	ps.data.DevProfile = computeProfile(ps.data)
	_ = ps.save()
}

// RecordNewsShown はニュース見出しの表示回数をインクリメントする。
// エントリが 200 件を超えたら表示済み（count >= 2）のものから削除して圧縮する。
func (ps *ProfileStore) RecordNewsShown(headline string) {
	ps.mu.Lock()
	defer ps.mu.Unlock()

	ni := &ps.data.NewsInterests
	if ni.ShownHeadlines == nil {
		ni.ShownHeadlines = make(map[string]int)
	}
	ni.ShownHeadlines[headline]++

	// 肥大化防止: 200件超えたら表示済み(>=2)エントリを削除
	if len(ni.ShownHeadlines) > 200 {
		for k, v := range ni.ShownHeadlines {
			if v >= 2 {
				delete(ni.ShownHeadlines, k)
			}
		}
	}

	ps.data.DevProfile = computeProfile(ps.data)
	_ = ps.save()
}

func appendCapped(list []string, item string, max int) []string {
	// 重複チェック
	for _, s := range list {
		if s == item {
			return list
		}
	}
	list = append(list, item)
	if len(list) > max {
		list = list[len(list)-max:]
	}
	return list
}

func computeRelationship(d fileData) Relationship {
	level := d.TotalActivity / 100
	if level > 100 {
		level = 100
	}
	// Trust: ビルド成功 + 質問回答数（各2pt）の合算
	trust := d.BuildSuccess/5 + d.AnswerCount*2
	if trust > 100 {
		trust = 100
	}
	pref := "gentle"
	if d.BuildFail > d.BuildSuccess*2 {
		pref = "strict"
	}
	return Relationship{
		Level:                   level,
		Trust:                   trust,
		EncouragementPreference: pref,
	}
}

func commitFrequency(count int) string {
	if count >= 5 {
		return "high"
	}
	if count >= 2 {
		return "medium"
	}
	return "low"
}

func buildFailRate(success, fail int) string {
	total := success + fail
	if total == 0 {
		return "low"
	}
	rate := float64(fail) / float64(total) * 100
	if rate > 60 {
		return "high"
	}
	if rate > 30 {
		return "medium"
	}
	return "low"
}

func nightCoder(nightActivity, totalActivity int) bool {
	if totalActivity == 0 {
		return false
	}
	return float64(nightActivity)/float64(totalActivity) >= 0.3
}

func isNightHour(hour int) bool {
	return hour >= 23 || hour < 5
}
