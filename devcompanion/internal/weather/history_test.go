package weather

import (
	"os"
	"testing"
	"time"
)

func TestTempDiff(t *testing.T) {
	cases := []struct {
		label   string
		current int
		base    int
		wantNE  string // not empty
	}{
		{"昨日", 22, 15, "昨日より7°C暖かい"},
		{"昨日", 22, 20, "昨日より少し暖かい"},
		{"昨日", 22, 22, "昨日とほぼ同じ気温"},
		{"昨日", 22, 24, "昨日より少し寒い"},
		{"昨日", 22, 30, "昨日より8°C寒い"},
	}
	for _, c := range cases {
		got := tempDiff(c.label, c.current, c.base)
		if got != c.wantNE {
			t.Errorf("tempDiff(%q, %d, %d) = %q, want %q", c.label, c.current, c.base, got, c.wantNE)
		}
	}
}

func TestDeltaContext(t *testing.T) {
	dir, err := os.MkdirTemp("", "sakura-weather-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)

	// 履歴なし → 空
	if got := deltaContext(dir, 20); got != "" {
		t.Errorf("expected empty, got %q", got)
	}

	// 昨日の記録を挿入
	yesterday := time.Now().AddDate(0, 0, -1).Format("2006-01-02")
	records := []DailyRecord{{Date: yesterday, TempC: 15, Desc: "Cloudy"}}
	saveHistory(dir, records)

	// 今日 20°C → 昨日 15°C = +5°C
	got := deltaContext(dir, 20)
	if got != "昨日より5°C暖かい" {
		t.Errorf("unexpected delta: %q", got)
	}
}
