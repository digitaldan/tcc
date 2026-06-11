package config

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
)

// Config is ctmux's user configuration, read from ~/.ctmux/config.toml.
// Only simple `key = "value"` lines are supported.
type Config struct {
	Prefix string // e.g. "ctrl+q", "C-a", "^b"
}

// LoadConfig reads the config file; missing file yields defaults.
func LoadConfig() Config {
	cfg := Config{Prefix: "ctrl+q"}
	dir, err := Dir()
	if err != nil {
		return cfg
	}
	f, err := os.Open(filepath.Join(dir, "config.toml"))
	if err != nil {
		return cfg
	}
	defer f.Close()

	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, val, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		val = strings.Trim(strings.TrimSpace(val), `"'`)
		if key == "prefix" {
			cfg.Prefix = val
		}
	}
	return cfg
}

// PrefixByte converts a prefix spec ("ctrl+q", "C-a", "^b") to its control
// byte. Returns 0 for unparseable specs (caller falls back to the default).
func (c Config) PrefixByte() byte {
	s := strings.ToLower(strings.TrimSpace(c.Prefix))
	for _, p := range []string{"ctrl+", "ctrl-", "c-", "^"} {
		if strings.HasPrefix(s, p) {
			s = strings.TrimPrefix(s, p)
			break
		}
	}
	if len(s) == 1 && s[0] >= 'a' && s[0] <= 'z' {
		return s[0] - 'a' + 1
	}
	return 0
}

// PrefixLabel renders the prefix for the UI, e.g. "^Q".
func (c Config) PrefixLabel() string {
	b := c.PrefixByte()
	if b == 0 {
		return "^Q"
	}
	return "^" + string('A'+rune(b)-1)
}
