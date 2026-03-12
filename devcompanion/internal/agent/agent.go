package agent

import (
	"sakura-kodama/internal/types"
)

// AgentAdapter は特定のAIエージェントの検知と活動抽出を行う。
type AgentAdapter interface {
	Name() string
	DetectProcess() []string
	DetectLogPaths() []string
	ParseLogLine(line string) *types.Signal
}

// Registry は利用可能な AI エージェントの Adapter を管理する。
type Registry struct {
	adapters []AgentAdapter
}

func NewRegistry() *Registry {
	return &Registry{
		adapters: make([]AgentAdapter, 0),
	}
}

func (r *Registry) Register(adapter AgentAdapter) {
	r.adapters = append(r.adapters, adapter)
}

func (r *Registry) Adapters() []AgentAdapter {
	return r.adapters
}
