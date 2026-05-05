package httpapi

import (
	"time"

	"github.com/agendash/AgenLeash/internal/model"
)

type SessionListResponse struct {
	Sessions []SessionSummary `json:"sessions"`
}

type StartSessionRequest struct {
	Adapter          string   `json:"adapter"`
	CWD              string   `json:"cwd"`
	AgentVersionHint string   `json:"agent_version_hint,omitempty"`
	StartMode        string   `json:"start_mode,omitempty"`
	ConversationID   string   `json:"conversation_id,omitempty"`
	Args             []string `json:"args,omitempty"`
}

type StartSessionResponse struct {
	SessionID string         `json:"session_id"`
	Session   SessionSummary `json:"session"`
}

type SessionDetailResponse struct {
	Session      SessionSummary              `json:"session"`
	Messages     []model.ConversationMessage `json:"messages,omitempty"`
	MessagesPage *MessagesPage               `json:"messages_page,omitempty"`
}

type MessagesPage struct {
	Limit      int  `json:"limit"`
	Offset     int  `json:"offset"`
	Returned   int  `json:"returned"`
	HasMore    bool `json:"has_more"`
	NextOffset int  `json:"next_offset,omitempty"`
}

type SessionMessageRequest struct {
	MessageID string `json:"message_id,omitempty"`
	Content   string `json:"content"`
}

type SessionMessageResponse struct {
	OK        bool   `json:"ok"`
	SessionID string `json:"session_id"`
	MessageID string `json:"message_id,omitempty"`
}

type FilePreviewResponse struct {
	Path      string `json:"path"`
	Name      string `json:"name"`
	MIMEType  string `json:"mime_type,omitempty"`
	Kind      string `json:"kind"`
	Language  string `json:"language,omitempty"`
	Size      int64  `json:"size"`
	Encoding  string `json:"encoding,omitempty"`
	Content   string `json:"content,omitempty"`
	Truncated bool   `json:"truncated,omitempty"`
}

type SessionSummary struct {
	ID                string                  `json:"id"`
	Adapter           string                  `json:"adapter"`
	Origin            model.SessionOrigin     `json:"origin,omitempty"`
	RuntimeMode       string                  `json:"runtime_mode,omitempty"`
	State             model.SessionState      `json:"state"`
	DetailPath        string                  `json:"detail_path,omitempty"`
	PID               int                     `json:"pid,omitempty"`
	CreatedAt         time.Time               `json:"created_at"`
	LastSeen          time.Time               `json:"last_seen"`
	ConnectedClients  int                     `json:"connected_clients"`
	Capabilities      model.Capabilities      `json:"capabilities"`
	Features          model.FeatureSet        `json:"features,omitempty"`
	Conversation      ConversationSummary     `json:"conversation"`
	Workspace         WorkspaceSummary        `json:"workspace"`
	Highlight         *model.SessionHighlight `json:"highlight,omitempty"`
	LastOutputPreview string                  `json:"last_output_preview,omitempty"`
}

type ConversationSummary struct {
	NativeID       string               `json:"native_id,omitempty"`
	StartMode      model.StartMode      `json:"start_mode"`
	ResumeStrategy model.ResumeStrategy `json:"resume_strategy"`
}

type WorkspaceSummary struct {
	CWD         string `json:"cwd,omitempty"`
	Root        string `json:"root,omitempty"`
	Fingerprint string `json:"fingerprint,omitempty"`
	GitRoot     string `json:"git_root,omitempty"`
	GitBranch   string `json:"git_branch,omitempty"`
}

type ErrorResponse struct {
	Error string `json:"error"`
}
