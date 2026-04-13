package llm

import (
	"time"
)

// PersonalityType represents the current personality of the assistant.
type PersonalityType string

const (
	PersonalityCute  PersonalityType = "cute"
	PersonalityCool  PersonalityType = "cool"
	PersonalityGenki PersonalityType = "genki"
)

// PersonalityContext contains the factors that influence personality transitions.
type PersonalityContext struct {
	Reason            Reason
	FailStreak        int
	SuccessStreak     int
	SameFileEditCount int
	GenkiSpeechCount  int
	SessionDuration   time.Duration
}

// PersonalityManager handles the state transitions between different personalities.
type PersonalityManager struct {
	current PersonalityType
	context PersonalityContext
}

func NewPersonalityManager() *PersonalityManager {
	return &PersonalityManager{
		current: PersonalityCute,
	}
}

func (m *PersonalityManager) Current() PersonalityType {
	if m.current == "" {
		return PersonalityCute
	}
	return m.current
}

func (m *PersonalityManager) SetCurrent(p PersonalityType) {
	m.current = p
}

// Update determines the next personality state based on the current context and events.
func (m *PersonalityManager) Update(ctx PersonalityContext) PersonalityType {
	m.context = ctx

	// 1. Cool condition (Highest priority)
	if m.isCoolConditionMet() {
		m.current = PersonalityCool
		return m.current
	}

	// 2. Cool recovery
	if m.current == PersonalityCool {
		if ctx.SuccessStreak >= 2 {
			m.current = PersonalityCute
		}
		return m.current
	}

	// 3. Genki transition
	if m.isSuccessMilestone(ctx.Reason) {
		m.current = PersonalityGenki
		return m.current
	}

	// 4. Genki decay
	if m.current == PersonalityGenki {
		if ctx.GenkiSpeechCount >= 3 {
			m.current = PersonalityCute
		}
		return m.current
	}

	// 5. Default
	if m.current == "" {
		m.current = PersonalityCute
	}

	return m.current
}

func (m *PersonalityManager) isCoolConditionMet() bool {
	ctx := m.context
	return ctx.FailStreak >= 2 ||
		ctx.SameFileEditCount >= 3 ||
		ctx.Reason == ReasonLongInactivity ||
		ctx.SessionDuration >= 90*time.Minute
}

func (m *PersonalityManager) isSuccessMilestone(r Reason) bool {
	return r == ReasonSuccess || r == ReasonGitCommit || r == ReasonGitPush
}
