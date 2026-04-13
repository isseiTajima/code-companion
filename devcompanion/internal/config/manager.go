package config

import (
	"fmt"
	"os"
	"path/filepath"
)

// Manager handles the persistence of configuration and OS-level integrations.
type Manager struct {
	configPath string
}

func NewManager() (*Manager, error) {
	path, err := DefaultConfigPath()
	if err != nil {
		return nil, err
	}
	return &Manager{configPath: path}, nil
}

func (m *Manager) Load() (*AppConfig, error) {
	return LoadConfig()
}

func (m *Manager) Save(cfg *Config) error {
	if err := Save(cfg, m.configPath); err != nil {
		return err
	}
	return m.UpdateAutoStart(cfg.AutoStart)
}

// UpdateAutoStart enables or disables the application to start at login (macOS only).
func (m *Manager) UpdateAutoStart(enabled bool) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}

	plistDir := filepath.Join(home, "Library", "LaunchAgents")
	plistPath := filepath.Join(plistDir, "com.sakura-kodama.plist")

	if !enabled {
		if _, err := os.Stat(plistPath); err == nil {
			return os.Remove(plistPath)
		}
		return nil
	}

	execPath, err := os.Executable()
	if err != nil {
		return err
	}

	if err := os.MkdirAll(plistDir, 0755); err != nil {
		return err
	}

	plistContent := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>Label</key>
	<string>com.sakura-kodama</string>
	<key>ProgramArguments</key>
	<array>
		<string>%s</string>
	</array>
	<key>RunAtLoad</key>
	<true/>
</dict>
</plist>`, execPath)

	return os.WriteFile(plistPath, []byte(plistContent), 0644)
}
