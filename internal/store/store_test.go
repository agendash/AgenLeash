package store

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/agendash/AgenLeash/internal/model"
)

func TestFileStoreRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")

	fs, err := NewFileStore(path)
	if err != nil {
		t.Fatalf("NewFileStore: %v", err)
	}

	session := model.Session{
		ID:             "sess-1",
		Adapter:        "claudecode",
		StartMode:      model.StartModeNew,
		ResumeStrategy: model.ResumeStrategyProcessOnly,
		State:          model.SessionStateRunning,
		CreatedAt:      time.Now().UTC(),
		LastSeen:       time.Now().UTC(),
	}

	if err := fs.UpsertSession(session); err != nil {
		t.Fatalf("UpsertSession: %v", err)
	}

	if err := fs.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	reopened, err := NewFileStore(path)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}

	got, ok := reopened.GetSession("sess-1")
	if !ok {
		t.Fatalf("expected session to exist")
	}
	if got.Adapter != "claudecode" {
		t.Fatalf("Adapter = %q, want claudecode", got.Adapter)
	}
}

func TestMemoryStoreRejectsEmptyIDs(t *testing.T) {
	ms := NewMemoryStore()

	if err := ms.UpsertSession(model.Session{}); err == nil {
		t.Fatalf("expected error for empty session id")
	}
	if err := ms.UpsertWorkspace(model.Workspace{}); err == nil {
		t.Fatalf("expected error for empty workspace id")
	}
	if err := ms.UpsertConversation(model.Conversation{}); err == nil {
		t.Fatalf("expected error for empty conversation id")
	}
}

func TestSQLiteStoreRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.sqlite")

	st, err := NewSQLiteStore(path)
	if err != nil {
		t.Fatalf("NewSQLiteStore: %v", err)
	}

	createdAt := time.Now().UTC().Add(-5 * time.Minute).Round(0)
	lastSeen := createdAt.Add(2 * time.Minute)
	session := model.Session{
		ID:                   "sess-1",
		Adapter:              "claudecode",
		NativeConversationID: "native-conv-1",
		StartMode:            model.StartModeResume,
		ResumeStrategy:       model.ResumeStrategyNativeID,
		State:                model.SessionStateStopped,
		WorkspaceID:          "ws-1",
		WorkspacePath:        "/tmp/agen/workspace-a",
		DetailPath:           "/tmp/agen/workspace-a/.claude/session.jsonl",
		CreatedAt:            createdAt,
		LastSeen:             lastSeen,
	}
	workspace := model.Workspace{
		ID:        "ws-1",
		Path:      "/tmp/agen/workspace-a",
		CreatedAt: createdAt,
		UpdatedAt: lastSeen,
	}
	conversation := model.Conversation{
		ID:          "conv-1",
		SessionID:   "sess-1",
		NativeID:    "native-conv-1",
		WorkspaceID: "ws-1",
		CreatedAt:   createdAt,
		UpdatedAt:   lastSeen,
	}

	if err := st.UpsertWorkspace(workspace); err != nil {
		t.Fatalf("UpsertWorkspace: %v", err)
	}
	if err := st.UpsertConversation(conversation); err != nil {
		t.Fatalf("UpsertConversation: %v", err)
	}
	if err := st.UpsertSession(session); err != nil {
		t.Fatalf("UpsertSession: %v", err)
	}
	if err := st.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	reopened, err := NewSQLiteStore(path)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	got, ok := reopened.GetSession("sess-1")
	if !ok {
		t.Fatalf("expected session to exist")
	}
	if got.NativeConversationID != session.NativeConversationID {
		t.Fatalf("NativeConversationID = %q, want %q", got.NativeConversationID, session.NativeConversationID)
	}
	if got.WorkspaceID != session.WorkspaceID {
		t.Fatalf("WorkspaceID = %q, want %q", got.WorkspaceID, session.WorkspaceID)
	}
	if got.WorkspacePath != session.WorkspacePath {
		t.Fatalf("WorkspacePath = %q, want %q", got.WorkspacePath, session.WorkspacePath)
	}
	if got.DetailPath != session.DetailPath {
		t.Fatalf("DetailPath = %q, want %q", got.DetailPath, session.DetailPath)
	}
	if !got.CreatedAt.Equal(createdAt) {
		t.Fatalf("CreatedAt = %s, want %s", got.CreatedAt, createdAt)
	}
	if !got.LastSeen.Equal(lastSeen) {
		t.Fatalf("LastSeen = %s, want %s", got.LastSeen, lastSeen)
	}

	snapshot := reopened.Snapshot()
	if gotWorkspace, ok := snapshot.Workspaces[workspace.ID]; !ok {
		t.Fatalf("expected workspace %q to exist", workspace.ID)
	} else if gotWorkspace.Path != workspace.Path {
		t.Fatalf("workspace path = %q, want %q", gotWorkspace.Path, workspace.Path)
	}
	if gotConversation, ok := snapshot.Conversations[conversation.ID]; !ok {
		t.Fatalf("expected conversation %q to exist", conversation.ID)
	} else {
		if gotConversation.SessionID != conversation.SessionID {
			t.Fatalf("conversation session_id = %q, want %q", gotConversation.SessionID, conversation.SessionID)
		}
		if gotConversation.NativeID != conversation.NativeID {
			t.Fatalf("conversation native_id = %q, want %q", gotConversation.NativeID, conversation.NativeID)
		}
	}
}

