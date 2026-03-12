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

type WebSensor struct {
	pollInterval time.Duration
	lastURL      string
	lastTitle    string
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
			title, url, err := s.getActiveTab()
			if err != nil || url == "" {
				continue
			}

			if url != s.lastURL || title != s.lastTitle {
				s.lastURL = url
				s.lastTitle = title

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
		}
	}
}

func (s *WebSensor) getActiveTab() (string, string, error) {
	// macOS AppleScript to get the active tab URL and title from common browsers.
	script := `
	tell application "System Events"
		set frontApp to name of first application process whose frontmost is true
	end tell

	if frontApp is "Google Chrome" or frontApp is "Google Chrome Canary" or frontApp is "Brave Browser" then
		tell application frontApp
			set activeTab to active tab of front window
			return (title of activeTab) & "|||" & (URL of activeTab)
		end tell
	else if frontApp is "Safari" then
		tell application "Safari"
			set activeTab to current tab of front window
			return (name of activeTab) & "|||" & (URL of activeTab)
		end tell
	else if frontApp is "Arc" then
		tell application "Arc"
			tell front window
				set activeTab to active tab
				return (title of activeTab) & "|||" & (URL of activeTab)
			end tell
		end tell
	end if
	return ""
	`

	out, err := exec.Command("osascript", "-e", script).Output()
	if err != nil {
		return "", "", err
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
