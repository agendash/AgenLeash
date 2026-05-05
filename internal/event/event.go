package event

import (
	"encoding/base64"
	"time"

	"github.com/agendash/AgenLeash/internal/model"
)

const (
	MsgTypeSessionSnapshot   = "session_snapshot"
	MsgTypeMessageStarted    = "message_started"
	MsgTypeMessageDelta      = "message_delta"
	MsgTypeMessageCompleted  = "message_completed"
	MsgTypeInputRequested    = "input_requested"
	MsgTypeStateChanged      = "state_changed"
	MsgTypeConversationBound = "conversation_bound"
	MsgTypeWorkspaceUpdated  = "workspace_updated"
	MsgTypeSyncEnd           = "sync_end"
	MsgTypeRawChunk          = "raw_chunk"

	CommandUserMessage      = "user_message"
	CommandInterrupt        = "interrupt"
	CommandHeartbeat        = "heartbeat"
	CommandRequestRawStream = "request_raw_stream"
	CommandRuntimeResize    = "runtime_resize"
)

type Event struct {
	MsgType              string                  `json:"msg_type"`
	SessionID            string                  `json:"session_id,omitempty"`
	Adapter              string                  `json:"adapter,omitempty"`
	RuntimeMode          string                  `json:"runtime_mode,omitempty"`
	NativeConversationID string                  `json:"native_conversation_id,omitempty"`
	ResumeStrategy       string                  `json:"resume_strategy,omitempty"`
	StartMode            string                  `json:"start_mode,omitempty"`
	State                string                  `json:"state,omitempty"`
	MessageID            string                  `json:"message_id,omitempty"`
	Role                 string                  `json:"role,omitempty"`
	Delta                string                  `json:"delta,omitempty"`
	Content              string                  `json:"content,omitempty"`
	Reason               string                  `json:"reason,omitempty"`
	CWD                  string                  `json:"cwd,omitempty"`
	WorkspaceRoot        string                  `json:"workspace_root,omitempty"`
	WorkspaceFingerprint string                  `json:"workspace_fingerprint,omitempty"`
	GitRoot              string                  `json:"git_root,omitempty"`
	GitBranch            string                  `json:"git_branch,omitempty"`
	Source               string                  `json:"source,omitempty"`
	Raw                  string                  `json:"raw,omitempty"`
	Highlight            *model.SessionHighlight `json:"highlight,omitempty"`
	Timestamp            time.Time               `json:"timestamp,omitempty"`
}

type Command struct {
	MsgType   string `json:"msg_type"`
	MessageID string `json:"message_id,omitempty"`
	Content   string `json:"content,omitempty"`
	Enabled   bool   `json:"enabled,omitempty"`
	Width     int    `json:"width,omitempty"`
	Height    int    `json:"height,omitempty"`
}

func SessionSnapshot(session model.Session) Event {
	now := time.Now().UTC()
	return Event{
		MsgType:              MsgTypeSessionSnapshot,
		SessionID:            session.ID,
		Adapter:              session.Adapter,
		RuntimeMode:          session.RuntimeMode,
		NativeConversationID: session.NativeConversationID,
		StartMode:            string(session.StartMode),
		ResumeStrategy:       string(session.ResumeStrategy),
		State:                string(session.State),
		CWD:                  session.WorkspacePath,
		WorkspaceRoot:        session.WorkspaceRoot,
		WorkspaceFingerprint: session.WorkspaceFingerprint,
		GitRoot:              session.GitRoot,
		GitBranch:            session.GitBranch,
		Highlight:            model.HighlightForSession(session, now),
		Timestamp:            now,
	}
}

func StateChanged(sessionID string, state model.SessionState) Event {
	return Event{
		MsgType:   MsgTypeStateChanged,
		SessionID: sessionID,
		State:     string(state),
		Timestamp: time.Now().UTC(),
	}
}

func ConversationBound(session model.Session) Event {
	return Event{
		MsgType:              MsgTypeConversationBound,
		SessionID:            session.ID,
		Adapter:              session.Adapter,
		NativeConversationID: session.NativeConversationID,
		ResumeStrategy:       string(session.ResumeStrategy),
		Timestamp:            time.Now().UTC(),
	}
}

func WorkspaceUpdated(session model.Session) Event {
	return Event{
		MsgType:              MsgTypeWorkspaceUpdated,
		SessionID:            session.ID,
		CWD:                  session.WorkspacePath,
		WorkspaceRoot:        session.WorkspaceRoot,
		WorkspaceFingerprint: session.WorkspaceFingerprint,
		GitRoot:              session.GitRoot,
		GitBranch:            session.GitBranch,
		Timestamp:            time.Now().UTC(),
	}
}

func SyncEnd(sessionID string) Event {
	return Event{
		MsgType:   MsgTypeSyncEnd,
		SessionID: sessionID,
		Timestamp: time.Now().UTC(),
	}
}

func MessageStarted(sessionID, messageID, role string) Event {
	return Event{
		MsgType:   MsgTypeMessageStarted,
		SessionID: sessionID,
		MessageID: messageID,
		Role:      role,
		Timestamp: time.Now().UTC(),
	}
}

func MessageDelta(sessionID, messageID, role, delta string) Event {
	return Event{
		MsgType:   MsgTypeMessageDelta,
		SessionID: sessionID,
		MessageID: messageID,
		Role:      role,
		Delta:     delta,
		Timestamp: time.Now().UTC(),
	}
}

func MessageCompleted(sessionID, messageID, role string) Event {
	return Event{
		MsgType:   MsgTypeMessageCompleted,
		SessionID: sessionID,
		MessageID: messageID,
		Role:      role,
		Timestamp: time.Now().UTC(),
	}
}

func InputRequested(sessionID, reason string) Event {
	return Event{
		MsgType:   MsgTypeInputRequested,
		SessionID: sessionID,
		Reason:    reason,
		Timestamp: time.Now().UTC(),
	}
}

func RawChunk(sessionID, source string, payload []byte) Event {
	return Event{
		MsgType:   MsgTypeRawChunk,
		SessionID: sessionID,
		Source:    source,
		Raw:       base64.StdEncoding.EncodeToString(payload),
		Timestamp: time.Now().UTC(),
	}
}