func TestSQLiteStoreSkipsTouchForUnchangedPayload(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.sqlite")

	st, err := NewSQLiteStore(path)
	if err != nil {
		t.Fatalf("NewSQLiteStore: %v", err)
	}

	session := model.Session{
		ID:             "sess-unchanged",
		Adapter:        "codex",
		StartMode:      model.StartModeNew,
		ResumeStrategy: model.ResumeStrategyProcessOnly,
		State:          model.SessionStateStopped,
		CreatedAt:      time.Now().UTC().Round(0),
		LastSeen:       time.Now().UTC().Round(0),
	}

	if err := st.UpsertSession(session); err != nil {
		t.Fatalf("first UpsertSession: %v", err)
	}
	firstUpdatedAt := st.Snapshot().UpdatedAt
	if firstUpdatedAt.IsZero() {
		t.Fatal("expected non-zero updated_at after first upsert")
	}

	time.Sleep(10 * time.Millisecond)

	if err := st.UpsertSession(session); err != nil {
		t.Fatalf("second UpsertSession: %v", err)
	}
	secondUpdatedAt := st.Snapshot().UpdatedAt
	if !secondUpdatedAt.Equal(firstUpdatedAt) {
		t.Fatalf("updated_at changed for unchanged payload: first=%s second=%s", firstUpdatedAt, secondUpdatedAt)
	}
}

func TestSQLiteStoreDeleteRemovesEntries(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.sqlite")

	st, err := NewSQLiteStore(path)
	if err != nil {
		t.Fatalf("NewSQLiteStore: %v", err)
	}

	now := time.Now().UTC().Round(0)
	session := model.Session{
		ID:             "sess-delete",
		Adapter:        "claudecode",
		Origin:         model.SessionOriginDiscovered,
		ConversationID: "conv-delete",
		WorkspaceID:    "ws-delete",
		State:          model.SessionStateStopped,
		CreatedAt:      now,
		LastSeen:       now,
	}
	workspace := model.Workspace{
		ID:        "ws-delete",
		Path:      "/tmp/agen/delete-me",
		CreatedAt: now,
		UpdatedAt: now,
	}
	conversation := model.Conversation{
		ID:          "conv-delete",
		SessionID:   "sess-delete",
		WorkspaceID: "ws-delete",
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	if err := st.UpsertWorkspace(workspace); err != nil {
		t.Fatalf("UpsertWorkspace: %v", err)
	}
	if err := st.UpsertConversation(conversation); err != nil {
		t.Fatalf("UpsertConversation: %v", err)
	}
	if err := st.UpsertSession(session); err != nil {
		t.Fatalf("UpsertSession: %v", err)
	}

	if err := st.DeleteSession(session.ID); err != nil {
		t.Fatalf("DeleteSession: %v", err)
	}
	if err := st.DeleteConversation(conversation.ID); err != nil {
		t.Fatalf("DeleteConversation: %v", err)
	}
	if err := st.DeleteWorkspace(workspace.ID); err != nil {
		t.Fatalf("DeleteWorkspace: %v", err)
	}

	if _, ok := st.GetSession(session.ID); ok {
		t.Fatal("expected session to be deleted")
	}
	snapshot := st.Snapshot()
	if _, ok := snapshot.Conversations[conversation.ID]; ok {
		t.Fatal("expected conversation to be deleted")
	}
	if _, ok := snapshot.Workspaces[workspace.ID]; ok {
		t.Fatal("expected workspace to be deleted")
	}
}
