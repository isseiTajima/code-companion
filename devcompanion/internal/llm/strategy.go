package llm

// GenerationStrategy はセリフの生成方式を表す。
type GenerationStrategy int

const (
	// StrategyPool はバッチ事前生成したセリフをプールから取り出す方式。
	//
	// 特性:
	//   - 低レイテンシ（キャッシュ済みを即時返却）
	//   - 汎用・繰り返し型イベント向け
	//   - イベント発生時のリアルタイムコンテキストを参照しない
	//   - Ollama 起動前にプリウォーム可能
	//
	// 適するイベント例: heartbeat (定期観察), working (編集中), achievement (成功/コミット)
	StrategyPool GenerationStrategy = iota

	// StrategyDirect はイベント発生時に LLM をリアルタイム呼び出しする方式。
	//
	// 特性:
	//   - 高レイテンシ（LLM 呼び出し分）
	//   - コンテキスト依存・一回限り型イベント向け
	//   - 時間帯・ユーザー入力・プロファイル等をリアルタイムで参照する
	//
	// 適するイベント例: greeting (時間帯依存), user_question (質問内容依存)
	StrategyDirect
)

// reasonStrategies は各 Reason の生成戦略を定義するテーブル。
// 新しい Reason を追加する際はここに追記する。
// 未登録の Reason はデフォルトで StrategyPool が使われる。
var reasonStrategies = map[Reason]GenerationStrategy{
	// --- StrategyDirect: コンテキスト依存・一回限りイベント ---

	// 時間帯依存。起動直後は Ollama が未起動でプリウォーム失敗するため
	// プールに頼るとfallback固定になる。
	ReasonGreeting:  StrategyDirect,
	ReasonInitSetup: StrategyDirect,

	// ユーザー入力への直接応答。内容がリアルタイムコンテキストに完全依存。
	ReasonUserQuestion:     StrategyDirect,
	ReasonQuestionAnswered: StrategyDirect,

	// 閲覧中のURLやページ内容への反応。リアルタイムコンテキストが必要。
	ReasonWebBrowsing: StrategyDirect,

	// --- StrategyPool: 汎用・繰り返し型イベント（デフォルト）---
	// 以下は明示的に Pool を指定（省略可だが可読性のために列挙）

	ReasonThinkingTick:          StrategyPool,
	ReasonActiveEdit:            StrategyPool,
	ReasonSuccess:               StrategyPool,
	ReasonFail:                  StrategyPool,
	ReasonGitCommit:             StrategyPool,
	ReasonGitPush:               StrategyPool,
	ReasonGitAdd:                StrategyPool,
	ReasonIdle:                  StrategyPool,
	ReasonNightWork:             StrategyPool,
	ReasonUserClick:             StrategyPool,
	ReasonAISessionStarted:      StrategyPool,
	ReasonAISessionActive:       StrategyPool,
	ReasonDevSessionStarted:     StrategyPool,
	ReasonProductiveToolActivity: StrategyPool,
	ReasonDocWriting:            StrategyPool,
	ReasonLongInactivity:        StrategyPool,
	ReasonInitObservation:       StrategyPool,
	ReasonInitSupport:           StrategyPool,
	ReasonInitCuriosity:         StrategyPool,
	ReasonInitMemory:            StrategyPool,
}

// strategyFor は Reason に対応する生成戦略を返す。
// question が指定されている場合は常に StrategyDirect を返す。
// 未登録の Reason は StrategyPool にフォールバックする。
func strategyFor(reason Reason, hasQuestion bool) GenerationStrategy {
	if hasQuestion {
		return StrategyDirect
	}
	if s, ok := reasonStrategies[reason]; ok {
		return s
	}
	return StrategyPool // 未登録はプール（安全なデフォルト）
}
