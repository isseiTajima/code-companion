package monitor

// MoodType はキャラクターの気分を表す。
type MoodType string

const (
	MoodHappy   MoodType = "Happy"
	MoodNervous MoodType = "Nervous"
	MoodFocus   MoodType = "Focus"
	MoodCalm    MoodType = "Calm"
)

// InferMood はStateとTaskの組み合わせからMoodを決定する。
func InferMood(s StateType, t TaskType) MoodType {
	switch {
	case s == StateSuccess:
		return MoodHappy
	case s == StateFail:
		return MoodNervous
	case s == StateRunning && (t == TaskDebug || t == TaskFixFailingTests):
		return MoodFocus
	default:
		return MoodCalm
	}
}
