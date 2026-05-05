package model

import "time"

type SessionState string

const (
	SessionStatePending SessionState = "pending"
	SessionStateRunning SessionState = "running"
	SessionStatePaused  SessionState = "paused"
	SessionStateStopped SessionState = "stopped"
	SessionStateErrored SessionState = "errored"
	SessionStateUnknown SessionState = "unknown"
)

type SessionOrigin string

const (
	SessionOriginManaged    SessionOrigin = "managed"
	SessionOriginDiscovered SessionOrigin = "discovered"
)

type StartMode string

const (
	StartModeNew    StartMode = "new"
	StartModeResume StartMode = "resume"
)

type ResumeStrategy string

const (
	ResumeStrategyProcessOnly ResumeStrategy = "process_only"
	ResumeStrategyNativeID    ResumeStrategy = "native_id"
	ResumeStrategyHybrid      ResumeStrategy = "hybrid"
	ResumeStrategyUnknown     ResumeStrategy = "unknown"
)

type Capabilities struct {
	SupportsResume             bool `json:"supports_resume"`
	SupportsInterrupt          bool `json:"supports_interrupt"`
	RequiresTTY                bool `json:"requires_tty"`
	RequiresRuntimeResize      bool `json:"requires_runtime_resize"`
	SupportsRawDebug           bool `json:"supports_raw_debug"`
	SupportsWorkspaceSwitch    bool `json:"supports_workspace_switch"`
	SupportsNativeConversation bool `json:"supports_native_conversation_id"`
}

type FeatureSet map[string]bool

func (f FeatureSet) Clone() FeatureSet {
	if f == nil {
		return nil
	}

	out := make(FeatureSet, len(f))
	for key, value := range f {
		out[key] = value
	}
	return out
}

type Session struct {
	ID                   string         `json:"id"`
	Adapter              string         `json:"adapter"`
	Origin               SessionOrigin  `json:"origin,omitempty"`
	RuntimeMode          string         `json:"runtime_mode,omitempty"`
	NativeConversationID string         `json:"native_conversation_id,omitempty"`
	StartMode            StartMode      `json:"start_mode"`
	ResumeStrategy       ResumeStrategy `json:"resume_strategy"`
	WorkspacePath        string         `json:"workspace_path,omitempty"`
	WorkspaceRoot        string         `json:"workspace_root,omitempty"`
	WorkspaceFingerprint string         `json:"workspace_fingerprint,omitempty"`
	GitRoot              string         `json:"git_root,omitempty"`
	GitBranch            string         `json:"git_branch,omitempty"`
	State                SessionState   `json:"state"`
	ConversationID       string         `json:"conversation_id,omitempty"`
	WorkspaceID          string         `json:"workspace_id,omitempty"`
	DetailPath           string         `json:"detail_path,omitempty"`
	LastOutputPreview    string         `json:"last_output_preview,omitempty"`
	Capabilities         Capabilities   `json:"capabilities"`
	Features             FeatureSet     `json:"features,omitempty"`
	CreatedAt            time.Time      `json:"created_at"`
	LastSeen             time.Time      `json:"last_seen"`
}

type Workspace struct {
	ID              string    `json:"id"`
	Path            string    `json:"path"`
	Root            string    `json:"root,omitempty"`
	Fingerprint     string    `json:"fingerprint,omitempty"`
	GitRoot         string    `json:"git_root,omitempty"`
	GitBranch       string    `json:"git_branch,omitempty"`
	ActiveSessionID string    `json:"active_session_id,omitempty"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}

type Conversation struct {
	ID          string    `json:"id"`
	SessionID   string    `json:"session_id"`
	NativeID    string    `json:"native_id,omitempty"`
	WorkspaceID string    `json:"workspace_id,omitempty"`
	State       string    `json:"state,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type ConversationMessage struct {
	ID             string            `json:"id"`
	ConversationID string            `json:"conversation_id,omitempty"`
	Role           string            `json:"role"`
	Content        string            `json:"content"`
	Status         string            `json:"status,omitempty"`
	CreatedAt      time.Time         `json:"created_at,omitempty"`
	UpdatedAt      time.Time         `json:"updated_at,omitempty"`
	ToolUses       []ToolUse         `json:"tool_uses,omitempty"`
	Metadata       map[string]string `json:"metadata,omitempty"`
}

type ToolUse struct {
	ID          string         `json:"id"`
	Name        string         `json:"name"`
	State       string         `json:"state,omitempty"`
	Description string         `json:"description,omitempty"`
	Input       map[string]any `json:"input,omitempty"`
	Output      map[string]any `json:"output,omitempty"`
	StartedAt   time.Time      `json:"started_at,omitempty"`
	CompletedAt time.Time      `json:"completed_at,omitempty"`
	ArtifactIDs []string       `json:"artifact_ids,omitempty"`
}
