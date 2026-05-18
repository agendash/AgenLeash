package app

import (
	"context"
	"encoding/base64"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/agendash/AgenLeash/internal/adapter"
	"github.com/agendash/AgenLeash/internal/event"
	"github.com/agendash/AgenLeash/internal/history"
	"github.com/agendash/AgenLeash/internal/httpapi"
	"github.com/agendash/AgenLeash/internal/model"
	agenruntime "github.com/agendash/AgenLeash/internal/runtime"
	"github.com/agendash/AgenLeash/internal/session"
	"github.com/agendash/AgenLeash/internal/store"
)

func TestBuildRuntimeArgsForClaudeCode(t *testing.T) {
	spec := loadClaudeAdapterSpec(t)

	effective, err := adapter.Resolve(spec, "2.1.100")
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}

	args := buildRuntimeArgs(spec, effective, model.StartModeResume, "conv_123", []string{"--debug"})
	want := []string{
		"-p",
		"--verbose",
		"--output-format",
		"stream-json",
		"--dangerously-skip-permissions",
		"--resume",
		"conv_123",
		"--debug",
	}
	if len(args) != len(want) {
		t.Fatalf("args length = %d, want %d (%v)", len(args), len(want), args)
	}
	for i := range want {
		if args[i] != want[i] {
			t.Fatalf("args[%d] = %q, want %q (args=%v)", i, args[i], want[i], args)
		}
	}
}

func TestBuildRuntimeArgsForOpencode(t *testing.T) {
	spec := loadOpencodeAdapterSpec(t)

	effective, err := adapter.Resolve(spec, "1.14.20")
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}

	args := buildRuntimeArgs(spec, effective, model.StartModeResume, "ses_123", nil)
	want := []string{"run", "--session", "ses_123", "--format", "json"}
	if len(args) != len(want) {
		t.Fatalf("args length = %d, want %d (%v)", len(args), len(want), args)
	}
	for i := range want {
		if args[i] != want[i] {
			t.Fatalf("args[%d] = %q, want %q (args=%v)", i, args[i], want[i], args)
		}
	}
}

func TestBuildRuntimeArgsForCodexResume(t *testing.T) {
	spec := loadCodexAdapterSpec(t)

	effective, err := adapter.Resolve(spec, "0.121.0")
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}

	args := buildRuntimeArgs(spec, effective, model.StartModeResume, "thread_123", nil)
	want := []string{"exec", "resume", "--json", "--skip-git-repo-check", "--dangerously-bypass-approvals-and-sandbox", "thread_123"}
	if len(args) != len(want) {
		t.Fatalf("args length = %d, want %d (%v)", len(args), len(want), args)
	}
	for i := range want {
		if args[i] != want[i] {
			t.Fatalf("args[%d] = %q, want %q (args=%v)", i, args[i], want[i], args)
		}
	}
}

func TestNewRequiresTokenUnlessExplicitlyDisabled(t *testing.T) {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	root := filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))

	_, err := New(Config{
		DataDir:    t.TempDir(),
		AdapterDir: filepath.Join(root, "adapters"),
	})
	if err == nil {
		t.Fatal("New() error = nil, want token validation failure")
	}
	if !strings.Contains(err.Error(), "AGENLEASH_TOKEN") {
		t.Fatalf("New() error = %v, want token guidance", err)
	}
}

func TestHandleStartSessionRejectsWorkspaceOutsideAllowedRoots(t *testing.T) {
	spec := loadClaudeAdapterSpec(t)

	allowedRoot := t.TempDir()
	disallowedRoot := t.TempDir()
	if err := os.MkdirAll(filepath.Join(disallowedRoot, "repo"), 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}

	app := &App{
		Config: Config{
			AllowNoToken:          true,
			AllowedWorkspaceRoots: []string{allowedRoot},
		},
		adapters: map[string]adapter.AdapterSpec{spec.Metadata.Name: spec},
	}

	req := httptest.NewRequest("POST", "/api/v1/agent/start", nil)
	_, err := app.handleStartSession(req, httpapi.StartSessionRequest{
		Adapter:          spec.Metadata.Name,
		CWD:              filepath.Join(disallowedRoot, "repo"),
		AgentVersionHint: "2.1.100",
		StartMode:        string(model.StartModeNew),
	})
	if err == nil {
		t.Fatal("handleStartSession() error = nil, want workspace root rejection")
	}
	if !strings.Contains(err.Error(), "AGENLEASH_ALLOWED_WORKSPACE_ROOTS") {
		t.Fatalf("handleStartSession() error = %v, want workspace root guidance", err)
	}
}

func TestBuildRuntimeArgsStripsBaselineResumeFlags(t *testing.T) {
	spec := loadClaudeAdapterSpec(t)
	mode := "pty"
	entrypoint := "claude"
	spec.Spec.Runtime.Mode = &mode
	spec.Spec.Runtime.Entrypoint = &entrypoint
	spec.Spec.Runtime.Args = []string{"--resume", "old", "--verbose"}

	effective, err := adapter.Resolve(spec, "2.1.100")
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	effective.Runtime.Args = []string{"--resume", "old", "--verbose"}

	args := buildRuntimeArgs(spec, effective, model.StartModeNew, "", nil)
	want := []string{"--verbose"}
	if len(args) != len(want) || args[0] != want[0] {
		t.Fatalf("args = %v, want %v", args, want)
	}
}

func loadClaudeAdapterSpec(t *testing.T) adapter.AdapterSpec {
	t.Helper()

	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	root := filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
	spec, err := adapter.LoadFile(filepath.Join(root, "adapters", "claudecode.json"))
	if err != nil {
		t.Fatalf("LoadFile() error = %v", err)
	}
	return spec
}

func loadCodexAdapterSpec(t *testing.T) adapter.AdapterSpec {
	t.Helper()

	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	root := filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
	spec, err := adapter.LoadFile(filepath.Join(root, "adapters", "codex.json"))
	if err != nil {
		t.Fatalf("LoadFile() error = %v", err)
	}
	return spec
}

func loadOpencodeAdapterSpec(t *testing.T) adapter.AdapterSpec {
	t.Helper()

	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	root := filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
	spec, err := adapter.LoadFile(filepath.Join(root, "adapters", "opencode.json"))
	if err != nil {
		t.Fatalf("LoadFile() error = %v", err)
	}
	return spec
}

