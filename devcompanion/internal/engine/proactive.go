package engine

import (
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"sync"
	"time"

	"sakura-kodama/internal/config"
	"sakura-kodama/internal/llm"
	"sakura-kodama/internal/monitor"
	"sakura-kodama/internal/news"
	"sakura-kodama/internal/profile"
	"sakura-kodama/internal/types"
	"sakura-kodama/internal/weather"
)

// sakuraDataDir は ~/.sakura-kodama のパスを返す。取得失敗時は空文字を返す。
func sakuraDataDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".sakura-kodama")
}

var initiativeWeights = map[types.InitiativeType]float64{
	types.InitObservation: 0.45,
	types.InitSupport:     0.27,
	types.InitCuriosity:   0.15,
	types.InitMemory:      0.05,
	types.InitWeather:     0.08,
}

const (
	MinInitiativeInterval = 6 * time.Minute // 最短インターバル（前回から6分以内はスキップ）
	// ニュース保証: スロット開始後このくらい経ってから発火（起動直後を避ける）
	NewsSlotGrace = 20 * time.Minute
	// 起動後この時間が経過したら最初のニュースを発言
	StartupNewsDelay = 3 * time.Minute
	// ランダムニュース発言の独立確率（通常Tickで追加チェック）
	NewsRandomProbability = 0.15
)

// initiativeProbability は SpeechFrequency に基づく自発発話の通過確率を返す。
//
//   - freq=1 (低): 5%  — 期待20分に1回
//   - freq=2 (中): 10% — 期待10分に1回（デフォルト）
//   - freq=3 (高): 25% — 期待4分に1回
//   - freq=4 (dev): 100% — 毎Tickで発火（開発・テスト用）
func initiativeProbability(freq int) float64 {
	switch freq {
	case 1:
		return 0.05
	case 3:
		return 0.25
	case 4:
		return 1.0
	default: // 2
		return 0.10
	}
}

// minInitiativeInterval は SpeechFrequency に基づく自発発話の最短インターバルを返す。
//
//   - freq=4 (dev): 30秒 — 開発・テスト用超高頻度モード
//   - それ以外: 6分
func minInitiativeInterval(freq int) time.Duration {
	if freq == 4 {
		return 30 * time.Second
	}
	return MinInitiativeInterval
}

// newsSlot は時刻をスロット番号に変換する。
// 0:朝 6-12, 1:昼 12-18, 2:夜 18-24, 3:深夜 0-6
func newsSlot(t time.Time) int {
	h := t.Hour()
	switch {
	case h >= 6 && h < 12:
		return 0
	case h >= 12 && h < 18:
		return 1
	case h >= 18:
		return 2
	default:
		return 3
	}
}

// newsSlotStart は指定スロットの開始時刻を返す。
func newsSlotStart(t time.Time) time.Time {
	y, m, d := t.Date()
	loc := t.Location()
	startHours := [4]int{6, 12, 18, 0}
	h := startHours[newsSlot(t)]
	base := time.Date(y, m, d, h, 0, 0, 0, loc)
	// 深夜スロット(3)は当日0時だが、18時以降に見ると翌日になるので調整不要
	return base
}

