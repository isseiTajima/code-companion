package plugin

import (
	"sakura-kodama/internal/types"
	"sync"
)

// Plugin defines the interface for external extensions.
type Plugin interface {
	Name() string
	OnSignal(signal types.Signal)
}

// Registry manages active plugins.
type Registry struct {
	mu      sync.RWMutex
	plugins []Plugin
}

func NewRegistry() *Registry {
	return &Registry{
		plugins: make([]Plugin, 0),
	}
}

func (r *Registry) Register(p Plugin) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.plugins = append(r.plugins, p)
}

func (r *Registry) NotifySignal(sig types.Signal) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, p := range r.plugins {
		p.OnSignal(sig)
	}
}
