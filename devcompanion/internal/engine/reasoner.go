package engine

import (
	"sakura-kodama/internal/llm"
	"sakura-kodama/internal/monitor"
	"sakura-kodama/internal/observer"
	"sakura-kodama/internal/types"
)

// EventReasoner maps low-level events to high-level speech reasons.
type EventReasoner struct{}

func NewEventReasoner() *EventReasoner {
	return &EventReasoner{}
}

// ReasonFromMonitorEvent determines the speech reason from a MonitorEvent.
func (r *EventReasoner) ReasonFromMonitorEvent(ev monitor.MonitorEvent) llm.Reason {
	// BehaviorResearching → ReasonActiveEdit の変換は WebBrowsing イベントには適用しない
	// （WebBrowsing はページタイトル付きの StrategyDirect で処理するため）
	if ev.Behavior.Type == types.BehaviorResearching && ev.Event != types.EventWebBrowsing {
		return llm.ReasonActiveEdit
	}

	// Priority 1: High-level Event strings
	eventMap := map[types.HighLevelEvent]llm.Reason{
		types.EventAISessionStarted:       llm.ReasonAISessionStarted,
		types.EventAISessionActive:        llm.ReasonAISessionActive,
		types.EventDevSessionStarted:      llm.ReasonDevSessionStarted,
		types.EventDevEditing:             llm.ReasonActiveEdit,
		types.EventGitActivity:            llm.ReasonGitCommit,
		types.EventProductiveToolActivity: llm.ReasonProductiveToolActivity,
		types.EventDocWriting:             llm.ReasonDocWriting,
		types.EventLongInactivity:         llm.ReasonLongInactivity,
		types.EventWebBrowsing:            llm.ReasonWebBrowsing,
	}
	if reason, ok := eventMap[ev.Event]; ok {
		// AIセッション中: ファイル編集イベントはAIセッション継続に吸収
		if ev.IsAISession && reason == llm.ReasonActiveEdit {
			return llm.ReasonAISessionActive
		}
		return reason
	}

	// Priority 2: State-based reasons
	stateMap := map[types.ContextState]llm.Reason{
		types.StateSuccess: llm.ReasonSuccess,
		types.StateFail:    llm.ReasonFail,
		types.StateCoding:  llm.ReasonActiveEdit,
		types.StateIdle:    llm.ReasonIdle,
	}
	if reason, ok := stateMap[ev.State]; ok {
		// AIセッション中: エラー・ファイル変更はAIエージェントの動作であり発話不要
		if ev.IsAISession && (reason == llm.ReasonFail || reason == llm.ReasonActiveEdit) {
			return llm.ReasonAISessionActive
		}
		return reason
	}

	return llm.ReasonThinkingTick
}

// ReasonFromObservation determines the speech reason from a DevObservation.
func (r *EventReasoner) ReasonFromObservation(obs observer.DevObservation) llm.Reason {
	obsMap := map[observer.ObservationType]llm.Reason{
		observer.ObsGitCommit:    llm.ReasonGitCommit,
		observer.ObsGitPush:      llm.ReasonGitPush,
		observer.ObsGitAdd:       llm.ReasonGitAdd,
		observer.ObsIdleStart:    llm.ReasonIdle,
		observer.ObsNightWork:    llm.ReasonNightWork,
		observer.ObsActiveEditing: llm.ReasonActiveEdit,
	}
	if reason, ok := obsMap[obs.Type]; ok {
		return reason
	}
	return llm.ReasonThinkingTick
}
