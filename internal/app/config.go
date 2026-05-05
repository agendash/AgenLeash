package app

import (
	"os"
	"path/filepath"
	"strings"
	"time"
)

type Config struct {
	Addr                   string
	Token                  string
	AllowNoToken           bool
	EnableWeb              bool
	DataDir                string
	AdapterDir             string
	LogLevel               string
	HistoryRefreshInterval time.Duration
	SessionPersistInterval time.Duration
	DiscoverClaude         bool
	DiscoverCodex          bool
	DiscoverOpencode       bool
	ClaudeHome             string
	CodexHome              string
	OpencodeHome           string
	AllowedWorkspaceRoots  []string
}

func LoadConfig() Config {
	cfg := Config{
		Addr:                   envOrDefault("AGENLEASH_ADDR", "0.0.0.0:8081"),
		Token:                  strings.TrimSpace(os.Getenv("AGENLEASH_TOKEN")),
		AllowNoToken:           envBoolOrDefault("AGENLEASH_ALLOW_NO_TOKEN", false),
		EnableWeb:              envBoolOrDefault("AGENLEASH_ENABLE_WEB", false),
		DataDir:                envOrDefault("AGENLEASH_DATA_DIR", filepath.Join("tmp", "agenleash")),
		AdapterDir:             envOrDefault("AGENLEASH_ADAPTER_DIR", filepath.Join("adapters")),
		LogLevel:               envOrDefault("AGENLEASH_LOG_LEVEL", "info"),
		HistoryRefreshInterval: envDurationOrDefault("AGENLEASH_HISTORY_REFRESH_INTERVAL", 30*time.Second),
		SessionPersistInterval: envDurationOrDefault("AGENLEASH_SESSION_PERSIST_INTERVAL", 2*time.Second),
		DiscoverClaude:         envBoolOrDefault("AGENLEASH_DISCOVER_CLAUDE", true),
		DiscoverCodex:          envBoolOrDefault("AGENLEASH_DISCOVER_CODEX", true),
		DiscoverOpencode:       envBoolOrDefault("AGENLEASH_DISCOVER_OPENCODE", true),
		ClaudeHome:             envOrDefault("AGENLEASH_CLAUDE_HOME", defaultHistoryRoot(".claude")),
		CodexHome:              envOrDefault("AGENLEASH_CODEX_HOME", defaultHistoryRoot(".codex")),
		OpencodeHome:           envOrDefault("AGENLEASH_OPENCODE_HOME", defaultXDGDataRoot("opencode")),
		AllowedWorkspaceRoots:  envPathList("AGENLEASH_ALLOWED_WORKSPACE_ROOTS"),
	}

	cfg.Addr = strings.TrimSpace(cfg.Addr)
	cfg.Token = strings.TrimSpace(cfg.Token)
	cfg.DataDir = filepath.Clean(strings.TrimSpace(cfg.DataDir))
	cfg.AdapterDir = filepath.Clean(strings.TrimSpace(cfg.AdapterDir))
	cfg.LogLevel = strings.ToLower(strings.TrimSpace(cfg.LogLevel))
	cfg.ClaudeHome = normalizeOptionalPath(cfg.ClaudeHome)
	cfg.CodexHome = normalizeOptionalPath(cfg.CodexHome)
	cfg.OpencodeHome = normalizeOptionalPath(cfg.OpencodeHome)
	cfg.AllowedWorkspaceRoots = normalizePathList(cfg.AllowedWorkspaceRoots)

	if cfg.Addr == "" {
		cfg.Addr = "0.0.0.0:8081"
	}
	if cfg.DataDir == "." || cfg.DataDir == "" {
		cfg.DataDir = filepath.Join("tmp", "agenleash")
	}
	if cfg.AdapterDir == "." || cfg.AdapterDir == "" {
		cfg.AdapterDir = filepath.Join("adapters")
	}
	if cfg.LogLevel == "" {
		cfg.LogLevel = "info"
	}
	if cfg.HistoryRefreshInterval < 0 {
		cfg.HistoryRefreshInterval = 30 * time.Second
	}
	if cfg.SessionPersistInterval < 0 {
		cfg.SessionPersistInterval = 2 * time.Second
	}

	return cfg
}

func envOrDefault(key, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	return fallback
}

func envBoolOrDefault(key string, fallback bool) bool {
	raw := strings.ToLower(strings.TrimSpace(os.Getenv(key)))
	switch raw {
	case "1", "true", "yes", "on":
		return true
	case "0", "false", "no", "off":
		return false
	default:
		return fallback
	}
}

func envPathList(key string) []string {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return nil
	}
	parts := strings.FieldsFunc(raw, func(r rune) bool {
		return r == ',' || r == '\n' || r == '\r' || r == os.PathListSeparator
	})
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		out = append(out, part)
	}
	return out
}

func defaultHistoryRoot(hiddenDir string) string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	home = strings.TrimSpace(home)
	if home == "" {
		return ""
	}
	return filepath.Join(home, hiddenDir)
}

func defaultXDGDataRoot(appName string) string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	home = strings.TrimSpace(home)
	if home == "" {
		return ""
	}
	if base := strings.TrimSpace(os.Getenv("XDG_DATA_HOME")); base != "" {
		return filepath.Join(base, appName)
	}
	return filepath.Join(home, ".local", "share", appName)
}

func normalizeOptionalPath(path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return ""
	}
	return filepath.Clean(path)
}

func normalizePathList(paths []string) []string {
	out := make([]string, 0, len(paths))
	seen := make(map[string]struct{}, len(paths))
	for _, path := range paths {
		path = normalizeOptionalPath(path)
		if path == "" {
			continue
		}
		if _, ok := seen[path]; ok {
			continue
		}
		seen[path] = struct{}{}
		out = append(out, path)
	}
	return out
}

func envDurationOrDefault(key string, fallback time.Duration) time.Duration {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return fallback
	}
	value, err := time.ParseDuration(raw)
	if err != nil {
		return fallback
	}
	return value
}
