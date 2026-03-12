package engine

import (
	"fmt"
	"math/rand"
	"sync"
	"time"

	"sakura-kodama/internal/config"
	"sakura-kodama/internal/llm"
	"sakura-kodama/internal/monitor"
	"sakura-kodama/internal/profile"
	"sakura-kodama/internal/types"
)

var initiativeWeights = map[types.InitiativeType]float64{
	types.InitObservation: 0.50,
	types.InitSupport:     0.30,
	types.InitCuriosity:   0.15,
	types.InitMemory:      0.05,
}

const (
	MinInitiativeInterval = 15 * time.Minute
	InitiativeProbability = 0.03
)

// ProactiveEngine は外部トリガーなしに Sakura が自発的に話しかける機能を担う。
//
// 不変条件:
//   - profileStore と dispatcher は nil であってはならない
//   - cfg は nil であってはならない
type ProactiveEngine struct {
	mu           sync.Mutex
	state        types.InitiativeState
	profileStore *profile.ProfileStore
	dispatcher   SpeechDispatcher
	cfg          *config.Config
}

// NewProactiveEngine は ProactiveEngine を作成する。
//
// 事前条件:
//   - ps は nil であってはならない
//   - d は nil であってはならない
//   - cfg は nil であってはならない
func NewProactiveEngine(ps *profile.ProfileStore, d SpeechDispatcher, cfg *config.Config) *ProactiveEngine {
	if ps == nil {
		panic("engine: NewProactiveEngine: profileStore must not be nil")
	}
	if d == nil {
		panic("engine: NewProactiveEngine: dispatcher must not be nil")
	}
	if cfg == nil {
		panic("engine: NewProactiveEngine: cfg must not be nil")
	}
	return &ProactiveEngine{
		profileStore: ps,
		dispatcher:   d,
		cfg:          cfg,
		state: types.InitiativeState{
			LastTime: types.TimeToStr(time.Now()),
		},
	}
}

// UpdateConfig は設定を更新する。Engine.UpdateConfig から呼ばれる。
// 事前条件: cfg は nil であってはならない。
func (p *ProactiveEngine) UpdateConfig(cfg *config.Config) {
	if cfg == nil {
		panic("engine: ProactiveEngine.UpdateConfig: cfg must not be nil")
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	p.cfg = cfg
}

// Tick は定期的に呼ばれ、自発的発話の機会をチェックする。
// ev は現在のモニタリングイベント（コンテキスト取得のため）。
func (p *ProactiveEngine) Tick(ev monitor.MonitorEvent) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if time.Since(types.StrToTime(p.state.LastTime)) < MinInitiativeInterval {
		return
	}

	world, _ := p.dispatcher.WorldState()
	if world.IsDeepWork {
		return
	}

	if rand.Float64() > InitiativeProbability {
		return
	}

	initType := p.selectInitiativeType()
	go p.executeInitiative(initType, ev)

	p.state.LastTime = types.TimeToStr(time.Now())
	p.state.LastType = initType
	p.state.DailyCount++
}

func (p *ProactiveEngine) selectInitiativeType() types.InitiativeType {
	r := rand.Float64()
	var cumulative float64
	for t, w := range initiativeWeights {
		cumulative += w
		if r <= cumulative {
			if t == p.state.LastType {
				continue
			}
			return t
		}
	}
	return types.InitObservation
}

func (p *ProactiveEngine) executeInitiative(t types.InitiativeType, ev monitor.MonitorEvent) {
	prof := p.profileStore.Get()

	reason := llm.Reason("initiative_" + string(t))

	// InitMemory の場合はメモリの内容をイベント詳細に埋め込む
	if t == types.InitMemory && len(prof.Memories) > 0 {
		m := prof.Memories[rand.Intn(len(prof.Memories))]
		ev.Details = fmt.Sprintf("Remember: %s (at %s)", m.Message, m.Timestamp)
	}

	p.dispatcher.DispatchSpeech("observation_event", ev, reason, "")
}