func TestHandleStartSessionDetachesRuntimeFromRequestContext(t *testing.T) {
	tempDir := t.TempDir()
	repo := store.NewMemoryStore()

	spec := loadClaudeAdapterSpec(t)
	mode := "pty"
	entrypoint := "claude"
	spec.Spec.Runtime.Mode = &mode
	spec.Spec.Runtime.Entrypoint = &entrypoint
	spec.Spec.Runtime.Args = nil

	rt := &capturingRuntime{events: closedRuntimeEvents()}
	app := &App{
		Store:    repo,
		Hub:      event.NewHub(16),
		Sessions: session.NewManagerWithFactory(16, func(agenruntime.Spec) (agenruntime.Runtime, error) { return rt, nil }),
		adapters: map[string]adapter.AdapterSpec{spec.Metadata.Name: spec},
	}

	reqCtx, cancel := context.WithCancel(context.Background())
	req := httptest.NewRequest("POST", "/api/v1/agent/start", nil).WithContext(reqCtx)
	snapshot, err := app.handleStartSession(req, httpapi.StartSessionRequest{
		Adapter:          spec.Metadata.Name,
		CWD:              tempDir,
		AgentVersionHint: "2.1.100",
		StartMode:        string(model.StartModeNew),
	})
	if err != nil {
		t.Fatalf("handleStartSession() error = %v", err)
	}

	cancel()
	select {
	case <-rt.startCtx.Done():
		t.Fatal("runtime context was canceled with the request context")
	default:
	}
	if snapshot.RuntimeMode != "stdio" {
		t.Fatalf("RuntimeMode = %q, want stdio", snapshot.RuntimeMode)
	}
}

func TestHandleStartSessionRejectsMissingEntrypoint(t *testing.T) {
	tempDir := t.TempDir()

	spec := loadClaudeAdapterSpec(t)
	entrypoint := "agenleash-definitely-missing-binary"
	spec.Spec.Runtime.Entrypoint = &entrypoint
	spec.Spec.Detection.BinaryNames = []string{entrypoint}
	spec.Spec.Detection.VersionStrategy.Command = []string{entrypoint, "--version"}
	for i := range spec.Spec.VersionProfiles {
		if spec.Spec.VersionProfiles[i].Overrides.Runtime == nil {
			continue
		}
		spec.Spec.VersionProfiles[i].Overrides.Runtime.Entrypoint = &entrypoint
	}

	app := &App{
		Config: Config{
			AllowNoToken: true,
		},
		adapters: map[string]adapter.AdapterSpec{spec.Metadata.Name: spec},
	}

	req := httptest.NewRequest("POST", "/api/v1/agent/start", nil)
	_, err := app.handleStartSession(req, httpapi.StartSessionRequest{
		Adapter:          spec.Metadata.Name,
		CWD:              tempDir,
		AgentVersionHint: "2.1.100",
		StartMode:        string(model.StartModeNew),
	})
	if err == nil {
		t.Fatal("handleStartSession() error = nil, want missing entrypoint error")
	}
	if !strings.Contains(err.Error(), "not found in PATH") {
		t.Fatalf("handleStartSession() error = %v, want missing PATH guidance", err)
	}
}

