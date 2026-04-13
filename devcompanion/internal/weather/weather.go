package weather

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"sync"
	"time"
)

const (
	cacheTTL     = 3 * time.Hour
	fetchTimeout = 5 * time.Second
)

// Info は取得した天気情報を保持する。
type Info struct {
	City  string
	TempC int
	Desc  string // 天気の説明 (English from wttr.in)
}

// String は天気情報を1行テキストに変換する（LLMへのコンテキストとして使用）。
func (i *Info) String() string {
	return fmt.Sprintf("%s: %s, %d°C", i.City, i.Desc, i.TempC)
}

// Fetcher はIPジオロケーション + wttr.in による天気情報を取得・キャッシュする。
// 位置情報が取れない場合は天気発言をスキップする設計。
type Fetcher struct {
	mu      sync.RWMutex
	cache   *Info
	cacheAt time.Time
	dataDir string // 履歴ファイルの保存先（空の場合は履歴を記録しない）
}

func NewFetcher() *Fetcher {
	return &Fetcher{}
}

// NewFetcherWithHistory は天気履歴を dataDir に保存する Fetcher を返す。
func NewFetcherWithHistory(dataDir string) *Fetcher {
	return &Fetcher{dataDir: dataDir}
}

// Get は天気情報を返す。
// overrideCity が空でない場合は IP ジオロケーションをスキップしてその都市名を使う。
// 位置情報の取得に失敗した場合、またはAPIエラーの場合は (nil, false) を返す。
// キャッシュが有効な場合はネットワークアクセスなしで返す。
func (f *Fetcher) Get(overrideCity string) (*Info, bool) {
	f.mu.RLock()
	if f.cache != nil && time.Since(f.cacheAt) < cacheTTL {
		info := *f.cache
		f.mu.RUnlock()
		return &info, true
	}
	f.mu.RUnlock()

	f.mu.Lock()
	defer f.mu.Unlock()
	// ロック取得後に再確認
	if f.cache != nil && time.Since(f.cacheAt) < cacheTTL {
		info := *f.cache
		return &info, true
	}

	city := overrideCity
	if city == "" {
		var ok bool
		city, ok = detectCity()
		if !ok {
			return nil, false
		}
	}
	info, ok := fetchWeather(city)
	if !ok {
		return nil, false
	}
	f.cache = info
	f.cacheAt = time.Now()
	if f.dataDir != "" {
		recordToday(f.dataDir, info)
	}
	return info, true
}

// DeltaContext は現在の気温と保存済み履歴を比較した差分テキストを返す。
// 履歴がない・差が小さい場合は空文字を返す。
func (f *Fetcher) DeltaContext(currentTempC int) string {
	if f.dataDir == "" {
		return ""
	}
	return deltaContext(f.dataDir, currentTempC)
}

// ipLocation はIPジオロケーションAPIのレスポンス（必要フィールドのみ）。
type ipLocation struct {
	Status string `json:"status"`
	City   string `json:"city"`
}

func detectCity() (string, bool) {
	client := &http.Client{Timeout: fetchTimeout}
	resp, err := client.Get("http://ip-api.com/json?fields=status,city&lang=en")
	if err != nil {
		return "", false
	}
	defer resp.Body.Close()

	var loc ipLocation
	if err := json.NewDecoder(resp.Body).Decode(&loc); err != nil {
		return "", false
	}
	if loc.Status != "success" || loc.City == "" {
		return "", false
	}
	return loc.City, true
}

// wttrResponse は wttr.in の JSON レスポンス（必要な部分のみ）。
type wttrResponse struct {
	CurrentCondition []struct {
		TempC       string `json:"temp_C"`
		WeatherDesc []struct {
			Value string `json:"value"`
		} `json:"weatherDesc"`
	} `json:"current_condition"`
}

func fetchWeather(city string) (*Info, bool) {
	client := &http.Client{Timeout: fetchTimeout}
	apiURL := "https://wttr.in/" + url.PathEscape(city) + "?format=j1"
	resp, err := client.Get(apiURL)
	if err != nil {
		return nil, false
	}
	defer resp.Body.Close()

	var wr wttrResponse
	if err := json.NewDecoder(resp.Body).Decode(&wr); err != nil {
		return nil, false
	}
	if len(wr.CurrentCondition) == 0 {
		return nil, false
	}
	cc := wr.CurrentCondition[0]
	desc := ""
	if len(cc.WeatherDesc) > 0 {
		desc = cc.WeatherDesc[0].Value
	}
	var tempC int
	fmt.Sscan(cc.TempC, &tempC)
	return &Info{
		City:  city,
		TempC: tempC,
		Desc:  desc,
	}, true
}
