package ws

// Event はUIへブロードキャストするイベント型。
type Event struct {
	State  string `json:"state"`
	Task   string `json:"task"`
	Mood   string `json:"mood"`
	Speech string `json:"speech"`
}
