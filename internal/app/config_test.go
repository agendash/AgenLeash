package app

import (
	"strings"
	"testing"
	"time"
)

func TestLoadConfigDefaults(t *testing.T) {
	t.Setenv("AGENLEASH_ADDR", "")
	t.Setenv("AGENLEASH_TOKEN", "")
	t.Setenv("AGENLEASH_DATA_DIR", "")
	t.Setenv("AGENLEASH_ADAPTER_DIR", "")
	t.Setenv("AGENLEASH_LOG_LEVEL", "")
	t.Setenv("AGENLEASH_HISTORY_REFRESH_INTERVAL", "")
	t.Setenv("AGENLEASH_SESSION_PERSIST_INTERVAL", "")
	t.Setenv("AGENLEASH_ALLOW_NO_TOKEN", "")
	t.Setenv("AGENLEASH_ENABLE_WEB", "")
	t.Setenv("AGENLEASH_DISCOVER_CLAUDE", "")
	t.Setenv("AGENLEASH_DISCOVER_CODEX", "")
	t.Setenv("AGENLEASH_DISCOVER_OPENCODE", "")
	t.Setenv("AGENLEASH_CLAUDE_HOME", "")
	t.Setenv("AGENLEASH_CODEX_HOME", "")
	t.Setenv("AGENLEASH_OPENCODE_HOME", "")
	t.Setenv("AGENLEASH_ALLOWED_WORKSPACE_ROOTS", "")

	cfg := LoadConfig()

	if cfg.Addr != "0.0.0.0:8081" {
		t.Fatalf("Addr = %q, want default", cfg.Addr)
	}
	if cfg.Token != "" {
		t.Fatalf("Token = %q, want empty by default", cfg.Token)
	}
	if cfg.AllowNoToken {
		t.Fatalf("AllowNoToken = true, want false")
	}
	if cfg.EnableWeb {
		t.Fatalf("EnableWeb = true, want false")
	}
	if cfg.DataDir != "tmp/agenleash" {
		t.Fatalf("DataDir = %q, want tmp/agenleash", cfg.DataDir)
	}
	if cfg.AdapterDir != "adapters" {
		t.Fatalf("AdapterDir = %q, want adapters", cfg.AdapterDir)
	}
	if cfg.LogLevel != "info" {
		t.Fatalf("LogLevel = %q, want info", cfg.LogLevel)
	}
	if cfg.HistoryRefreshInterval != 30*time.Second {
		t.Fatalf("HistoryRefreshInterval = %s, want 30s", cfg.HistoryRefreshInterval)
	}
	if cfg.SessionPersistInterval != 2*time.Second {
		t.Fatalf("SessionPersistInterval = %s, want 2s", cfg.SessionPersistInterval)
	}
	if !cfg.DiscoverClaude {
		t.Fatalf("DiscoverClaude = false, want true")
	}
	if !cfg.DiscoverCodex {
		t.Fatalf("DiscoverCodex = false, want true")
	}
	if !cfg.DiscoverOpencode {
		t.Fatalf("DiscoverOpencode = false, want true")
	}
	if cfg.ClaudeHome == "" {
		t.Fatalf("ClaudeHome = empty, want default path")
	}
	if cfg.CodexHome == "" {
		t.Fatalf("CodexHome = empty, want default path")
	}
	if cfg.OpencodeHome == "" {
		t.Fatalf("OpencodeHome = empty, want default path")
	}
	if !strings.HasSuffix(cfg.ClaudeHome, "/.claude") && cfg.ClaudeHome != ".claude" {
		t.Fatalf("ClaudeHome = %q, want a .claude path", cfg.ClaudeHome)
	}
	if !strings.HasSuffix(cfg.CodexHome, "/.codex") && cfg.CodexHome != ".codex" {
		t.Fatalf("CodexHome = %q, want a .codex path", cfg.CodexHome)
	}
	if !strings.HasSuffix(cfg.OpencodeHome, "/opencode") && cfg.OpencodeHome != "opencode" {
		t.Fatalf("OpencodeHome = %q, want an opencode path", cfg.OpencodeHome)
	}
	if len(cfg.AllowedWorkspaceRoots) != 0 {
		t.Fatalf("AllowedWorkspaceRoots = %v, want none", cfg.AllowedWorkspaceRoots)
	}
}

