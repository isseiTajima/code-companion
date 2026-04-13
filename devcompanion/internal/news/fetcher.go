package news

import (
	"encoding/xml"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"time"
)

const (
	fetchTimeout = 5 * time.Second
	cacheTTL     = 60 * time.Minute
	maxHeadlines = 5
)

// Feed はひとつの RSS フィードを表す。
type Feed struct {
	URL  string
	Lang string // "ja" or "en"
	Tags []string
}

// feedCatalog は利用可能なフィード一覧。
// Tag: "tech", "game", "anime", "general"
var feedCatalog = []Feed{
	// --- tech (en) ---
	{URL: "https://hnrss.org/frontpage", Lang: "en", Tags: []string{"tech"}},
	{URL: "https://feeds.feedburner.com/TechCrunch", Lang: "en", Tags: []string{"tech"}},
	// --- tech (ja) ---
	{URL: "https://gigazine.net/news/rss_2.0/", Lang: "ja", Tags: []string{"tech"}},
	{URL: "https://rss.itmedia.co.jp/rss/2.0/techplus.xml", Lang: "ja", Tags: []string{"tech"}},
	// --- general (ja) ---
	{URL: "https://www.nhk.or.jp/rss/news/cat0.xml", Lang: "ja", Tags: []string{"general"}},
	// --- game (ja) ---
	{URL: "https://automaton-media.com/feed/", Lang: "ja", Tags: []string{"game"}},
	{URL: "https://www.4gamer.net/rss/index.xml", Lang: "ja", Tags: []string{"game"}},
	// --- general (en) ---
	{URL: "https://feeds.bbci.co.uk/news/world/rss.xml", Lang: "en", Tags: []string{"general"}},
	{URL: "https://feeds.reuters.com/reuters/topNews", Lang: "en", Tags: []string{"general"}},
	// --- game (en) ---
	{URL: "https://www.ign.com/articles/feed/all", Lang: "en", Tags: []string{"game"}},
	// --- anime/manga (ja) ---
	{URL: "https://natalie.mu/comic/feed/news", Lang: "ja", Tags: []string{"anime"}},
	{URL: "https://natalie.mu/music/feed/news", Lang: "ja", Tags: []string{"anime"}},
}

type cacheKey struct {
	lang string
	tags string // sorted joined tags
}

type cacheEntry struct {
	headlines []string
	fetchedAt time.Time
}

// Fetcher は RSS フィードからニュースの見出しを取得してキャッシュする。
// 言語 × トピックの組み合わせごとに独立したキャッシュを持つ。
type Fetcher struct {
	mu          sync.Mutex
	cache       map[cacheKey]cacheEntry
	customFeeds map[string][]string // カテゴリ → URL リスト（nil = デフォルト使用）
}

// NewFetcher は Fetcher を返す。
func NewFetcher() *Fetcher {
	return &Fetcher{
		cache: make(map[cacheKey]cacheEntry),
	}
}

// SetCustomFeeds はカテゴリ別カスタムフィードを更新する。
// 設定変更時に呼び出す。空スライスはそのカテゴリのデフォルトリセットを意味する。
func (f *Fetcher) SetCustomFeeds(feeds map[string][]string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.customFeeds = feeds
	// カスタムフィード変更時はキャッシュを破棄
	f.cache = make(map[cacheKey]cacheEntry)
}

// Headlines は言語とトピックタグに合ったニュース見出しを最大5件返す。
// tags が空の場合は "tech" のみ。
func (f *Fetcher) Headlines(lang string, tags []string) []string {
	if lang == "" {
		lang = "ja"
	}
	if len(tags) == 0 {
		tags = []string{"tech"}
	}
	key := cacheKey{lang: lang, tags: strings.Join(tags, ",")}

	f.mu.Lock()
	defer f.mu.Unlock()

	entry := f.cache[key]
	if time.Since(entry.fetchedAt) < cacheTTL && len(entry.headlines) > 0 {
		return entry.headlines
	}

	fresh := f.fetchForLangTags(lang, tags)
	if len(fresh) > 0 {
		f.cache[key] = cacheEntry{headlines: fresh, fetchedAt: time.Now()}
		return fresh
	}
	return entry.headlines // 取得失敗: 前回キャッシュ
}

