package model

import (
	"testing"
	"time"
)

func TestHighlightForSessionPausedNeedsReply(t *testing.T) {
	now := time.Date(2026, 4, 20, 5, 0, 0, 0, time.UTC)
	highlight := HighlightForSession(Session{
		ID:        "sess-1",
		State:     SessionStatePaused,
		CreatedAt: now.Add(-10 * time.Minute),
		LastSeen:  now.Add(-30 * time.Second),
	}, now)

	if highlight == nil {
		t.Fatal("expected highlight")
	}
	if highlight.Kind != HighlightNeedsResponse {
		t.Fatalf("kind = %q, want %q", highlight.Kind, HighlightNeedsResponse)
	}
	if !highlight.RequiresAction {
		t.Fatal("expected requires_action")
	}
}

func TestHighlightForSessionRecentCompletionNeedsReview(t *testing.T) {
	now := time.Date(2026, 4, 20, 5, 0, 0, 0, time.UTC)
	highlight := HighlightForSession(Session{
		ID:        "sess-2",
		State:     SessionStateStopped,
		CreatedAt: now.Add(-2 * time.Hour),
		LastSeen:  now.Add(-12 * time.Minute),
	}, now)

	if highlight == nil {
		t.Fatal("expected highlight")
	}
	if highlight.Kind != HighlightRecentCompletion {
		t.Fatalf("kind = %q, want %q", highlight.Kind, HighlightRecentCompletion)
	}
	if !highlight.ReviewRequired {
		t.Fatal("expected review_required")
	}
}
