package model

import "time"

const (
	HighlightNeedsResponse    = "needs_response"
	HighlightNeedsAttention   = "needs_attention"
	HighlightRecentCompletion = "recent_completion"
	HighlightActiveNow        = "active_now"
	HighlightRecentUpdate     = "recent_update"
)

const (
	HighlightToneWarning = "warning"
	HighlightToneDanger  = "danger"
	HighlightToneInfo    = "info"
	HighlightToneSuccess = "success"
)

const (
	recentActivityWindow   = 20 * time.Minute
	recentCompletionWindow = 45 * time.Minute
)

type SessionHighlight struct {
	Kind           string    `json:"kind,omitempty"`
	Label          string    `json:"label,omitempty"`
	Detail         string    `json:"detail,omitempty"`
	Tone           string    `json:"tone,omitempty"`
	SortRank       int       `json:"sort_rank,omitempty"`
	RequiresAction bool      `json:"requires_action,omitempty"`
	ReviewRequired bool      `json:"review_required,omitempty"`
	RecentActivity bool      `json:"recent_activity,omitempty"`
	UpdatedAt      time.Time `json:"updated_at,omitempty"`
}

func HighlightForSession(session Session, now time.Time) *SessionHighlight {
	updatedAt := session.LastSeen
	if updatedAt.IsZero() {
		updatedAt = session.CreatedAt
	}
	if updatedAt.IsZero() {
		updatedAt = now
	}

	age := now.Sub(updatedAt)
	if age < 0 {
		age = 0
	}
	recentActivity := age <= recentActivityWindow

	switch session.State {
	case SessionStatePaused:
		return &SessionHighlight{
			Kind:           HighlightNeedsResponse,
			Label:          "Needs reply",
			Detail:         "Awaiting your next instruction, approval, or feedback.",
			Tone:           HighlightToneWarning,
			SortRank:       0,
			RequiresAction: true,
			ReviewRequired: true,
			RecentActivity: true,
			UpdatedAt:      updatedAt,
		}
	case SessionStateErrored:
		return &SessionHighlight{
			Kind:           HighlightNeedsAttention,
			Label:          "Needs attention",
			Detail:         "The agent hit an execution problem and should be checked.",
			Tone:           HighlightToneDanger,
			SortRank:       1,
			RequiresAction: true,
			ReviewRequired: true,
			RecentActivity: recentActivity,
			UpdatedAt:      updatedAt,
		}
	case SessionStateStopped:
		if age <= recentCompletionWindow {
			return &SessionHighlight{
				Kind:           HighlightRecentCompletion,
				Label:          "Just finished",
				Detail:         "Fresh output is ready for review or follow-up.",
				Tone:           HighlightToneInfo,
				SortRank:       2,
				ReviewRequired: true,
				RecentActivity: true,
				UpdatedAt:      updatedAt,
			}
		}
	case SessionStateRunning, SessionStatePending:
		return &SessionHighlight{
			Kind:           HighlightActiveNow,
			Label:          "Active",
			Detail:         "The agent is actively running in this workspace.",
			Tone:           HighlightToneSuccess,
			SortRank:       3,
			RecentActivity: true,
			UpdatedAt:      updatedAt,
		}
	}

	if recentActivity {
		return &SessionHighlight{
			Kind:           HighlightRecentUpdate,
			Label:          "Recent",
			Detail:         "Recently updated on this leash node.",
			Tone:           HighlightToneInfo,
			SortRank:       4,
			RecentActivity: true,
			UpdatedAt:      updatedAt,
		}
	}

	return nil
}
