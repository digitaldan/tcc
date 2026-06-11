// Package config centralizes filesystem paths for ctmux and Claude Code.
package config

import (
	"os"
	"path/filepath"
)

// Dir returns ctmux's data directory (~/.ctmux), creating it if needed.
func Dir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	d := filepath.Join(home, ".ctmux")
	if err := os.MkdirAll(d, 0o755); err != nil {
		return "", err
	}
	return d, nil
}

// StateDir returns the directory where hook invocations write per-tab state
// files, creating it if needed.
func StateDir() (string, error) {
	d, err := Dir()
	if err != nil {
		return "", err
	}
	sd := filepath.Join(d, "state")
	if err := os.MkdirAll(sd, 0o755); err != nil {
		return "", err
	}
	return sd, nil
}

// HooksSettingsPath returns the path of the settings file ctmux passes to
// claude via --settings.
func HooksSettingsPath() (string, error) {
	d, err := Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(d, "hooks-settings.json"), nil
}

// ClaudeConfigDir returns Claude Code's config directory, honoring
// CLAUDE_CONFIG_DIR.
func ClaudeConfigDir() string {
	if d := os.Getenv("CLAUDE_CONFIG_DIR"); d != "" {
		return d
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".claude")
}
