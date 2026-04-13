package i18n

import (
	"embed"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"gopkg.in/yaml.v3"
)

//go:embed locales/*.yaml
var localeFS embed.FS

type LocaleData map[string]interface{}

var (
	locales     = make(map[string]LocaleData)
	overrideDir string // if set, try reading from here first (dev hot-reload)
	mu          sync.RWMutex
)

// SetOverrideDir sets a directory to read locale files from in preference to the embedded FS.
// Pass an empty string to disable. Automatically calls Reload for all known languages.
func SetOverrideDir(dir string) {
	mu.Lock()
	overrideDir = dir
	// Clear cache so files are re-read from the new location
	locales = make(map[string]LocaleData)
	mu.Unlock()
}

// Reload clears the cached locale data for the given languages (or all languages if none given).
// The next call to T/TVariant will re-read from disk (or embedded FS).
func Reload(langs ...string) {
	mu.Lock()
	if len(langs) == 0 {
		locales = make(map[string]LocaleData)
	} else {
		for _, lang := range langs {
			delete(locales, lang)
		}
	}
	mu.Unlock()
}

// T returns the translated string for the given language and key.
func T(lang, key string) string {
	mu.RLock()
	data, ok := locales[lang]
	mu.RUnlock()

	if !ok {
		// Load on demand
		if err := loadLocale(lang); err != nil {
			return key
		}
		mu.RLock()
		data = locales[lang]
		mu.RUnlock()
	}

	val := getNested(data, key)
	if val == "" {
		// Fallback to ja if not found in requested lang
		if lang != "ja" {
			return T("ja", key)
		}
		return key
	}
	return val
}

// TVariant returns a random variant for the given key.
func TVariant(lang, key string) []string {
	mu.RLock()
	data, ok := locales[lang]
	mu.RUnlock()

	if !ok {
		if err := loadLocale(lang); err != nil {
			return []string{key}
		}
		mu.RLock()
		data = locales[lang]
		mu.RUnlock()
	}

	val := getNestedRaw(data, key)
	if val == nil {
		if lang != "ja" {
			return TVariant("ja", key)
		}
		return []string{key}
	}

	switch v := val.(type) {
	case []interface{}:
		res := make([]string, len(v))
		for i, s := range v {
			res[i] = fmt.Sprint(s)
		}
		return res
	case string:
		return []string{v}
	default:
		return []string{fmt.Sprint(v)}
	}
}

func loadLocale(lang string) error {
	mu.Lock()
	defer mu.Unlock()

	if _, ok := locales[lang]; ok {
		return nil
	}

	var raw []byte

	// Try override directory first (dev hot-reload)
	if overrideDir != "" {
		diskPath := filepath.Join(overrideDir, lang+".yaml")
		if b, err := os.ReadFile(diskPath); err == nil {
			raw = b
		}
	}

	// Fall back to embedded FS
	if len(raw) == 0 {
		b, err := localeFS.ReadFile(fmt.Sprintf("locales/%s.yaml", lang))
		if err != nil {
			return err
		}
		raw = b
	}

	var localeData LocaleData
	if err := yaml.Unmarshal(raw, &localeData); err != nil {
		return err
	}

	locales[lang] = localeData
	return nil
}

func getNested(data LocaleData, key string) string {
	val := getNestedRaw(data, key)
	if s, ok := val.(string); ok {
		return s
	}
	return ""
}

func getNestedRaw(data LocaleData, key string) interface{} {
	parts := strings.Split(key, ".")
	var current interface{} = data

	for _, part := range parts {
		if m, ok := current.(LocaleData); ok {
			current = m[part]
		} else if m, ok := current.(map[string]interface{}); ok {
			current = m[part]
		} else {
			return nil
		}
	}
	return current
}