// ProactiveEngine は外部トリガーなしに Sakura が自発的に話しかける機能を担う。
//
// 不変条件:
//   - profileStore と dispatcher は nil であってはならない
//   - cfg は nil であってはならない
type ProactiveEngine struct {
	mu              sync.Mutex
	state           types.InitiativeState
	profileStore    *profile.ProfileStore
	dispatcher      SpeechDispatcher
	cfg             *config.Config
	newsFetcher     *news.Fetcher
	weatherFetcher  *weather.Fetcher
	lastNewsTime    time.Time // 最後にニュース発言した時刻
	startTime       time.Time // 起動時刻
	startupNewsDone bool      // 起動後ニュース発言済みフラグ
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
	fetcher := news.NewFetcher()
	if cfg.NewsFeeds != nil {
		fetcher.SetCustomFeeds(cfg.NewsFeeds)
	}
	now := time.Now()
	return &ProactiveEngine{
		profileStore:   ps,
		dispatcher:     d,
		cfg:            cfg,
		newsFetcher:    fetcher,
		weatherFetcher: weather.NewFetcherWithHistory(sakuraDataDir()),
		startTime:      now,
		state: types.InitiativeState{
			LastTime: types.TimeToStr(now),
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
	if cfg.NewsFeeds != nil {
		p.newsFetcher.SetCustomFeeds(cfg.NewsFeeds)
	}
}

// Tick は定期的に呼ばれ、自発的発話の機会をチェックする。
// ev は現在のモニタリングイベント（コンテキスト取得のため）。
func (p *ProactiveEngine) Tick(ev monitor.MonitorEvent) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if time.Since(types.StrToTime(p.state.LastTime)) < minInitiativeInterval(p.cfg.SpeechFrequency) {
		return
	}

	world, _ := p.dispatcher.WorldState()
	// IsAISession中はDeepWorkブロックをスキップ（バイブコーディング中はユーザーは暇）
	// DeepWork中でも完全ブロックしない: 30%の確率で通過
	if world.IsDeepWork && !world.IsAISession && rand.Float64() > 0.30 {
		return
	}

	now := time.Now()

	// 起動後ニュース: StartupNewsDelay 経過後に一度だけ強制発火
	startupNews := !p.startupNewsDone && now.Sub(p.startTime) >= StartupNewsDelay

	// スロット保証ニュース: 今のスロットでまだ発言していない、かつグレース期間経過後
	slotNews := newsSlot(now) != newsSlot(p.lastNewsTime) &&
		now.Sub(newsSlotStart(now)) >= NewsSlotGrace

	forceNews := startupNews || slotNews

	if !forceNews && rand.Float64() > initiativeProbability(p.cfg.SpeechFrequency) {
		return
	}

	initType := p.selectInitiativeType()
	// 強制ニュース or ランダムでも NewsRandomProbability でニュースを優先
	if forceNews || rand.Float64() < NewsRandomProbability {
		initType = types.InitCuriosity
	}
	go p.executeInitiative(initType, ev)

	if initType == types.InitCuriosity {
		p.lastNewsTime = now
		if startupNews {
			p.startupNewsDone = true
		}
	}
	p.state.LastTime = types.TimeToStr(now)
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

// relativeTimeLabel は ISO 8601 タイムスタンプを「この前」「昨日」などの相対表現に変換する。
func relativeTimeLabel(timestamp, lang string) string {
	t := types.StrToTime(timestamp)
	if t.IsZero() {
		if lang == "en" {
			return "a while ago"
		}
		return "以前"
	}
	d := time.Since(t)
	if lang == "en" {
		switch {
		case d < 2*time.Hour:
			return "just now"
		case d < 24*time.Hour:
			return "earlier today"
		case d < 48*time.Hour:
			return "yesterday"
		case d < 7*24*time.Hour:
			return "the other day"
		default:
			return "last week"
		}
	}
	switch {
	case d < 2*time.Hour:
		return "さっき"
	case d < 24*time.Hour:
		return "この前"
	case d < 48*time.Hour:
		return "昨日"
	case d < 7*24*time.Hour:
		return "先日"
	default:
		return "先週"
	}
}

// newsTagsFromProfile はプロフィールからニュースカテゴリタグを決定する。
// "tech" は常に含まれる。スコア > -0.3 のカテゴリを追加し、明示的に嫌いなもの（< -0.3）は除外。
func newsTagsFromProfile(prof profile.DevProfile) []string {
	tags := []string{"tech", "general"}
	scores := prof.NewsInterests.CategoryScores

	// トレイト学習済み → カテゴリ候補として追加（スコアで除外されない限り）
	candidates := map[string]types.TraitID{
		"game":  types.TraitGamePreference,
		"anime": types.TraitAnimePreference,
	}
	for tag, trait := range candidates {
		_, learned := prof.Evolution[trait]
		score := scores[tag] // 未学習は 0.0
		// 嫌い（-0.3未満）でなければ追加。トレイト学習済みまたはスコア > 0 なら積極追加。
		if score > -0.3 && (learned || score > 0) {
			tags = append(tags, tag)
		}
	}
	return tags
}

// initiativePrep は initiative 発火前にイベントを加工する関数型。
type initiativePrep func(p *ProactiveEngine, prof profile.DevProfile, ev *monitor.MonitorEvent)

// initiativePreps は InitiativeType ごとの事前加工テーブル。
// 新しい InitiativeType を追加する場合はここに追記するだけでよい。
var initiativePreps = map[types.InitiativeType]initiativePrep{
	types.InitCuriosity: func(p *ProactiveEngine, prof profile.DevProfile, ev *monitor.MonitorEvent) {
		tags := newsTagsFromProfile(prof)
		all := p.newsFetcher.Headlines(p.cfg.Language, tags)

		// リアクション済み or 表示2回以上の見出しを除外して1件選ぶ
		ni := prof.NewsInterests
		reacted := make(map[string]bool, len(ni.LikedHeadlines)+len(ni.DislikedHeadlines))
		for _, h := range ni.LikedHeadlines {
			reacted[h] = true
		}
		for _, h := range ni.DislikedHeadlines {
			reacted[h] = true
		}
		var chosen string
		for _, h := range all {
			if reacted[h] {
				continue
			}
			if ni.ShownHeadlines[h] >= 2 {
				continue
			}
			chosen = h
			break
		}
		if chosen == "" {
			return
		}
		ev.NewsContext = chosen
		ev.NewsTags = tags
		p.profileStore.RecordNewsShown(chosen)
	},
	types.InitMemory: func(p *ProactiveEngine, prof profile.DevProfile, ev *monitor.MonitorEvent) {
		if len(prof.PersonalMemories) > 0 {
			m := prof.PersonalMemories[rand.Intn(len(prof.PersonalMemories))]
			ev.Details = fmt.Sprintf("Memory: %s %s", relativeTimeLabel(m.CreatedAt, p.cfg.Language), m.Content)
		} else if len(prof.Memories) > 0 {
			m := prof.Memories[rand.Intn(len(prof.Memories))]
			ev.Details = fmt.Sprintf("Remember: %s (at %s)", m.Message, m.Timestamp)
		}
	},
	types.InitWeather: func(p *ProactiveEngine, prof profile.DevProfile, ev *monitor.MonitorEvent) {
		if info, ok := p.weatherFetcher.Get(p.cfg.WeatherLocation); ok {
			ev.WeatherContext = info.String()
			if delta := p.weatherFetcher.DeltaContext(info.TempC); delta != "" {
				ev.WeatherContext += "（" + delta + "）"
			}
		}
		// 位置情報が取れない場合は WeatherContext が空のまま → DispatchSpeech で空チェック
	},
}

func (p *ProactiveEngine) executeInitiative(t types.InitiativeType, ev monitor.MonitorEvent) {
	prof := p.profileStore.Get()
	if prep, ok := initiativePreps[t]; ok {
		prep(p, prof, &ev)
	}
	// 天気・ニュース発言はコンテキストが取れた場合のみ実行（取得失敗なら静かにスキップ）
	// スキップしないとLLMがニュース/天気を架空で生成してしまう（TurboQuant等のハルシネーション）
	if t == types.InitWeather && ev.WeatherContext == "" {
		return
	}
	if t == types.InitCuriosity && ev.NewsContext == "" {
		return
	}
	p.dispatcher.DispatchSpeech("observation_event", ev, llm.Reason("initiative_"+string(t)), "")
}
