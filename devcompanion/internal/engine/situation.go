package engine

import (
	"sakura-kodama/internal/types"
	"sync"
	"time"
)

const (
	DeepWorkActivityThreshold = 15   // 15イベント連続でDeepWork判定（5だと即quiet化して単調になる）
	DeepWorkDurationThreshold = 10 * time.Minute
	StrugglingThreshold       = 3    // failed builds in a row
)

// SituationEngine manages the WorldModel and Sakura's EmotionState.
type SituationEngine struct {
	mu         sync.RWMutex
	world      types.WorldModel
	emotion    types.EmotionState

	lastFailCount  int
	activityCount  int
	sessionStart   time.Time
	lastDeepWorkAt time.Time // 最後にDeepWork/Codingイベントを受け取った時刻
}

func NewSituationEngine() *SituationEngine {
	return &SituationEngine{
		emotion:      types.EmotionSupportive,
		sessionStart: time.Now(),
		world: types.WorldModel{
			LastActive: types.TimeToStr(time.Now()),
		},
	}
}

// ProcessEvent updates the WorldModel and infers the EmotionState.
func (s *SituationEngine) ProcessEvent(ev types.Event) (types.WorldModel, types.EmotionState) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.world.LastActive = types.TimeToStr(time.Now())

	switch ev.Type {
	case "monitor_event":
		state := ev.Payload["state"].(string)
		if state == string(types.StateFail) {
			s.lastFailCount++
			if s.lastFailCount >= StrugglingThreshold {
				s.world.StrugglingLevel = 0.8
			}
		} else if state == string(types.StateSuccess) {
			s.lastFailCount = 0
			s.world.StrugglingLevel = 0.0
			s.world.Momentum += 0.2
		}
		
		if state == string(types.StateDeepWork) || state == string(types.StateCoding) {
			s.activityCount++
			s.lastDeepWorkAt = time.Now()
			if s.activityCount >= DeepWorkActivityThreshold {
				s.world.IsDeepWork = true
			}
		}

	case "observation_event":
		// Handle idle detection to end DeepWork
		obsType := ev.Payload["type"].(string)
		if obsType == "idle_start" {
			s.world.IsDeepWork = false
			s.activityCount = 0
		}
	}

	// Momentum decay: 0.99/event で緩やかに減衰（コミット後の勢いが数時間持続）
	s.world.Momentum *= 0.99
	if s.world.Momentum > 1.0 {
		s.world.Momentum = 1.0
	}

	s.emotion = s.inferEmotion()
	return s.world, s.emotion
}

const deepWorkIdleTimeout = 30 * time.Minute

func (s *SituationEngine) inferEmotion() types.EmotionState {
	// DeepWork中でも、最後のコーディングイベントから30分以上経過したらリセット
	if s.world.IsDeepWork {
		if !s.lastDeepWorkAt.IsZero() && time.Since(s.lastDeepWorkAt) > deepWorkIdleTimeout {
			s.world.IsDeepWork = false
			s.activityCount = 0
		}
	}

	// エラー中は常に concerned
	if s.world.StrugglingLevel > 0.5 {
		return types.EmotionConcerned
	}
	// 勢いがある（コミット直後など）は excited
	if s.world.Momentum > 0.6 {
		return types.EmotionExcited
	}
	// DeepWork中でも単調にならないよう、作業量で分岐
	// activityCount が多い＝長時間集中 → supportive で励ます
	if s.world.IsDeepWork {
		if s.activityCount > 30 {
			// 長時間集中: 応援モード
			return types.EmotionSupportive
		}
		return types.EmotionQuiet
	}
	return types.EmotionSupportive
}

func (s *SituationEngine) GetState() (types.WorldModel, types.EmotionState) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.world, s.emotion
}