// feedsFor は lang × tags にマッチするフィードを返す。
// 同じ lang のものを優先し、マッチするものがなければ fallback として反対言語を追加。
func (f Feed) matchesTags(tagSet map[string]bool) bool {
	for _, t := range f.Tags {
		if tagSet[t] {
			return true
		}
	}
	return false
}

func feedsFor(lang string, tags []string) []Feed {
	tagSet := make(map[string]bool, len(tags))
	for _, t := range tags {
		tagSet[t] = true
	}

	var primary, fallback []Feed
	for _, f := range feedCatalog {
		if !f.matchesTags(tagSet) {
			continue
		}
		if f.Lang == lang {
			primary = append(primary, f)
		} else {
			fallback = append(fallback, f)
		}
	}
	return append(primary, fallback...)
}

func (f *Fetcher) feedsForWithCustom(lang string, tags []string) []Feed {
	if len(f.customFeeds) == 0 {
		return feedsFor(lang, tags)
	}
	// カスタムフィードが設定されているカテゴリは置き換え、未設定はデフォルト使用
	tagSet := make(map[string]bool, len(tags))
	for _, t := range tags {
		tagSet[t] = true
	}

	var result []Feed
	coveredByCustom := make(map[string]bool)
	for _, tag := range tags {
		if urls, ok := f.customFeeds[tag]; ok && len(urls) > 0 {
			coveredByCustom[tag] = true
			for _, u := range urls {
				result = append(result, Feed{URL: u, Lang: lang, Tags: []string{tag}})
			}
		}
	}
	// カスタムで未カバーのカテゴリはデフォルトカタログから補完
	for _, feed := range feedCatalog {
		if !feed.matchesTags(tagSet) {
			continue
		}
		// このフィードのタグが全てカスタムでカバー済みならスキップ
		alreadyCovered := true
		for _, t := range feed.Tags {
			if tagSet[t] && !coveredByCustom[t] {
				alreadyCovered = false
				break
			}
		}
		if !alreadyCovered {
			result = append(result, feed)
		}
	}
	return result
}

func (f *Fetcher) fetchForLangTags(lang string, tags []string) []string {
	var result []string
	for _, feed := range f.feedsForWithCustom(lang, tags) {
		for _, title := range fetchFeed(feed.URL) {
			result = append(result, title)
			if len(result) >= maxHeadlines {
				return result
			}
		}
	}
	return result
}

type rssRoot struct {
	Channel struct {
		Items []struct {
			Title string `xml:"title"`
		} `xml:"item"`
	} `xml:"channel"`
}

// titleSuffixRe はタイトル末尾のサイト名サフィックスを除去する。
// 例: " - はてなブックマーク", " | TechCrunch", " - Gigazine" など
var titleSuffixRe = regexp.MustCompile(`\s*[-|｜]\s*(はてなブックマーク|Hacker News|TechCrunch|Gigazine|ITmedia|4Gamer|IGN|Natalie|AUTOMATON)[^-|｜]*$`)

func cleanTitle(s string) string {
	s = titleSuffixRe.ReplaceAllString(strings.TrimSpace(s), "")
	return strings.TrimSpace(s)
}

func fetchFeed(url string) []string {
	client := &http.Client{Timeout: fetchTimeout}
	resp, err := client.Get(url)
	if err != nil {
		return nil
	}
	defer resp.Body.Close()

	var root rssRoot
	if err := xml.NewDecoder(resp.Body).Decode(&root); err != nil {
		return nil
	}

	var titles []string
	for _, item := range root.Channel.Items {
		if t := cleanTitle(item.Title); t != "" {
			titles = append(titles, t)
		}
		if len(titles) >= maxHeadlines {
			break
		}
	}
	return titles
}
