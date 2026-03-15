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
	}
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