func TestHandleGetSessionPaginatesMessages(t *testing.T) {
	tempDir := t.TempDir()
	historyPath := filepath.Join(tempDir, "claude.jsonl")
	body := `{"type":"user","uuid":"user-1","timestamp":"2026-04-20T02:15:44.254Z","message":{"role":"user","content":"first"}}
{"type":"assistant","uuid":"assistant-1","timestamp":"2026-04-20T02:15:45.254Z","message":{"role":"assistant","content":[{"type":"text","text":"FIRST"}]}}
{"type":"user","uuid":"user-2","timestamp":"2026-04-20T02:15:46.254Z","message":{"role":"user","content":"second"}}
{"type":"assistant","uuid":"assistant-2","timestamp":"2026-04-20T02:15:47.254Z","message":{"role":"assistant","content":[{"type":"text","text":"SECOND"}]}}
{"type":"user","uuid":"user-3","timestamp":"2026-04-20T02:15:48.254Z","message":{"role":"user","content":"third"}}
{"type":"assistant","uuid":"assistant-3","timestamp":"2026-04-20T02:15:49.254Z","message":{"role":"assistant","content":[{"type":"text","text":"THIRD"}]}}
`
	if err := os.WriteFile(historyPath, []byte(body), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	repo := store.NewMemoryStore()
	session := model.Session{
		ID:             "sess_hist",
		Adapter:        "claudecode",
		State:          model.SessionStateStopped,
		ConversationID: "conv_hist",
		WorkspaceID:    "ws_hist",
		WorkspacePath:  tempDir,
		DetailPath:     historyPath,
		CreatedAt:      time.Now().UTC().Add(-time.Minute),
		LastSeen:       time.Now().UTC(),
	}
	if err := repo.UpsertSession(session); err != nil {
		t.Fatalf("UpsertSession() error = %v", err)
	}

	app := &App{
		Store: repo,
	}

	req := httptest.NewRequest("GET", "/api/v1/sessions/sess_hist?limit=2", nil)
	detail, err := app.handleGetSession(req, "sess_hist")
	if err != nil {
		t.Fatalf("handleGetSession() error = %v", err)
	}
	if len(detail.Messages) != 2 {
		t.Fatalf("messages = %d, want 2", len(detail.Messages))
	}
	if detail.Messages[0].Content != "third" || detail.Messages[1].Content != "THIRD" {
		t.Fatalf("messages = %#v", detail.Messages)
	}
	if detail.MessagesPage == nil || !detail.MessagesPage.HasMore || detail.MessagesPage.NextOffset != 2 {
		t.Fatalf("messages_page = %#v", detail.MessagesPage)
	}

	olderReq := httptest.NewRequest("GET", "/api/v1/sessions/sess_hist?limit=2&offset=2", nil)
	olderDetail, err := app.handleGetSession(olderReq, "sess_hist")
	if err != nil {
		t.Fatalf("older handleGetSession() error = %v", err)
	}
	if len(olderDetail.Messages) != 2 {
		t.Fatalf("older messages = %d, want 2", len(olderDetail.Messages))
	}
	if olderDetail.Messages[0].Content != "second" || olderDetail.Messages[1].Content != "SECOND" {
		t.Fatalf("older messages = %#v", olderDetail.Messages)
	}
}

func TestHandleGetFilePreviewReturnsWorkspaceText(t *testing.T) {
	workspacePath := t.TempDir()
	previewPath := filepath.Join(workspacePath, "docker-compose.dual-gpu.yml")
	if err := os.WriteFile(previewPath, []byte("services:\n  web:\n    image: demo\n"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	repo := store.NewMemoryStore()
	session := model.Session{
		ID:            "sess_preview",
		Adapter:       "codex",
		State:         model.SessionStateStopped,
		WorkspacePath: workspacePath,
		WorkspaceRoot: workspacePath,
		GitRoot:       workspacePath,
		CreatedAt:     time.Now().UTC().Add(-time.Minute),
		LastSeen:      time.Now().UTC(),
	}
	if err := repo.UpsertSession(session); err != nil {
		t.Fatalf("UpsertSession() error = %v", err)
	}

	app := &App{
		Config: Config{AllowedWorkspaceRoots: []string{workspacePath}},
		Store:  repo,
	}
	req := httptest.NewRequest("GET", "/api/v1/sessions/sess_preview/files/preview?path=docker-compose.dual-gpu.yml", nil)
	preview, err := app.handleGetFilePreview(req, "sess_preview", "docker-compose.dual-gpu.yml")
	if err != nil {
		t.Fatalf("handleGetFilePreview() error = %v", err)
	}
	if preview.Kind != "text" || preview.Language != "yaml" || preview.Encoding != "utf-8" {
		t.Fatalf("preview metadata = %#v", preview)
	}
	canonicalPreviewPath, err := filepath.EvalSymlinks(previewPath)
	if err != nil {
		t.Fatalf("EvalSymlinks() error = %v", err)
	}
	if preview.Path != canonicalPreviewPath {
		t.Fatalf("path = %q, want %q", preview.Path, canonicalPreviewPath)
	}
	if !strings.Contains(preview.Content, "services:") {
		t.Fatalf("content = %q", preview.Content)
	}
}

func TestHandleGetFilePreviewAcceptsLineSuffix(t *testing.T) {
	workspacePath := t.TempDir()
	previewPath := filepath.Join(workspacePath, "src", "media_controller.cpp")
	if err := os.MkdirAll(filepath.Dir(previewPath), 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(previewPath, []byte("int main() { return 0; }\n"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	repo := store.NewMemoryStore()
	session := model.Session{
		ID:            "sess_preview",
		Adapter:       "codex",
		State:         model.SessionStateStopped,
		WorkspacePath: workspacePath,
		WorkspaceRoot: workspacePath,
		GitRoot:       workspacePath,
		CreatedAt:     time.Now().UTC().Add(-time.Minute),
		LastSeen:      time.Now().UTC(),
	}
	if err := repo.UpsertSession(session); err != nil {
		t.Fatalf("UpsertSession() error = %v", err)
	}

	app := &App{
		Config: Config{AllowedWorkspaceRoots: []string{workspacePath}},
		Store:  repo,
	}
	req := httptest.NewRequest("GET", "/api/v1/sessions/sess_preview/files/preview?path="+previewPath+":1992", nil)
	preview, err := app.handleGetFilePreview(req, "sess_preview", previewPath+":1992")
	if err != nil {
		t.Fatalf("handleGetFilePreview() error = %v", err)
	}
	if preview.Path == previewPath+":1992" {
		t.Fatalf("path kept line suffix: %q", preview.Path)
	}
	if !strings.Contains(preview.Content, "return 0") {
		t.Fatalf("content = %q", preview.Content)
	}
}

func TestHandleGetFilePreviewRejectsOutsideWorkspace(t *testing.T) {
	workspacePath := t.TempDir()
	outsidePath := filepath.Join(t.TempDir(), "secret.txt")
	if err := os.WriteFile(outsidePath, []byte("secret"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	repo := store.NewMemoryStore()
	session := model.Session{
		ID:            "sess_preview",
		Adapter:       "codex",
		State:         model.SessionStateStopped,
		WorkspacePath: workspacePath,
		WorkspaceRoot: workspacePath,
		GitRoot:       workspacePath,
		CreatedAt:     time.Now().UTC().Add(-time.Minute),
		LastSeen:      time.Now().UTC(),
	}
	if err := repo.UpsertSession(session); err != nil {
		t.Fatalf("UpsertSession() error = %v", err)
	}

	app := &App{
		Config: Config{AllowedWorkspaceRoots: []string{workspacePath}},
		Store:  repo,
	}
	req := httptest.NewRequest("GET", "/api/v1/sessions/sess_preview/files/preview?path="+outsidePath, nil)
	_, err := app.handleGetFilePreview(req, "sess_preview", outsidePath)
	if err == nil {
		t.Fatal("handleGetFilePreview() error = nil, want forbidden")
	}
	var statusErr *httpapi.StatusError
	if !errors.As(err, &statusErr) || statusErr.Code != http.StatusForbidden {
		t.Fatalf("error = %v, want forbidden status", err)
	}
}

func TestHandleGetFilePreviewAllowsTemporaryImage(t *testing.T) {
	workspacePath := t.TempDir()
	outsideDir := t.TempDir()
	previewPath := filepath.Join(outsideDir, "selected-title.png")
	imageBytes, err := base64.StdEncoding.DecodeString("iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+/p9sAAAAASUVORK5CYII=")
	if err != nil {
		t.Fatalf("DecodeString() error = %v", err)
	}
	if err := os.WriteFile(previewPath, imageBytes, 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	repo := store.NewMemoryStore()
	session := model.Session{
		ID:            "sess_preview",
		Adapter:       "codex",
		State:         model.SessionStateStopped,
		WorkspacePath: workspacePath,
		WorkspaceRoot: workspacePath,
		GitRoot:       workspacePath,
		CreatedAt:     time.Now().UTC().Add(-time.Minute),
		LastSeen:      time.Now().UTC(),
	}
	if err := repo.UpsertSession(session); err != nil {
		t.Fatalf("UpsertSession() error = %v", err)
	}

	app := &App{
		Config: Config{AllowedWorkspaceRoots: []string{workspacePath}},
		Store:  repo,
	}
	req := httptest.NewRequest("GET", "/api/v1/sessions/sess_preview/files/preview?path="+previewPath, nil)
	preview, err := app.handleGetFilePreview(req, "sess_preview", previewPath)
	if err != nil {
		t.Fatalf("handleGetFilePreview() error = %v", err)
	}
	if preview.Kind != "image" || preview.Encoding != "base64" || preview.Content == "" {
		t.Fatalf("preview = %#v", preview)
	}
}

func TestHandleGetFilePreviewRejectsTemporaryFakeImage(t *testing.T) {
	workspacePath := t.TempDir()
	outsideDir := t.TempDir()
	previewPath := filepath.Join(outsideDir, "not-really-an-image.png")
	if err := os.WriteFile(previewPath, []byte("secret text"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	repo := store.NewMemoryStore()
	session := model.Session{
		ID:            "sess_preview",
		Adapter:       "codex",
		State:         model.SessionStateStopped,
		WorkspacePath: workspacePath,
		WorkspaceRoot: workspacePath,
		GitRoot:       workspacePath,
		CreatedAt:     time.Now().UTC().Add(-time.Minute),
		LastSeen:      time.Now().UTC(),
	}
	if err := repo.UpsertSession(session); err != nil {
		t.Fatalf("UpsertSession() error = %v", err)
	}

	app := &App{
		Config: Config{AllowedWorkspaceRoots: []string{workspacePath}},
		Store:  repo,
	}
	req := httptest.NewRequest("GET", "/api/v1/sessions/sess_preview/files/preview?path="+previewPath, nil)
	_, err := app.handleGetFilePreview(req, "sess_preview", previewPath)
	if err == nil {
		t.Fatal("handleGetFilePreview() error = nil, want forbidden")
	}
	var statusErr *httpapi.StatusError
	if !errors.As(err, &statusErr) || statusErr.Code != http.StatusForbidden {
		t.Fatalf("error = %v, want forbidden status", err)
	}
}

func TestConsumeCodexJSONOutputPublishesAssistantMessageAndBindsThread(t *testing.T) {
	rt := &capturingRuntime{events: closedRuntimeEvents()}
	sess, err := session.New(
		context.Background(),
		session.StartRequest{
			ID:             "sess_codex_live",
			Adapter:        "codex",
			RuntimeMode:    "stdio",
			ConversationID: "conv_codex_live",
			WorkspaceID:    "ws_codex_live",
			WorkspacePath:  t.TempDir(),
		},
		rt,
		16,
	)
	if err != nil {
		t.Fatalf("session.New() error = %v", err)
	}

	hub := event.NewHub(32)
	app := &App{
		Store: store.NewMemoryStore(),
		Hub:   hub,
	}

	sub := hub.Subscribe("sess_codex_live")
	defer sub.Close()

	remainder, consumed := app.consumeCodexJSONOutput(
		sess,
		"sess_codex_live",
		[]byte("{\"type\":\"thread.started\",\"thread_id\":\"thread-123\"}\n{\"type\":\"item.completed\",\"item\":{\"id\":\"item_1\",\"type\":\"agent_message\",\"text\":\"OK\"}}\n"),
		"",
	)
	if remainder != "" {
		t.Fatalf("remainder = %q, want empty", remainder)
	}
	if !consumed {
		t.Fatal("consumed = false, want true")
	}

	snapshot := sess.Snapshot()
	if snapshot.NativeConversationID != "thread-123" {
		t.Fatalf("native conversation id = %q, want thread-123", snapshot.NativeConversationID)
	}
	if snapshot.LastOutputPreview != "OK" {
		t.Fatalf("last output preview = %q, want OK", snapshot.LastOutputPreview)
	}

	foundDelta := false
	for len(sub.Recent) > 0 {
		evt := sub.Recent[0]
		sub.Recent = sub.Recent[1:]
		if evt.MsgType == event.MsgTypeMessageDelta && evt.Delta == "OK" {
			foundDelta = true
		}
	}
	for len(sub.C) > 0 {
		evt := <-sub.C
		if evt.MsgType == event.MsgTypeMessageDelta && evt.Delta == "OK" {
			foundDelta = true
		}
	}
	if !foundDelta {
		t.Fatal("did not receive assistant message delta for codex output")
	}
}

func TestPumpSessionSuppressesCodexUnstructuredRuntimeOutput(t *testing.T) {
	rtEvents := make(chan agenruntime.Event, 2)
	rt := &capturingRuntime{events: rtEvents}
	sess, err := session.New(
		context.Background(),
		session.StartRequest{
			ID:             "sess_codex_noise",
			Adapter:        "codex",
			RuntimeMode:    "stdio",
			ConversationID: "conv_codex_noise",
			WorkspaceID:    "ws_codex_noise",
			WorkspacePath:  t.TempDir(),
		},
		rt,
		16,
	)
	if err != nil {
		t.Fatalf("session.New() error = %v", err)
	}

	hub := event.NewHub(32)
	app := &App{
		Store: store.NewMemoryStore(),
		Hub:   hub,
	}
	sub := hub.Subscribe("sess_codex_noise")
	defer sub.Close()

	rtEvents <- agenruntime.Event{
		Kind: agenruntime.EventKindOutput,
		Data: []byte("Reading prompt from stdin...\n2026-05-18T07:31:08.226449Z ERROR codex_core_skills::manager: failed to install system skills\n"),
	}
	close(rtEvents)

	app.pumpSession(sess, codexJSONEffectiveSpec())

	if got := sess.Snapshot().LastOutputPreview; got != "" {
		t.Fatalf("last output preview = %q, want empty", got)
	}

	for len(sub.Recent) > 0 {
		evt := sub.Recent[0]
		sub.Recent = sub.Recent[1:]
		if evt.MsgType == event.MsgTypeMessageDelta {
			t.Fatalf("unexpected message delta for runtime noise: %q", evt.Delta)
		}
	}
	for len(sub.C) > 0 {
		evt := <-sub.C
		if evt.MsgType == event.MsgTypeMessageDelta {
			t.Fatalf("unexpected message delta for runtime noise: %q", evt.Delta)
		}
	}
}

func TestConsumeClaudeCodeJSONOutputPublishesResultAndBindsSession(t *testing.T) {
	rt := &capturingRuntime{events: closedRuntimeEvents()}
	sess, err := session.New(
		context.Background(),
		session.StartRequest{
			ID:             "sess_claude_live",
			Adapter:        "claudecode",
			RuntimeMode:    "stdio",
			ConversationID: "conv_claude_live",
			WorkspaceID:    "ws_claude_live",
			WorkspacePath:  t.TempDir(),
		},
		rt,
		16,
	)
	if err != nil {
		t.Fatalf("session.New() error = %v", err)
	}

	hub := event.NewHub(32)
	app := &App{
		Store: store.NewMemoryStore(),
		Hub:   hub,
	}

	sub := hub.Subscribe("sess_claude_live")
	defer sub.Close()

	remainder, consumed := app.consumeClaudeCodeJSONOutput(
		sess,
		"sess_claude_live",
		[]byte("{\"type\":\"system\",\"subtype\":\"init\",\"session_id\":\"claude-native-123\"}\n{\"type\":\"system\",\"subtype\":\"status\",\"message\":\"requesting\"}\n{\"type\":\"result\",\"subtype\":\"success\",\"session_id\":\"claude-native-123\",\"result\":\"AGENDASH_SMOKE_OK\"}\n"),
		"",
	)
	if remainder != "" {
		t.Fatalf("remainder = %q, want empty", remainder)
	}
	if !consumed {
		t.Fatal("consumed = false, want true")
	}

	snapshot := sess.Snapshot()
	if snapshot.NativeConversationID != "claude-native-123" {
		t.Fatalf("native conversation id = %q, want claude-native-123", snapshot.NativeConversationID)
	}
	if snapshot.LastOutputPreview != "AGENDASH_SMOKE_OK" {
		t.Fatalf("last output preview = %q, want AGENDASH_SMOKE_OK", snapshot.LastOutputPreview)
	}

	foundDelta := false
	for len(sub.Recent) > 0 {
		evt := sub.Recent[0]
		sub.Recent = sub.Recent[1:]
		if evt.MsgType == event.MsgTypeMessageDelta && evt.Delta == "AGENDASH_SMOKE_OK" {
			foundDelta = true
		}
		if evt.MsgType == event.MsgTypeMessageDelta && strings.Contains(evt.Delta, "requesting") {
			t.Fatalf("unexpected status delta: %q", evt.Delta)
		}
	}
	for len(sub.C) > 0 {
		evt := <-sub.C
		if evt.MsgType == event.MsgTypeMessageDelta && evt.Delta == "AGENDASH_SMOKE_OK" {
			foundDelta = true
		}
		if evt.MsgType == event.MsgTypeMessageDelta && strings.Contains(evt.Delta, "requesting") {
			t.Fatalf("unexpected status delta: %q", evt.Delta)
		}
	}
	if !foundDelta {
		t.Fatal("did not receive assistant message delta for claude result")
	}
}

func TestPumpSessionSuppressesClaudeCodeUnstructuredRuntimeOutput(t *testing.T) {
	rtEvents := make(chan agenruntime.Event, 2)
	rt := &capturingRuntime{events: rtEvents}
	sess, err := session.New(
		context.Background(),
		session.StartRequest{
			ID:             "sess_claude_noise",
			Adapter:        "claudecode",
			RuntimeMode:    "stdio",
			ConversationID: "conv_claude_noise",
			WorkspaceID:    "ws_claude_noise",
			WorkspacePath:  t.TempDir(),
		},
		rt,
		16,
	)
	if err != nil {
		t.Fatalf("session.New() error = %v", err)
	}

	hub := event.NewHub(32)
	app := &App{
		Store: store.NewMemoryStore(),
		Hub:   hub,
	}
	sub := hub.Subscribe("sess_claude_noise")
	defer sub.Close()

	rtEvents <- agenruntime.Event{
		Kind: agenruntime.EventKindOutput,
		Data: []byte("Claude Code v2.1.119\n> Reply with exactly AGENDASH_SMOKE_OK.\n* Manifesting...\n"),
	}
	close(rtEvents)

	app.pumpSession(sess, claudeCodeJSONEffectiveSpec())

	if got := sess.Snapshot().LastOutputPreview; got != "" {
		t.Fatalf("last output preview = %q, want empty", got)
	}

	for len(sub.Recent) > 0 {
		evt := sub.Recent[0]
		sub.Recent = sub.Recent[1:]
		if evt.MsgType == event.MsgTypeMessageDelta {
			t.Fatalf("unexpected message delta for claude TUI noise: %q", evt.Delta)
		}
	}
	for len(sub.C) > 0 {
		evt := <-sub.C
		if evt.MsgType == event.MsgTypeMessageDelta {
			t.Fatalf("unexpected message delta for claude TUI noise: %q", evt.Delta)
		}
	}
}

func TestSanitizeOutputTextDropsClaudeTTYNoise(t *testing.T) {
	raw := []byte("\x1b]0;⠐ Claude Code\x07\r\x1b[2K✶\r\n]0;⠐ Claude Code\nReal answer\n")
	if got := sanitizeOutputText(raw); got != "Real answer" {
		t.Fatalf("sanitizeOutputText() = %q, want Real answer", got)
	}
}

func TestSanitizeOutputTextKeepsClaudeCodeInRealText(t *testing.T) {
	raw := []byte("Use Claude Code for this task.\n")
	if got := sanitizeOutputText(raw); got != "Use Claude Code for this task." {
		t.Fatalf("sanitizeOutputText() = %q", got)
	}
}

func TestRefreshHistoryPrunesDeletedDiscoveredSessions(t *testing.T) {
	claudeHome := filepath.Join(t.TempDir(), ".claude")
	workspaceOne := filepath.Join(t.TempDir(), "workspace-one")
	workspaceTwo := filepath.Join(t.TempDir(), "workspace-two")
	if err := os.MkdirAll(workspaceOne, 0o755); err != nil {
		t.Fatalf("MkdirAll(workspaceOne) error = %v", err)
	}
	if err := os.MkdirAll(workspaceTwo, 0o755); err != nil {
		t.Fatalf("MkdirAll(workspaceTwo) error = %v", err)
	}

	firstHistoryPath := filepath.Join(claudeHome, "projects", "workspace-one", "first.jsonl")
	secondHistoryPath := filepath.Join(claudeHome, "projects", "workspace-two", "second.jsonl")
	writeClaudeDiscoveryFile(t, firstHistoryPath, workspaceOne, "FIRST")
	writeClaudeDiscoveryFile(t, secondHistoryPath, workspaceTwo, "SECOND")

	repo := store.NewMemoryStore()
	now := time.Now().UTC()
	if err := repo.UpsertWorkspace(model.Workspace{
		ID:        "ws_managed",
		Path:      filepath.Join(t.TempDir(), "managed-workspace"),
		CreatedAt: now,
		UpdatedAt: now,
	}); err != nil {
		t.Fatalf("UpsertWorkspace(managed) error = %v", err)
	}
	if err := repo.UpsertConversation(model.Conversation{
		ID:          "conv_managed",
		SessionID:   "sess_managed",
		WorkspaceID: "ws_managed",
		CreatedAt:   now,
		UpdatedAt:   now,
	}); err != nil {
		t.Fatalf("UpsertConversation(managed) error = %v", err)
	}
	if err := repo.UpsertSession(model.Session{
		ID:             "sess_managed",
		Adapter:        "claudecode",
		Origin:         model.SessionOriginManaged,
		State:          model.SessionStateStopped,
		ConversationID: "conv_managed",
		WorkspaceID:    "ws_managed",
		CreatedAt:      now,
		LastSeen:       now,
	}); err != nil {
		t.Fatalf("UpsertSession(managed) error = %v", err)
	}

	app := &App{
		Config: Config{
			DiscoverClaude: true,
			ClaudeHome:     claudeHome,
		},
		Store:    repo,
		adapters: map[string]adapter.AdapterSpec{"claudecode": {}},
	}

	if err := app.refreshHistory(context.Background(), true); err != nil {
		t.Fatalf("first refreshHistory() error = %v", err)
	}

	before := repo.Snapshot()
	if len(before.Sessions) != 3 {
		t.Fatalf("sessions before delete = %d, want 3", len(before.Sessions))
	}

	if err := os.Remove(secondHistoryPath); err != nil {
		t.Fatalf("Remove(secondHistoryPath) error = %v", err)
	}

	if err := app.refreshHistory(context.Background(), true); err != nil {
		t.Fatalf("second refreshHistory() error = %v", err)
	}

	after := repo.Snapshot()
	if len(after.Sessions) != 2 {
		t.Fatalf("sessions after delete = %d, want 2", len(after.Sessions))
	}
	if _, ok := after.Sessions["sess_managed"]; !ok {
		t.Fatal("expected managed session to be preserved")
	}

	var (
		foundFirst   bool
		foundSecond  bool
		foundManaged bool
	)
	for _, session := range after.Sessions {
		switch session.NativeConversationID {
		case "first":
			foundFirst = true
		case "second":
			foundSecond = true
		}
		if session.ID == "sess_managed" {
			foundManaged = true
		}
	}
	if !foundFirst {
		t.Fatal("expected first discovered session to remain")
	}
	if foundSecond {
		t.Fatal("expected deleted discovered session to be pruned")
	}
	if !foundManaged {
		t.Fatal("expected managed session to remain")
	}

	var secondWorkspaceExists bool
	for _, workspace := range after.Workspaces {
		if filepath.Clean(workspace.Path) == filepath.Clean(workspaceTwo) {
			secondWorkspaceExists = true
			break
		}
	}
	if secondWorkspaceExists {
		t.Fatal("expected orphaned workspace for deleted session to be pruned")
	}

	for _, conversation := range after.Conversations {
		if conversation.NativeID == "second" {
			t.Fatal("expected orphaned conversation for deleted session to be pruned")
		}
	}
}

func TestConsumeOpencodeJSONOutput(t *testing.T) {
	rt := &capturingRuntime{events: closedRuntimeEvents()}
	sess, err := session.New(
		context.Background(),
		session.StartRequest{
			ID:             "sess_opencode_live",
			Adapter:        "opencode",
			RuntimeMode:    "stdio",
			ConversationID: "conv_opencode_live",
			WorkspaceID:    "ws_opencode_live",
			WorkspacePath:  t.TempDir(),
		},
		rt,
		16,
	)
	if err != nil {
		t.Fatalf("session.New() error = %v", err)
	}

	hub := event.NewHub(32)
	app := &App{
		Store: store.NewMemoryStore(),
		Hub:   hub,
	}

	sub := hub.Subscribe("sess_opencode_live")
	defer sub.Close()

	remainder, consumed := app.consumeOpencodeJSONOutput(
		sess,
		"sess_opencode_live",
		[]byte("{\"type\":\"step_start\",\"timestamp\":1776841195468,\"sessionID\":\"ses_native_123\",\"part\":{\"id\":\"prt_1\",\"messageID\":\"msg_1\",\"sessionID\":\"ses_native_123\",\"type\":\"step-start\"}}\n{\"type\":\"text\",\"timestamp\":1776841195497,\"sessionID\":\"ses_native_123\",\"part\":{\"id\":\"prt_2\",\"messageID\":\"msg_1\",\"sessionID\":\"ses_native_123\",\"type\":\"text\",\"text\":\"OK\"}}\n"),
		"",
	)
	if remainder != "" {
		t.Fatalf("remainder = %q, want empty", remainder)
	}
	if !consumed {
		t.Fatal("consumed = false, want true")
	}

	snapshot := sess.Snapshot()
	if snapshot.NativeConversationID != "ses_native_123" {
		t.Fatalf("native conversation id = %q, want ses_native_123", snapshot.NativeConversationID)
	}
	if snapshot.LastOutputPreview != "OK" {
		t.Fatalf("last output preview = %q, want OK", snapshot.LastOutputPreview)
	}

	foundDelta := false
	for len(sub.Recent) > 0 {
		evt := sub.Recent[0]
		sub.Recent = sub.Recent[1:]
		if evt.MsgType == event.MsgTypeMessageDelta && evt.Delta == "OK" {
			foundDelta = true
		}
	}
	for len(sub.C) > 0 {
		evt := <-sub.C
		if evt.MsgType == event.MsgTypeMessageDelta && evt.Delta == "OK" {
			foundDelta = true
		}
	}
	if !foundDelta {
		t.Fatal("did not receive assistant message delta for opencode output")
	}
}

func TestReconcileHistoryRecordsBackfillsManagedCodexNativeIDs(t *testing.T) {
	repo := store.NewMemoryStore()
	base := time.Date(2026, 4, 21, 11, 6, 0, 0, time.UTC)
	workspacePath := filepath.Join(t.TempDir(), "repo")
	now := base.Add(-time.Hour)

	if err := repo.UpsertWorkspace(model.Workspace{
		ID:        "ws_managed",
		Path:      workspacePath,
		CreatedAt: now,
		UpdatedAt: now,
	}); err != nil {
		t.Fatalf("UpsertWorkspace(managed) error = %v", err)
	}
	if err := repo.UpsertConversation(model.Conversation{
		ID:          "conv_managed",
		SessionID:   "sess_managed_old",
		WorkspaceID: "ws_managed",
		CreatedAt:   base,
		UpdatedAt:   base,
	}); err != nil {
		t.Fatalf("UpsertConversation(managed) error = %v", err)
	}
	if err := repo.UpsertSession(model.Session{
		ID:                "sess_managed_old",
		Adapter:           "codex",
		Origin:            model.SessionOriginManaged,
		RuntimeMode:       "stdio",
		StartMode:         model.StartModeNew,
		ResumeStrategy:    model.ResumeStrategyProcessOnly,
		WorkspacePath:     workspacePath,
		WorkspaceRoot:     workspacePath,
		WorkspaceID:       "ws_managed",
		ConversationID:    "conv_managed",
		State:             model.SessionStateErrored,
		LastOutputPreview: "Reading prompt from stdin...",
		CreatedAt:         base,
		LastSeen:          base.Add(20 * time.Second),
		Capabilities: model.Capabilities{
			SupportsResume:             true,
			SupportsNativeConversation: true,
		},
	}); err != nil {
		t.Fatalf("UpsertSession(managed old) error = %v", err)
	}
	if err := repo.UpsertSession(model.Session{
		ID:                   "sess_managed_bound",
		Adapter:              "codex",
		Origin:               model.SessionOriginManaged,
		NativeConversationID: "thread-used",
		WorkspacePath:        workspacePath,
		WorkspaceRoot:        workspacePath,
		State:                model.SessionStateStopped,
		CreatedAt:            base.Add(-5 * time.Minute),
		LastSeen:             base.Add(-4 * time.Minute),
	}); err != nil {
		t.Fatalf("UpsertSession(managed bound) error = %v", err)
	}

	app := &App{
		Store:    repo,
		Sessions: session.NewManager(),
		adapters: map[string]adapter.AdapterSpec{"codex": {}},
	}

	records := []history.Record{
		{
			Session: model.Session{
				ID:                   "sess_discovered_match",
				Adapter:              "codex",
				Origin:               model.SessionOriginDiscovered,
				NativeConversationID: "thread-123",
				WorkspacePath:        workspacePath,
				WorkspaceRoot:        workspacePath,
				State:                model.SessionStateStopped,
				DetailPath:           "/tmp/state.sqlite#threads/thread-123",
				LastOutputPreview:    "CODEX-OK",
				CreatedAt:            base.Add(6 * time.Minute),
				LastSeen:             base.Add(8 * time.Minute),
				Capabilities: model.Capabilities{
					SupportsResume:             true,
					SupportsNativeConversation: true,
				},
			},
			Conversation: model.Conversation{
				ID:        "conv_discovered_match",
				NativeID:  "thread-123",
				CreatedAt: base.Add(6 * time.Minute),
				UpdatedAt: base.Add(8 * time.Minute),
			},
			Workspace: model.Workspace{
				ID:        "ws_discovered_match",
				Path:      workspacePath,
				CreatedAt: base.Add(6 * time.Minute),
				UpdatedAt: base.Add(8 * time.Minute),
			},
		},
		{
			Session: model.Session{
				ID:                   "sess_discovered_used",
				Adapter:              "codex",
				Origin:               model.SessionOriginDiscovered,
				NativeConversationID: "thread-used",
				WorkspacePath:        workspacePath,
				WorkspaceRoot:        workspacePath,
				State:                model.SessionStateStopped,
				DetailPath:           "/tmp/state.sqlite#threads/thread-used",
				LastOutputPreview:    "USED",
				CreatedAt:            base.Add(4 * time.Minute),
				LastSeen:             base.Add(5 * time.Minute),
				Capabilities: model.Capabilities{
					SupportsResume:             true,
					SupportsNativeConversation: true,
				},
			},
			Conversation: model.Conversation{
				ID:        "conv_discovered_used",
				NativeID:  "thread-used",
				CreatedAt: base.Add(4 * time.Minute),
				UpdatedAt: base.Add(5 * time.Minute),
			},
			Workspace: model.Workspace{
				ID:        "ws_discovered_used",
				Path:      workspacePath,
				CreatedAt: base.Add(4 * time.Minute),
				UpdatedAt: base.Add(5 * time.Minute),
			},
		},
		{
			Session: model.Session{
				ID:                   "sess_discovered_too_old",
				Adapter:              "codex",
				Origin:               model.SessionOriginDiscovered,
				NativeConversationID: "thread-too-old",
				WorkspacePath:        workspacePath,
				WorkspaceRoot:        workspacePath,
				State:                model.SessionStateStopped,
				DetailPath:           "/tmp/state.sqlite#threads/thread-too-old",
				LastOutputPreview:    "TOO-OLD",
				CreatedAt:            base.Add(-2 * time.Minute),
				LastSeen:             base.Add(-time.Minute),
				Capabilities: model.Capabilities{
					SupportsResume:             true,
					SupportsNativeConversation: true,
				},
			},
			Conversation: model.Conversation{
				ID:        "conv_discovered_too_old",
				NativeID:  "thread-too-old",
				CreatedAt: base.Add(-2 * time.Minute),
				UpdatedAt: base.Add(-time.Minute),
			},
			Workspace: model.Workspace{
				ID:        "ws_discovered_too_old",
				Path:      workspacePath,
				CreatedAt: base.Add(-2 * time.Minute),
				UpdatedAt: base.Add(-time.Minute),
			},
		},
	}

	if err := app.reconcileHistoryRecords("codex", records); err != nil {
		t.Fatalf("reconcileHistoryRecords() error = %v", err)
	}

	updated, ok := repo.GetSession("sess_managed_old")
	if !ok {
		t.Fatal("managed session missing after reconcile")
	}
	if updated.NativeConversationID != "thread-123" {
		t.Fatalf("NativeConversationID = %q, want thread-123", updated.NativeConversationID)
	}
	if updated.ResumeStrategy != model.ResumeStrategyNativeID {
		t.Fatalf("ResumeStrategy = %q, want native_id", updated.ResumeStrategy)
	}
	if updated.DetailPath != "/tmp/state.sqlite#threads/thread-123" {
		t.Fatalf("DetailPath = %q, want discovered detail path", updated.DetailPath)
	}
	if updated.LastOutputPreview != "CODEX-OK" {
		t.Fatalf("LastOutputPreview = %q, want CODEX-OK", updated.LastOutputPreview)
	}
	if updated.State != model.SessionStateStopped {
		t.Fatalf("State = %q, want stopped", updated.State)
	}

	snapshot := repo.Snapshot()
	conversation := snapshot.Conversations["conv_managed"]
	if conversation.NativeID != "thread-123" {
		t.Fatalf("conversation.NativeID = %q, want thread-123", conversation.NativeID)
	}
}

func TestReconcileHistoryRecordsBackfillsManagedDetailForKnownNativeID(t *testing.T) {
	repo := store.NewMemoryStore()
	workspacePath := filepath.Join(t.TempDir(), "workspace")
	base := time.Date(2026, time.April, 22, 7, 11, 35, 0, time.UTC)

	if err := repo.UpsertWorkspace(model.Workspace{
		ID:        "ws_managed",
		Path:      workspacePath,
		CreatedAt: base,
		UpdatedAt: base,
	}); err != nil {
		t.Fatalf("UpsertWorkspace(managed) error = %v", err)
	}
	if err := repo.UpsertConversation(model.Conversation{
		ID:          "conv_managed",
		SessionID:   "sess_managed_native",
		WorkspaceID: "ws_managed",
		NativeID:    "ses_known",
		CreatedAt:   base,
		UpdatedAt:   base,
	}); err != nil {
		t.Fatalf("UpsertConversation(managed) error = %v", err)
	}
	if err := repo.UpsertSession(model.Session{
		ID:                   "sess_managed_native",
		Adapter:              "opencode",
		Origin:               model.SessionOriginManaged,
		RuntimeMode:          "stdio",
		StartMode:            model.StartModeNew,
		ResumeStrategy:       model.ResumeStrategyNativeID,
		NativeConversationID: "ses_known",
		WorkspacePath:        workspacePath,
		WorkspaceRoot:        workspacePath,
		WorkspaceID:          "ws_managed",
		ConversationID:       "conv_managed",
		State:                model.SessionStateErrored,
		CreatedAt:            base,
		LastSeen:             base.Add(2 * time.Second),
		Capabilities: model.Capabilities{
			SupportsResume:             true,
			SupportsNativeConversation: true,
		},
	}); err != nil {
		t.Fatalf("UpsertSession(managed) error = %v", err)
	}

	app := &App{
		Store:    repo,
		Sessions: session.NewManager(),
		adapters: map[string]adapter.AdapterSpec{"opencode": {}},
	}

	records := []history.Record{
		{
			Session: model.Session{
				ID:                   "sess_discovered_native",
				Adapter:              "opencode",
				Origin:               model.SessionOriginDiscovered,
				NativeConversationID: "ses_known",
				WorkspacePath:        workspacePath,
				WorkspaceRoot:        workspacePath,
				State:                model.SessionStateStopped,
				DetailPath:           "/tmp/opencode.db#sessions/ses_known",
				LastOutputPreview:    "LEASH_OPENCODE_OK",
				CreatedAt:            base.Add(4 * time.Second),
				LastSeen:             base.Add(6 * time.Second),
				Capabilities: model.Capabilities{
					SupportsResume:             true,
					SupportsNativeConversation: true,
				},
			},
			Conversation: model.Conversation{
				ID:        "conv_discovered_native",
				NativeID:  "ses_known",
				CreatedAt: base.Add(4 * time.Second),
				UpdatedAt: base.Add(6 * time.Second),
			},
			Workspace: model.Workspace{
				ID:        "ws_discovered_native",
				Path:      workspacePath,
				CreatedAt: base.Add(4 * time.Second),
				UpdatedAt: base.Add(6 * time.Second),
			},
		},
	}

	if err := app.reconcileHistoryRecords("opencode", records); err != nil {
		t.Fatalf("reconcileHistoryRecords() error = %v", err)
	}

	updated, ok := repo.GetSession("sess_managed_native")
	if !ok {
		t.Fatal("managed session missing after reconcile")
	}
	if updated.DetailPath != "/tmp/opencode.db#sessions/ses_known" {
		t.Fatalf("DetailPath = %q, want discovered detail path", updated.DetailPath)
	}
	if updated.LastOutputPreview != "LEASH_OPENCODE_OK" {
		t.Fatalf("LastOutputPreview = %q, want LEASH_OPENCODE_OK", updated.LastOutputPreview)
	}
	if updated.State != model.SessionStateStopped {
		t.Fatalf("State = %q, want stopped", updated.State)
	}
}

func TestSessionSubmitByteUsesCarriageReturnForTTY(t *testing.T) {
	manager := session.NewManagerWithFactory(16, func(agenruntime.Spec) (agenruntime.Runtime, error) {
		return &capturingRuntime{events: closedRuntimeEvents()}, nil
	})
	_, err := manager.Start(context.Background(), session.StartRequest{
		ID:           "sess_tty",
		Adapter:      "claudecode",
		Capabilities: model.Capabilities{RequiresTTY: true},
		Runtime: agenruntime.Spec{
			Command: "claude",
		},
	})
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	if got := sessionSubmitByte(manager, "sess_tty"); got != '\r' {
		t.Fatalf("sessionSubmitByte() = %q, want carriage return", got)
	}
}

func TestSessionSubmitByteFallsBackToNewline(t *testing.T) {
	if got := sessionSubmitByte(nil, "missing"); got != '\n' {
		t.Fatalf("sessionSubmitByte() = %q, want newline", got)
	}
}

func codexJSONEffectiveSpec() adapter.EffectiveSpec {
	parserType := "jsonl_events"
	parserProfile := "codex_exec"
	return adapter.EffectiveSpec{
		AdapterName: "codex",
		EventParser: adapter.EventParserSpec{
			Type:    &parserType,
			Profile: &parserProfile,
		},
	}
}

func claudeCodeJSONEffectiveSpec() adapter.EffectiveSpec {
	parserType := "json_events"
	parserProfile := "v2"
	return adapter.EffectiveSpec{
		AdapterName: "claudecode",
		EventParser: adapter.EventParserSpec{
			Type:    &parserType,
			Profile: &parserProfile,
		},
	}
}

type capturingRuntime struct {
	startCtx context.Context
	events   chan agenruntime.Event
}

func (r *capturingRuntime) Start(ctx context.Context, _ agenruntime.Spec) error {
	r.startCtx = ctx
	if r.events == nil {
		r.events = closedRuntimeEvents()
	}
	return nil
}

func (r *capturingRuntime) Write([]byte) (int, error) { return 0, nil }
func (r *capturingRuntime) Interrupt() error          { return nil }
func (r *capturingRuntime) Stop() error               { return nil }
func (r *capturingRuntime) Resize(uint16, uint16) error {
	return nil
}
func (r *capturingRuntime) Wait() error                      { return nil }
func (r *capturingRuntime) Close() error                     { return nil }
func (r *capturingRuntime) PID() int                         { return 0 }
func (r *capturingRuntime) State() agenruntime.State         { return agenruntime.StateRunning }
func (r *capturingRuntime) Events() <-chan agenruntime.Event { return r.events }
func (r *capturingRuntime) Snapshot() agenruntime.Snapshot {
	return agenruntime.Snapshot{State: agenruntime.StateRunning}
}

func closedRuntimeEvents() chan agenruntime.Event {
	ch := make(chan agenruntime.Event)
	close(ch)
	return ch
}

func writeClaudeDiscoveryFile(t *testing.T, path, cwd, preview string) {
	t.Helper()

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll(%s) error = %v", filepath.Dir(path), err)
	}

	body := `{"cwd":"` + cwd + `","version":"1.0.0","message":{"role":"assistant","content":[{"type":"text","text":"` + preview + `"}]}}` + "\n"
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("WriteFile(%s) error = %v", path, err)
	}
}
