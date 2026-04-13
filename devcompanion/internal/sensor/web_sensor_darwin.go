//go:build darwin
package sensor

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"sakura-kodama/internal/types"
)

// dwellThreshold: この回数連続して同じURLを検出したらシグナル発火（5秒×2=10秒滞在）
const dwellThreshold = 2

// noisyDomains: コメント対象外のドメイン/プレフィックス（検索結果・広告・ブラウザ内部ページ等）
var noisyDomains = []string{
	// 検索エンジン結果ページ
	"google.com/search",
	"google.co.jp/search",
	"bing.com/search",
	"search.yahoo.com",
	"search.yahoo.co.jp",
	"duckduckgo.com/?q=",
	"duckduckgo.com/?t=",
	// 広告・トラッキング
	"doubleclick.net",
	"googlesyndication.com",
	"googletagmanager.com",
	"analytics.google.com",
	// ブラウザ内部ページ
	"chrome://",
	"chrome-extension://",
	"edge://",
	"about:blank",
	"about:newtab",
	"favorites://",  // Safari 新規タブ
	"safari-resource://",
}

// isNoisyURL はコメント不要なURLかどうかを判定する。
func isNoisyURL(rawURL string) bool {
	for _, pattern := range noisyDomains {
		if strings.Contains(rawURL, pattern) {
			return true
		}
	}
	return false
}

type WebSensor struct {
	pollInterval time.Duration
	lastURL      string // 最後にシグナルを発火したURL
	// デバウンス用
	pendingURL   string
	pendingTitle string
	pendingCount int // 同じURLが連続して検出された回数
}

func NewWebSensor(interval time.Duration) *WebSensor {
	return &WebSensor{
		pollInterval: interval,
	}
}

func (s *WebSensor) Name() string {
	return "WebSensor"
}

func (s *WebSensor) Run(ctx context.Context, signals chan<- types.Signal) error {
	ticker := time.NewTicker(s.pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			title, url, err := s.getActiveTab(ctx)
			if err != nil || url == "" || isNoisyURL(url) {
				s.pendingURL = ""
				s.pendingCount = 0
				continue
			}

			if url == s.pendingURL {
				// 同じURLが続いている → カウントを増やす
				s.pendingCount++
				if s.pendingCount >= dwellThreshold && url != s.lastURL {
					// dwellThreshold 回連続で同じURL かつ 前回発火URLと異なる → 発火
					s.lastURL = url
					select {
					case signals <- types.Signal{
						Type:      types.SigWebNavigated,
						Source:    types.SourceWeb,
						Value:     url,
						Message:   fmt.Sprintf("browsing: %s", title),
						Timestamp: types.TimeToStr(time.Now()),
					}:
					default:
					}
				}
			} else {
				// URLが変わった → 新しい候補としてリセット
				s.pendingURL = url
				s.pendingTitle = title
				s.pendingCount = 1
			}
		}
	}
}

// browserDef は AppleScript でタブ情報を取得するブラウザの定義。
// 新しいブラウザを追加する場合は supportedBrowsers にエントリを追加するだけでよい。
type browserDef struct {
	appName   string // macOS アプリケーション名
	tabExpr   string // "active tab" (Chrome系) or "current tab" (Safari)
	titleProp string // "title" (Chrome系) or "name" (Safari)
}

// supportedBrowsers は対応ブラウザのリスト。優先順位順に並べる。
var supportedBrowsers = []browserDef{
	{"Brave Browser", "active tab", "title"},
	{"Google Chrome", "active tab", "title"},
	{"Safari", "current tab", "name"},
}

// buildBrowserScript は supportedBrowsers からタブ取得 AppleScript を生成する。
// フロントモストアプリがブラウザでない場合は空文字を返す（バックグラウンド閲覧を無視）。
func buildBrowserScript(browsers []browserDef) string {
	var sb strings.Builder
	// フロントモストのアプリ名を取得
	sb.WriteString("tell application \"System Events\"\n\tset frontApp to name of first application process whose frontmost is true\nend tell\n\n")
	for _, b := range browsers {
		fmt.Fprintf(&sb,
			"if frontApp is %q then\n\ttry\n\t\ttell application %q\n\t\t\tif (count of windows) > 0 then\n\t\t\t\tset activeTab to %s of front window\n\t\t\t\treturn (%s of activeTab) & \"|||\" & (URL of activeTab)\n\t\t\tend if\n\t\tend tell\n\ton error\n\tend try\nend if\n\n",
			b.appName, b.appName, b.tabExpr, b.titleProp,
		)
	}
	sb.WriteString("return \"\"\n")
	return sb.String()
}

func (s *WebSensor) getActiveTab(ctx context.Context) (string, string, error) {
	script := buildBrowserScript(supportedBrowsers)

	// 2秒のタイムアウトを設定（osascript が固まるのを防ぐ）
	tctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()

	out, err := exec.CommandContext(tctx, "osascript", "-e", script).Output()
	if err != nil {
		// タイムアウトや実行エラー時は静かに無視（システム負荷をかけない）
		return "", "", nil
	}

	result := strings.TrimSpace(string(out))
	if result == "" {
		return "", "", nil
	}

	parts := strings.Split(result, "|||")
	if len(parts) == 2 {
		return parts[0], parts[1], nil
	}

	return "", "", nil
}
