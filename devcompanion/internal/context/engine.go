package contextengine

import (
	"sakura-kodama/internal/types"
	"fmt"
	"sync"
)

// Estimator は Signals から状況を確率的に推定する。
type Estimator struct {
	mu            sync.RWMutex
	scores        map[types.ContextState]float64
	current       types.ContextInfo
	weights       map[types.SignalType]float64
	lastSignals   []types.Signal
	confidenceThreshold float64
	
	LastDecision  types.ContextDecision
}

func NewEstimator() *Estimator {
	return &Estimator{
		scores:              make(map[types.ContextState]float64),
		confidenceThreshold: 0.6,
		weights: map[types.SignalType]float64{
			types.SigProcessStarted: 0.5,
			types.SigFileModified:   0.1,
			types.SigGitCommit:      0.7,
			types.SigIdleStart:      0.8,
		},
		current: types.ContextInfo{
			State: types.StateIdle,
		},
	}
}

// ProcessSignal は個別のシグナルによってスコアを変動させる。
func (e *Estimator) ProcessSignal(sig types.Signal) types.ContextDecision {
	e.mu.Lock()
	defer e.mu.Unlock()

	e.lastSignals = append(e.lastSignals, sig)
	if len(e.lastSignals) > 10 {
		e.lastSignals = e.lastSignals[1:]
	}

	weight := e.weights[sig.Type]
	if weight == 0 {
		weight = 0.05 // Default low weight
	}

	// 確率ベースのスコアリング
	switch sig.Type {
	case types.SigProcessStarted:
		if sig.Source == types.SourceAgent {
			e.scores[types.StateAIPairing] += weight
		} else {
			e.scores[types.StateCoding] += weight
		}
	case types.SigFileModified:
		e.scores[types.StateCoding] += weight
		e.scores[types.StateDeepWork] += weight / 2
	case types.SigGitCommit:
		e.scores[types.StateCoding] += weight
	case types.SigIdleStart:
		e.scores[types.StateIdle] += weight
	}

	e.decay()

	var bestState types.ContextState = types.StateIdle
	var maxScore float64 = 0.0

	for state, score := range e.scores {
		if score > maxScore {
			maxScore = score
			bestState = state
		}
	}

	if maxScore >= e.confidenceThreshold {
		e.current.State = bestState
		e.current.Confidence = maxScore
		e.current.LastSignal = sig.Timestamp
	}

	// Decision record for explainability
	decision := types.ContextDecision{
		State:      e.current.State,
		Confidence: e.current.Confidence,
		Signals:    []types.SignalType{sig.Type},
		Reasons:    []string{fmt.Sprintf("Signal %s from %s added %0.2f weight", sig.Type, sig.Source, weight)},
	}
	e.LastDecision = decision
	return decision
}

func (e *Estimator) decay() {
	for state := range e.scores {
		e.scores[state] *= 0.90 // 10%減衰に加速
	}
}

func (e *Estimator) SetWeights(w map[types.SignalType]float64) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.weights = w
}
