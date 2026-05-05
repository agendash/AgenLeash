package httpapi

import (
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/agendash/AgenLeash/internal/model"
)

const (
	defaultStatsTopN = 15
	maxStatsTopN     = 100
)

type StatsResponse struct {
	GeneratedAt   time.Time        `json:"generated_at"`
	Totals        StatsTotals      `json:"totals"`
	Adapters      []AdapterStats   `json:"adapters"`
	Claudecode    *ClaudecodeStats `json:"claudecode,omitempty"`
	TopWorkspaces []WorkspaceStats `json:"top_workspaces,omitempty"`
}

type StatsTotals struct {
	Sessions            int `json:"sessions"`
	Managed             int `json:"managed"`
	Discovered          int `json:"discovered"`
	Pending             int `json:"pending"`
	Running             int `json:"running"`
	Paused              int `json:"paused"`
	Stopped             int `json:"stopped"`
	Errored             int `json:"errored"`
	ReviewRequired      int `json:"review_required"`
	RecentActivity      int `json:"recent_activity"`
	UniqueWorkspaces    int `json:"unique_workspaces"`
	UniqueConversations int `json:"unique_conversations"`
}

type AdapterStats struct {
	Adapter          string    `json:"adapter"`
	Sessions         int       `json:"sessions"`
	Managed          int       `json:"managed"`
	Discovered       int       `json:"discovered"`
	Pending          int       `json:"pending"`
	Running          int       `json:"running"`
	Paused           int       `json:"paused"`
	Stopped          int       `json:"stopped"`
	Errored          int       `json:"errored"`
	ReviewRequired   int       `json:"review_required"`
	RecentActivity   int       `json:"recent_activity"`
	UniqueWorkspaces int       `json:"unique_workspaces"`
	LastSeen         time.Time `json:"last_seen"`
}

type ClaudecodeStats struct {
	Sessions         int `json:"sessions"`
	PrimarySessions  int `json:"primary_sessions"`
	SubagentSessions int `json:"subagent_sessions"`
	UniqueWorkspaces int `json:"unique_workspaces"`
}

type WorkspaceStats struct {
	Path               string    `json:"path"`
	Sessions           int       `json:"sessions"`
	Managed            int       `json:"managed"`
	Discovered         int       `json:"discovered"`
	ClaudecodeSessions int       `json:"claudecode_sessions"`
	ClaudecodeSubagent int       `json:"claudecode_subagent_sessions"`
	LastSeen           time.Time `json:"last_seen"`
	Adapters           []string  `json:"adapters,omitempty"`
}

type adapterAccumulator struct {
	stats         AdapterStats
	workspaceSeen map[string]struct{}
}

type workspaceAccumulator struct {
	stats       WorkspaceStats
	adapterSeen map[string]struct{}
}

type claudecodeAccumulator struct {
	stats         ClaudecodeStats
	workspaceSeen map[string]struct{}
}

func ParseStatsTopN(req *http.Request) int {
	raw := strings.TrimSpace(req.URL.Query().Get("top"))
	if raw == "" {
		return defaultStatsTopN
	}
	value, err := parsePositiveInt(raw)
	if err != nil {
		return defaultStatsTopN
	}
	if value > maxStatsTopN {
		return maxStatsTopN
	}
	return value
}

