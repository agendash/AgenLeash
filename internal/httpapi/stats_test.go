package httpapi

import (
	"testing"
	"time"

	"github.com/agendash/AgenLeash/internal/model"
)

func TestBuildStatsResponse(t *testing.T) {
	now := time.Date(2026, 4, 21, 6, 0, 0, 0, time.UTC)
	sessions := []model.Session{
		{
			ID:             "sess_claude_primary",
			Adapter:        "claudecode",
			Origin:         model.SessionOriginDiscovered,
			State:          model.SessionStateStopped,
			ConversationID: "conv_1",
			WorkspacePath:  "/workspaces/pokemon",
			DetailPath:     "/data/agents/.claude/projects/-home-you-Workspace-pokemon/abc.jsonl",
			CreatedAt:      now.Add(-2 * time.Hour),
			LastSeen:       now.Add(-15 * time.Minute),
		},
		{
			ID:                   "sess_claude_subagent",
			Adapter:              "claudecode",
			Origin:               model.SessionOriginDiscovered,
			State:                model.SessionStateStopped,
			ConversationID:       "conv_2",
			NativeConversationID: "agent-123",
			WorkspacePath:        "/workspaces/pokemon",
			DetailPath:           "/data/agents/.claude/projects/-home-you-Workspace-pokemon/abc/subagents/agent-123.jsonl",
			CreatedAt:            now.Add(-90 * time.Minute),
			LastSeen:             now.Add(-10 * time.Minute),
		},
		{
			ID:             "sess_codex",
			Adapter:        "codex",
			Origin:         model.SessionOriginDiscovered,
			State:          model.SessionStatePaused,
			ConversationID: "conv_3",
			WorkspacePath:  "/workspaces/tools",
			DetailPath:     "/data/agents/.codex/state_5.sqlite#threads/thread-1",
			CreatedAt:      now.Add(-45 * time.Minute),
			LastSeen:       now.Add(-5 * time.Minute),
		},
		{
			ID:             "sess_managed",
			Adapter:        "codex",
			Origin:         model.SessionOriginManaged,
			State:          model.SessionStateRunning,
			ConversationID: "conv_4",
			WorkspacePath:  "/workspaces/tools",
			CreatedAt:      now.Add(-30 * time.Minute),
			LastSeen:       now.Add(-2 * time.Minute),
		},
	}

	stats := BuildStatsResponse(sessions, now, 10)
	if stats.Totals.Sessions != 4 {
		t.Fatalf("total sessions = %d, want 4", stats.Totals.Sessions)
	}
	if stats.Totals.Discovered != 3 || stats.Totals.Managed != 1 {
		t.Fatalf("origins = %#v", stats.Totals)
	}
	if stats.Totals.UniqueWorkspaces != 2 {
		t.Fatalf("unique workspaces = %d, want 2", stats.Totals.UniqueWorkspaces)
	}
	if stats.Claudecode == nil {
		t.Fatal("claudecode stats = nil")
	}
	if stats.Claudecode.Sessions != 2 || stats.Claudecode.PrimarySessions != 1 || stats.Claudecode.SubagentSessions != 1 {
		t.Fatalf("claudecode stats = %#v", stats.Claudecode)
	}
	if len(stats.Adapters) != 2 {
		t.Fatalf("adapters = %d, want 2", len(stats.Adapters))
	}
	adapterCounts := make(map[string]int, len(stats.Adapters))
	for _, item := range stats.Adapters {
		adapterCounts[item.Adapter] = item.Sessions
	}
	if adapterCounts["codex"] != 2 || adapterCounts["claudecode"] != 2 {
		t.Fatalf("adapter counts = %#v", adapterCounts)
	}
	if len(stats.TopWorkspaces) != 2 {
		t.Fatalf("top workspaces = %d, want 2", len(stats.TopWorkspaces))
	}
	var pokemon WorkspaceStats
	foundPokemon := false
	for _, item := range stats.TopWorkspaces {
		if item.Path == "/workspaces/pokemon" {
			pokemon = item
			foundPokemon = true
			break
		}
	}
	if !foundPokemon {
		t.Fatalf("pokemon workspace missing from %#v", stats.TopWorkspaces)
	}
	if pokemon.ClaudecodeSubagent != 1 {
		t.Fatalf("claude subagents = %d, want 1", pokemon.ClaudecodeSubagent)
	}
}