func TestLoadConfigCustomIntervals(t *testing.T) {
	t.Setenv("AGENLEASH_HISTORY_REFRESH_INTERVAL", "45s")
	t.Setenv("AGENLEASH_SESSION_PERSIST_INTERVAL", "750ms")

	cfg := LoadConfig()

	if cfg.HistoryRefreshInterval != 45*time.Second {
		t.Fatalf("HistoryRefreshInterval = %s, want 45s", cfg.HistoryRefreshInterval)
	}
	if cfg.SessionPersistInterval != 750*time.Millisecond {
		t.Fatalf("SessionPersistInterval = %s, want 750ms", cfg.SessionPersistInterval)
	}
}

func TestLoadConfigInvalidIntervalsFallback(t *testing.T) {
	t.Setenv("AGENLEASH_HISTORY_REFRESH_INTERVAL", "not-a-duration")
	t.Setenv("AGENLEASH_SESSION_PERSIST_INTERVAL", "still-bad")

	cfg := LoadConfig()

	if cfg.HistoryRefreshInterval != 30*time.Second {
		t.Fatalf("HistoryRefreshInterval = %s, want fallback 30s", cfg.HistoryRefreshInterval)
	}
	if cfg.SessionPersistInterval != 2*time.Second {
		t.Fatalf("SessionPersistInterval = %s, want fallback 2s", cfg.SessionPersistInterval)
	}
}

func TestLoadConfigCustomPathAndBooleanSettings(t *testing.T) {
	t.Setenv("AGENLEASH_ALLOW_NO_TOKEN", "true")
	t.Setenv("AGENLEASH_ENABLE_WEB", "true")
	t.Setenv("AGENLEASH_DISCOVER_CLAUDE", "false")
	t.Setenv("AGENLEASH_DISCOVER_CODEX", "1")
	t.Setenv("AGENLEASH_DISCOVER_OPENCODE", "1")
	t.Setenv("AGENLEASH_CLAUDE_HOME", "/srv/agents/.claude")
	t.Setenv("AGENLEASH_CODEX_HOME", "/srv/agents/.codex")
	t.Setenv("AGENLEASH_OPENCODE_HOME", "/srv/agents/opencode")
	t.Setenv("AGENLEASH_ALLOWED_WORKSPACE_ROOTS", "/workspace,/srv/repos\n/tmp/project")

	cfg := LoadConfig()

	if !cfg.AllowNoToken {
		t.Fatalf("AllowNoToken = false, want true")
	}
	if !cfg.EnableWeb {
		t.Fatalf("EnableWeb = false, want true")
	}
	if cfg.DiscoverClaude {
		t.Fatalf("DiscoverClaude = true, want false")
	}
	if !cfg.DiscoverCodex {
		t.Fatalf("DiscoverCodex = false, want true")
	}
	if !cfg.DiscoverOpencode {
		t.Fatalf("DiscoverOpencode = false, want true")
	}
	if cfg.ClaudeHome != "/srv/agents/.claude" {
		t.Fatalf("ClaudeHome = %q, want /srv/agents/.claude", cfg.ClaudeHome)
	}
	if cfg.CodexHome != "/srv/agents/.codex" {
		t.Fatalf("CodexHome = %q, want /srv/agents/.codex", cfg.CodexHome)
	}
	if cfg.OpencodeHome != "/srv/agents/opencode" {
		t.Fatalf("OpencodeHome = %q, want /srv/agents/opencode", cfg.OpencodeHome)
	}
	wantRoots := []string{"/workspace", "/srv/repos", "/tmp/project"}
	if len(cfg.AllowedWorkspaceRoots) != len(wantRoots) {
		t.Fatalf("AllowedWorkspaceRoots = %v, want %v", cfg.AllowedWorkspaceRoots, wantRoots)
	}
	for i := range wantRoots {
		if cfg.AllowedWorkspaceRoots[i] != wantRoots[i] {
			t.Fatalf("AllowedWorkspaceRoots[%d] = %q, want %q", i, cfg.AllowedWorkspaceRoots[i], wantRoots[i])
		}
	}
}