func BuildStatsResponse(sessions []model.Session, now time.Time, topN int) StatsResponse {
	topN = normalizeStatsTopN(topN)
	resp := StatsResponse{
		GeneratedAt: now,
		Adapters:    make([]AdapterStats, 0),
	}

	workspaceSeen := make(map[string]struct{})
	conversationSeen := make(map[string]struct{})
	adapterAgg := make(map[string]*adapterAccumulator)
	workspaceAgg := make(map[string]*workspaceAccumulator)
	claudecodeAgg := &claudecodeAccumulator{
		workspaceSeen: make(map[string]struct{}),
	}

	for _, session := range sessions {
		highlight := model.HighlightForSession(session, now)
		workspaceKey := uniqueWorkspaceKey(session)
		conversationKey := strings.TrimSpace(session.ConversationID)
		adapterName := strings.TrimSpace(session.Adapter)
		if adapterName == "" {
			adapterName = "unknown"
		}

		resp.Totals.Sessions++
		switch session.Origin {
		case model.SessionOriginManaged:
			resp.Totals.Managed++
		case model.SessionOriginDiscovered:
			resp.Totals.Discovered++
		}
		switch session.State {
		case model.SessionStatePending:
			resp.Totals.Pending++
		case model.SessionStateRunning:
			resp.Totals.Running++
		case model.SessionStatePaused:
			resp.Totals.Paused++
		case model.SessionStateStopped:
			resp.Totals.Stopped++
		case model.SessionStateErrored:
			resp.Totals.Errored++
		}
		if highlight != nil {
			if highlight.ReviewRequired {
				resp.Totals.ReviewRequired++
			}
			if highlight.RecentActivity {
				resp.Totals.RecentActivity++
			}
		}
		if workspaceKey != "" {
			workspaceSeen[workspaceKey] = struct{}{}
		}
		if conversationKey != "" {
			conversationSeen[conversationKey] = struct{}{}
		}

		agg := adapterAgg[adapterName]
		if agg == nil {
			agg = &adapterAccumulator{
				stats: AdapterStats{
					Adapter: adapterName,
				},
				workspaceSeen: make(map[string]struct{}),
			}
			adapterAgg[adapterName] = agg
		}
		agg.stats.Sessions++
		switch session.Origin {
		case model.SessionOriginManaged:
			agg.stats.Managed++
		case model.SessionOriginDiscovered:
			agg.stats.Discovered++
		}
		switch session.State {
		case model.SessionStatePending:
			agg.stats.Pending++
		case model.SessionStateRunning:
			agg.stats.Running++
		case model.SessionStatePaused:
			agg.stats.Paused++
		case model.SessionStateStopped:
			agg.stats.Stopped++
		case model.SessionStateErrored:
			agg.stats.Errored++
		}
		if highlight != nil {
			if highlight.ReviewRequired {
				agg.stats.ReviewRequired++
			}
			if highlight.RecentActivity {
				agg.stats.RecentActivity++
			}
		}
		if !session.LastSeen.IsZero() && session.LastSeen.After(agg.stats.LastSeen) {
			agg.stats.LastSeen = session.LastSeen
		}
		if workspaceKey != "" {
			agg.workspaceSeen[workspaceKey] = struct{}{}
		}

		workspaceLabel := workspaceDisplayPath(session)
		wagg := workspaceAgg[workspaceLabel]
		if wagg == nil {
			wagg = &workspaceAccumulator{
				stats: WorkspaceStats{
					Path: workspaceLabel,
				},
				adapterSeen: make(map[string]struct{}),
			}
			workspaceAgg[workspaceLabel] = wagg
		}
		wagg.stats.Sessions++
		switch session.Origin {
		case model.SessionOriginManaged:
			wagg.stats.Managed++
		case model.SessionOriginDiscovered:
			wagg.stats.Discovered++
		}
		if adapterName == "claudecode" {
			wagg.stats.ClaudecodeSessions++
			if isClaudecodeSubagent(session) {
				wagg.stats.ClaudecodeSubagent++
			}
		}
		if !session.LastSeen.IsZero() && session.LastSeen.After(wagg.stats.LastSeen) {
			wagg.stats.LastSeen = session.LastSeen
		}
		wagg.adapterSeen[adapterName] = struct{}{}

		if adapterName == "claudecode" {
			claudecodeAgg.stats.Sessions++
			if isClaudecodeSubagent(session) {
				claudecodeAgg.stats.SubagentSessions++
			} else {
				claudecodeAgg.stats.PrimarySessions++
			}
			if workspaceKey != "" {
				claudecodeAgg.workspaceSeen[workspaceKey] = struct{}{}
			}
		}
	}

	resp.Totals.UniqueWorkspaces = len(workspaceSeen)
	resp.Totals.UniqueConversations = len(conversationSeen)

	for _, agg := range adapterAgg {
		agg.stats.UniqueWorkspaces = len(agg.workspaceSeen)
		resp.Adapters = append(resp.Adapters, agg.stats)
	}
	sort.SliceStable(resp.Adapters, func(i, j int) bool {
		if resp.Adapters[i].Sessions == resp.Adapters[j].Sessions {
			return resp.Adapters[i].Adapter < resp.Adapters[j].Adapter
		}
		return resp.Adapters[i].Sessions > resp.Adapters[j].Sessions
	})

	topWorkspaces := make([]WorkspaceStats, 0, len(workspaceAgg))
	for _, agg := range workspaceAgg {
		agg.stats.Adapters = setToSortedSlice(agg.adapterSeen)
		topWorkspaces = append(topWorkspaces, agg.stats)
	}
	sort.SliceStable(topWorkspaces, func(i, j int) bool {
		if topWorkspaces[i].Sessions == topWorkspaces[j].Sessions {
			if topWorkspaces[i].LastSeen.Equal(topWorkspaces[j].LastSeen) {
				return topWorkspaces[i].Path < topWorkspaces[j].Path
			}
			return topWorkspaces[i].LastSeen.After(topWorkspaces[j].LastSeen)
		}
		return topWorkspaces[i].Sessions > topWorkspaces[j].Sessions
	})
	if len(topWorkspaces) > topN {
		topWorkspaces = topWorkspaces[:topN]
	}
	resp.TopWorkspaces = topWorkspaces

	if claudecodeAgg.stats.Sessions > 0 {
		claudecodeAgg.stats.UniqueWorkspaces = len(claudecodeAgg.workspaceSeen)
		resp.Claudecode = &claudecodeAgg.stats
	}

	return resp
}

func normalizeStatsTopN(topN int) int {
	if topN <= 0 {
		return defaultStatsTopN
	}
	if topN > maxStatsTopN {
		return maxStatsTopN
	}
	return topN
}

func parsePositiveInt(raw string) (int, error) {
	value, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil {
		return 0, err
	}
	if value <= 0 {
		return 0, strconv.ErrSyntax
	}
	return value, nil
}

func uniqueWorkspaceKey(session model.Session) string {
	for _, value := range []string{session.WorkspacePath, session.WorkspaceRoot, session.WorkspaceID} {
		value = strings.TrimSpace(value)
		if value != "" {
			return value
		}
	}
	return ""
}

func workspaceDisplayPath(session model.Session) string {
	for _, value := range []string{session.WorkspacePath, session.WorkspaceRoot} {
		value = strings.TrimSpace(value)
		if value != "" {
			return value
		}
	}
	if value := strings.TrimSpace(session.WorkspaceID); value != "" {
		return value
	}
	return "(unknown workspace)"
}

func isClaudecodeSubagent(session model.Session) bool {
	path := strings.ToLower(strings.TrimSpace(session.DetailPath))
	if strings.Contains(path, "/subagents/") {
		return true
	}
	return strings.HasPrefix(strings.ToLower(strings.TrimSpace(session.NativeConversationID)), "agent-")
}

func setToSortedSlice(values map[string]struct{}) []string {
	if len(values) == 0 {
		return nil
	}
	out := make([]string, 0, len(values))
	for value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		out = append(out, value)
	}
	sort.Strings(out)
	return out
}
