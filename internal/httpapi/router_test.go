package httpapi

import (
	"bufio"
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/agendash/AgenLeash/internal/event"
	"github.com/agendash/AgenLeash/internal/model"
	"github.com/agendash/AgenLeash/internal/store"
)

func TestRouterListAndStart(t *testing.T) {
	st := store.NewMemoryStore()
	hub := event.NewHub(8)
	router := NewRouter("secret", st, hub)
	router.NewID = func(prefix string) string {
		return prefix + "_test"
	}

	srv := httptest.NewServer(router)
	defer srv.Close()

	startBody := StartSessionRequest{
		Adapter:          "claudecode",
		CWD:              ".",
		AgentVersionHint: "1.12.0",
		StartMode:        string(model.StartModeNew),
		Args:             []string{"--agent-mode"},
	}
	startResp := doJSONRequest(t, srv.URL+"/api/v1/agent/start", "secret", startBody)
	defer startResp.Body.Close()
	if startResp.StatusCode != http.StatusCreated {
		t.Fatalf("start status = %d, want %d", startResp.StatusCode, http.StatusCreated)
	}

	var started StartSessionResponse
	decodeBody(t, startResp.Body, &started)
	if started.SessionID != "sess_test" {
		t.Fatalf("SessionID = %q, want sess_test", started.SessionID)
	}
	if started.Session.Adapter != "claudecode" {
		t.Fatalf("Adapter = %q, want claudecode", started.Session.Adapter)
	}

	snapshot := st.Snapshot()
	if len(snapshot.Sessions) != 1 {
		t.Fatalf("sessions = %d, want 1", len(snapshot.Sessions))
	}
	if len(snapshot.Workspaces) != 1 {
		t.Fatalf("workspaces = %d, want 1", len(snapshot.Workspaces))
	}
	if len(snapshot.Conversations) != 1 {
		t.Fatalf("conversations = %d, want 1", len(snapshot.Conversations))
	}

	listResp := doRequest(t, http.MethodGet, srv.URL+"/api/v1/sessions", "secret", nil)
	defer listResp.Body.Close()
	if listResp.StatusCode != http.StatusOK {
		t.Fatalf("list status = %d, want %d", listResp.StatusCode, http.StatusOK)
	}

	var listed SessionListResponse
	decodeBody(t, listResp.Body, &listed)
	if len(listed.Sessions) != 1 {
		t.Fatalf("listed sessions = %d, want 1", len(listed.Sessions))
	}
	if listed.Sessions[0].Conversation.StartMode != model.StartModeNew {
		t.Fatalf("start mode = %q, want new", listed.Sessions[0].Conversation.StartMode)
	}
}

func TestRouterRejectsMissingToken(t *testing.T) {
	router := NewRouter("secret", store.NewMemoryStore(), event.NewHub(8))
	srv := httptest.NewServer(router)
	defer srv.Close()

	resp := doRequest(t, http.MethodGet, srv.URL+"/api/v1/sessions", "", nil)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusUnauthorized)
	}
}

func TestRouterStatsPageDisabledByDefault(t *testing.T) {
	router := NewRouter("secret", store.NewMemoryStore(), event.NewHub(8))
	srv := httptest.NewServer(router)
	defer srv.Close()

	resp := doRequest(t, http.MethodGet, srv.URL+"/stats", "", nil)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusNotFound)
	}
}

func TestRouterStatsPageLoadsWithoutTokenWhenWebEnabled(t *testing.T) {
	router := NewRouter("secret", store.NewMemoryStore(), event.NewHub(8))
	router.EnableWeb = true
	srv := httptest.NewServer(router)
	defer srv.Close()

	resp := doRequest(t, http.MethodGet, srv.URL+"/stats", "", nil)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusOK)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	if !strings.Contains(string(body), "AgenLeash Stats") {
		t.Fatalf("stats page missing title: %s", string(body))
	}
}

func TestRouterStatsRequiresToken(t *testing.T) {
	router := NewRouter("secret", store.NewMemoryStore(), event.NewHub(8))
	srv := httptest.NewServer(router)
	defer srv.Close()

	resp := doRequest(t, http.MethodGet, srv.URL+"/api/v1/stats", "", nil)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusUnauthorized)
	}
}

func TestRouterStatsResponse(t *testing.T) {
	st := store.NewMemoryStore()
	now := time.Date(2026, 4, 21, 6, 0, 0, 0, time.UTC)
	for _, session := range []model.Session{
		{
			ID:             "sess_claude_primary",
			Adapter:        "claudecode",
			Origin:         model.SessionOriginDiscovered,
			State:          model.SessionStateStopped,
			ConversationID: "conv_1",
			WorkspacePath:  "/workspaces/pokemon",
			DetailPath:     "/data/agents/.claude/projects/pokemon/abc.jsonl",
			CreatedAt:      now.Add(-2 * time.Hour),
			LastSeen:       now.Add(-10 * time.Minute),
		},
		{
			ID:                   "sess_claude_subagent",
			Adapter:              "claudecode",
			Origin:               model.SessionOriginDiscovered,
			State:                model.SessionStateStopped,
			ConversationID:       "conv_2",
			NativeConversationID: "agent-123",
			WorkspacePath:        "/workspaces/pokemon",
			DetailPath:           "/data/agents/.claude/projects/pokemon/abc/subagents/agent-123.jsonl",
			CreatedAt:            now.Add(-90 * time.Minute),
			LastSeen:             now.Add(-8 * time.Minute),
		},
		{
			ID:             "sess_codex",
			Adapter:        "codex",
			Origin:         model.SessionOriginManaged,
			State:          model.SessionStateRunning,
			ConversationID: "conv_3",
			WorkspacePath:  "/workspaces/tools",
			CreatedAt:      now.Add(-30 * time.Minute),
			LastSeen:       now.Add(-2 * time.Minute),
		},
	} {
		if err := st.UpsertSession(session); err != nil {
			t.Fatalf("UpsertSession: %v", err)
		}
	}

	router := NewRouter("secret", st, event.NewHub(8))
	router.Now = func() time.Time { return now }
	srv := httptest.NewServer(router)
	defer srv.Close()

	resp := doRequest(t, http.MethodGet, srv.URL+"/api/v1/stats?top=5", "secret", nil)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusOK)
	}

	var stats StatsResponse
	decodeBody(t, resp.Body, &stats)
	if stats.Totals.Sessions != 3 {
		t.Fatalf("total sessions = %d, want 3", stats.Totals.Sessions)
	}
	if stats.Claudecode == nil || stats.Claudecode.SubagentSessions != 1 {
		t.Fatalf("claudecode stats = %#v", stats.Claudecode)
	}
	if len(stats.TopWorkspaces) == 0 || stats.TopWorkspaces[0].Path != "/workspaces/pokemon" {
		t.Fatalf("top workspaces = %#v", stats.TopWorkspaces)
	}
}

func TestRouterGetSessionDetail(t *testing.T) {
	st := store.NewMemoryStore()
	createdAt := time.Now().UTC().Add(-2 * time.Minute).Round(0)
	lastSeen := createdAt.Add(90 * time.Second)
	session := model.Session{
		ID:                   "sess_hist",
		Adapter:              "codex",
		State:                model.SessionStateStopped,
		NativeConversationID: "native_conv_123",
		WorkspaceID:          "ws_hist",
		WorkspacePath:        "/srv/workspaces/agendash",
		DetailPath:           "/home/tester/.codex/state_5.sqlite#threads/019d22f5-895d-7172-9100-adbee0c69948",
		LastOutputPreview:    "Latest agent reply preview",
		StartMode:            model.StartModeResume,
		ResumeStrategy:       model.ResumeStrategyNativeID,
		CreatedAt:            createdAt,
		LastSeen:             lastSeen,
	}
	if err := st.UpsertSession(session); err != nil {
		t.Fatalf("UpsertSession: %v", err)
	}

	router := NewRouter("secret", st, event.NewHub(8))
	srv := httptest.NewServer(router)
	defer srv.Close()

	resp := doRequest(t, http.MethodGet, srv.URL+"/api/v1/sessions/sess_hist", "secret", nil)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusOK)
	}

	var detail SessionDetailResponse
	decodeBody(t, resp.Body, &detail)
	if detail.Session.ID != session.ID {
		t.Fatalf("id = %q, want %q", detail.Session.ID, session.ID)
	}
	if detail.Session.Conversation.NativeID != session.NativeConversationID {
		t.Fatalf("native_id = %q, want %q", detail.Session.Conversation.NativeID, session.NativeConversationID)
	}
	if detail.Session.Workspace.CWD != session.WorkspacePath {
		t.Fatalf("workspace cwd = %q, want %q", detail.Session.Workspace.CWD, session.WorkspacePath)
	}
	if detail.Session.DetailPath != session.DetailPath {
		t.Fatalf("detail_path = %q, want %q", detail.Session.DetailPath, session.DetailPath)
	}
	if detail.Session.LastOutputPreview != session.LastOutputPreview {
		t.Fatalf("last_output_preview = %q, want %q", detail.Session.LastOutputPreview, session.LastOutputPreview)
	}
	if !detail.Session.CreatedAt.Equal(createdAt) {
		t.Fatalf("created_at = %s, want %s", detail.Session.CreatedAt, createdAt)
	}
	if !detail.Session.LastSeen.Equal(lastSeen) {
		t.Fatalf("last_seen = %s, want %s", detail.Session.LastSeen, lastSeen)
	}
}

func TestSessionSummarySuppressesRuntimeDiagnosticPreview(t *testing.T) {
	session := model.Session{
		ID:                "sess_noise",
		Adapter:           "codex",
		LastOutputPreview: "Reading prompt from stdin...\n2026-05-18T07:31:08.226449Z ERROR codex_core_skills::manager: failed to install system skills",
	}
	summary := SessionSummaryFromModel(session, time.Now().UTC())
	if summary.LastOutputPreview != "" {
		t.Fatalf("last_output_preview = %q, want empty", summary.LastOutputPreview)
	}

	session.LastOutputPreview = "Latest agent reply preview"
	summary = SessionSummaryFromModel(session, time.Now().UTC())
	if summary.LastOutputPreview != session.LastOutputPreview {
		t.Fatalf("last_output_preview = %q, want %q", summary.LastOutputPreview, session.LastOutputPreview)
	}
}

func TestRouterGetSessionFilePreview(t *testing.T) {
	st := store.NewMemoryStore()
	session := model.Session{
		ID:            "sess_preview",
		Adapter:       "codex",
		State:         model.SessionStateStopped,
		WorkspacePath: "/workspaces/demo",
		CreatedAt:     time.Now().UTC().Add(-2 * time.Minute),
		LastSeen:      time.Now().UTC(),
	}
	if err := st.UpsertSession(session); err != nil {
		t.Fatalf("UpsertSession: %v", err)
	}

	router := NewRouter("secret", st, event.NewHub(8))
	router.GetFilePreviewHandler = func(_ *http.Request, sessionID string, path string) (FilePreviewResponse, error) {
		if sessionID != "sess_preview" {
			t.Fatalf("sessionID = %q, want sess_preview", sessionID)
		}
		if path != "/workspaces/demo/docker-compose.yml" {
			t.Fatalf("path = %q", path)
		}
		return FilePreviewResponse{
			Path:     path,
			Name:     "docker-compose.yml",
			Kind:     "text",
			Language: "yaml",
			Encoding: "utf-8",
			Content:  "services: {}",
		}, nil
	}

	srv := httptest.NewServer(router)
	defer srv.Close()

	resp := doRequest(
		t,
		http.MethodGet,
		srv.URL+"/api/v1/sessions/sess_preview/files/preview?path="+url.QueryEscape("/workspaces/demo/docker-compose.yml"),
		"secret",
		nil,
	)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusOK)
	}

	var preview FilePreviewResponse
	decodeBody(t, resp.Body, &preview)
	if preview.Kind != "text" || preview.Language != "yaml" || preview.Content != "services: {}" {
		t.Fatalf("preview = %#v", preview)
	}
}

func TestRouterPostSessionMessageRoutesCommand(t *testing.T) {
	st := store.NewMemoryStore()
	session := model.Session{
		ID:        "sess_msg",
		Adapter:   "codex",
		State:     model.SessionStatePaused,
		CreatedAt: time.Now().UTC().Add(-time.Minute),
		LastSeen:  time.Now().UTC(),
	}
	if err := st.UpsertSession(session); err != nil {
		t.Fatalf("UpsertSession: %v", err)
	}

	router := NewRouter("secret", st, event.NewHub(8))
	cmdCh := make(chan event.Command, 1)
	router.CommandHandler = func(_ context.Context, sessionID string, cmd event.Command) error {
		if sessionID != "sess_msg" {
			t.Fatalf("sessionID = %q, want sess_msg", sessionID)
		}
		cmdCh <- cmd
		return nil
	}

	srv := httptest.NewServer(router)
	defer srv.Close()

	body := SessionMessageRequest{Content: "approve"}
	resp := doJSONRequest(t, srv.URL+"/api/v1/sessions/sess_msg/messages", "secret", body)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusAccepted {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusAccepted)
	}

	select {
	case cmd := <-cmdCh:
		if cmd.MsgType != event.CommandUserMessage {
			t.Fatalf("command type = %q, want %q", cmd.MsgType, event.CommandUserMessage)
		}
		if cmd.Content != "approve" {
			t.Fatalf("command content = %q, want approve", cmd.Content)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for command")
	}
}

func TestRouterListSessionsPrioritizesReviewableItems(t *testing.T) {
	st := store.NewMemoryStore()
	now := time.Date(2026, 4, 20, 6, 0, 0, 0, time.UTC)
	sessions := []model.Session{
		{
			ID:        "sess_running",
			Adapter:   "claudecode",
			State:     model.SessionStateRunning,
			CreatedAt: now.Add(-15 * time.Minute),
			LastSeen:  now.Add(-2 * time.Minute),
		},
		{
			ID:        "sess_recent_stop",
			Adapter:   "codex",
			State:     model.SessionStateStopped,
			CreatedAt: now.Add(-2 * time.Hour),
			LastSeen:  now.Add(-4 * time.Minute),
		},
		{
			ID:        "sess_paused",
			Adapter:   "codex",
			State:     model.SessionStatePaused,
			CreatedAt: now.Add(-40 * time.Minute),
			LastSeen:  now.Add(-1 * time.Minute),
		},
	}
	for _, session := range sessions {
		if err := st.UpsertSession(session); err != nil {
			t.Fatalf("UpsertSession(%s): %v", session.ID, err)
		}
	}

	router := NewRouter("secret", st, event.NewHub(8))
	router.Now = func() time.Time { return now }

	srv := httptest.NewServer(router)
	defer srv.Close()

	resp := doRequest(t, http.MethodGet, srv.URL+"/api/v1/sessions", "secret", nil)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusOK)
	}

	var listed SessionListResponse
	decodeBody(t, resp.Body, &listed)
	if len(listed.Sessions) != 3 {
		t.Fatalf("listed sessions = %d, want 3", len(listed.Sessions))
	}

	if listed.Sessions[0].ID != "sess_paused" {
		t.Fatalf("first session = %q, want sess_paused", listed.Sessions[0].ID)
	}
	if listed.Sessions[0].Highlight == nil || listed.Sessions[0].Highlight.Kind != model.HighlightNeedsResponse {
		t.Fatalf("first highlight = %#v, want needs_response", listed.Sessions[0].Highlight)
	}
	if listed.Sessions[1].ID != "sess_recent_stop" {
		t.Fatalf("second session = %q, want sess_recent_stop", listed.Sessions[1].ID)
	}
	if listed.Sessions[1].Highlight == nil || listed.Sessions[1].Highlight.Kind != model.HighlightRecentCompletion {
		t.Fatalf("second highlight = %#v, want recent_completion", listed.Sessions[1].Highlight)
	}
}

func TestRouterListSessionsRespectsLimitQuery(t *testing.T) {
	st := store.NewMemoryStore()
	now := time.Date(2026, 4, 20, 6, 0, 0, 0, time.UTC)
	for i, session := range []model.Session{
		{
			ID:        "sess_a",
			Adapter:   "claudecode",
			State:     model.SessionStateRunning,
			CreatedAt: now.Add(-30 * time.Minute),
			LastSeen:  now.Add(-3 * time.Minute),
		},
		{
			ID:        "sess_b",
			Adapter:   "codex",
			State:     model.SessionStatePaused,
			CreatedAt: now.Add(-20 * time.Minute),
			LastSeen:  now.Add(-1 * time.Minute),
		},
		{
			ID:        "sess_c",
			Adapter:   "codex",
			State:     model.SessionStateStopped,
			CreatedAt: now.Add(-10 * time.Minute),
			LastSeen:  now.Add(-2 * time.Minute),
		},
	} {
		if err := st.UpsertSession(session); err != nil {
			t.Fatalf("UpsertSession(%d): %v", i, err)
		}
	}

	router := NewRouter("secret", st, event.NewHub(8))
	router.Now = func() time.Time { return now }
	srv := httptest.NewServer(router)
	defer srv.Close()

	resp := doRequest(t, http.MethodGet, srv.URL+"/api/v1/sessions?limit=2", "secret", nil)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusOK)
	}

	var listed SessionListResponse
	decodeBody(t, resp.Body, &listed)
	if len(listed.Sessions) != 2 {
		t.Fatalf("listed sessions = %d, want 2", len(listed.Sessions))
	}
	if listed.Sessions[0].ID != "sess_b" {
		t.Fatalf("first session = %q, want sess_b", listed.Sessions[0].ID)
	}
	if listed.Sessions[1].ID != "sess_c" {
		t.Fatalf("second session = %q, want sess_c", listed.Sessions[1].ID)
	}
}

func TestRouterSessionEvents(t *testing.T) {
	st := store.NewMemoryStore()
	hub := event.NewHub(16)
	session := model.Session{
		ID:             "sess_test",
		Adapter:        "claudecode",
		StartMode:      model.StartModeNew,
		ResumeStrategy: model.ResumeStrategyProcessOnly,
		State:          model.SessionStateRunning,
		CreatedAt:      time.Now().UTC(),
		LastSeen:       time.Now().UTC(),
	}
	if err := st.UpsertSession(session); err != nil {
		t.Fatalf("UpsertSession: %v", err)
	}
	hub.Publish(session.ID, event.MessageDelta(session.ID, "msg-1", "assistant", "hello"))

	cmdCh := make(chan event.Command, 1)
	router := NewRouter("secret", st, hub)
	router.CommandHandler = func(ctx context.Context, sessionID string, cmd event.Command) error {
		select {
		case cmdCh <- cmd:
		default:
		}
		return nil
	}

	srv := httptest.NewServer(router)
	defer srv.Close()

	wsURL := strings.Replace(srv.URL, "http://", "ws://", 1) + "/ws/v1/sessions/sess_test/events?token=secret"
	conn, reader, cleanup := dialWebSocket(t, wsURL)
	defer cleanup()

	first := readWSFrame(t, reader)
	var evt event.Event
	decodeJSON(t, first, &evt)
	if evt.MsgType != event.MsgTypeSessionSnapshot {
		t.Fatalf("first event = %q, want session_snapshot", evt.MsgType)
	}

	second := readWSFrame(t, reader)
	decodeJSON(t, second, &evt)
	if evt.MsgType != event.MsgTypeMessageDelta {
		t.Fatalf("second event = %q, want message_delta", evt.MsgType)
	}

	third := readWSFrame(t, reader)
	decodeJSON(t, third, &evt)
	if evt.MsgType != event.MsgTypeSyncEnd {
		t.Fatalf("third event = %q, want sync_end", evt.MsgType)
	}

	if err := writeClientJSON(conn, event.Command{MsgType: event.CommandUserMessage, Content: "ping"}); err != nil {
		t.Fatalf("writeClientJSON: %v", err)
	}

	select {
	case cmd := <-cmdCh:
		if cmd.MsgType != event.CommandUserMessage {
			t.Fatalf("command = %q, want user_message", cmd.MsgType)
		}
		if cmd.Content != "ping" {
			t.Fatalf("command content = %q, want ping", cmd.Content)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for routed command")
	}
}

func doJSONRequest(t *testing.T, rawURL, token string, body any) *http.Response {
	t.Helper()

	var payload io.Reader
	if body != nil {
		buf, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("json.Marshal: %v", err)
		}
		payload = bytes.NewReader(buf)
	}
	req, err := http.NewRequest(http.MethodPost, rawURL, payload)
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("X-AgenLeash-Token", token)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	return resp
}

func doRequest(t *testing.T, method, rawURL, token string, body io.Reader) *http.Response {
	t.Helper()

	req, err := http.NewRequest(method, rawURL, body)
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	if token != "" {
		req.Header.Set("X-AgenLeash-Token", token)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	return resp
}

func decodeBody(t *testing.T, body io.ReadCloser, out any) {
	t.Helper()
	defer body.Close()

	if err := json.NewDecoder(body).Decode(out); err != nil {
		t.Fatalf("Decode: %v", err)
	}
}

func decodeJSON(t *testing.T, raw []byte, out any) {
	t.Helper()
	if err := json.NewDecoder(bytes.NewReader(raw)).Decode(out); err != nil {
		t.Fatalf("Decode: %v", err)
	}
}

func dialWebSocket(t *testing.T, rawURL string) (net.Conn, *bufio.Reader, func()) {
	t.Helper()

	parsed, err := url.Parse(rawURL)
	if err != nil {
		t.Fatalf("url.Parse: %v", err)
	}

	host := parsed.Host
	if !strings.Contains(host, ":") {
		host += ":80"
	}

	conn, err := net.Dial("tcp", host)
	if err != nil {
		t.Fatalf("net.Dial: %v", err)
	}

	key := make([]byte, 16)
	if _, err := rand.Read(key); err != nil {
		t.Fatalf("rand.Read: %v", err)
	}
	secKey := base64.StdEncoding.EncodeToString(key)

	req := fmt.Sprintf("GET %s HTTP/1.1\r\nHost: %s\r\nUpgrade: websocket\r\nConnection: Upgrade\r\nSec-WebSocket-Version: 13\r\nSec-WebSocket-Key: %s\r\nX-AgenLeash-Token: secret\r\n\r\n", parsed.RequestURI(), parsed.Host, secKey)
	if _, err := conn.Write([]byte(req)); err != nil {
		_ = conn.Close()
		t.Fatalf("write handshake: %v", err)
	}

	reader := bufio.NewReader(conn)
	status, err := reader.ReadString('\n')
	if err != nil {
		_ = conn.Close()
		t.Fatalf("read status: %v", err)
	}
	if !strings.Contains(status, "101") {
		_ = conn.Close()
		t.Fatalf("handshake status = %q, want 101", strings.TrimSpace(status))
	}

	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			_ = conn.Close()
			t.Fatalf("read header: %v", err)
		}
		if line == "\r\n" {
			break
		}
	}

	return conn, reader, func() {
		_ = conn.Close()
	}
}

func readWSFrame(t *testing.T, r *bufio.Reader) []byte {
	t.Helper()

	header := make([]byte, 2)
	if _, err := io.ReadFull(r, header); err != nil {
		t.Fatalf("read frame header: %v", err)
	}
	length := int64(header[1] & 0x7f)
	switch length {
	case 126:
		var ext [2]byte
		if _, err := io.ReadFull(r, ext[:]); err != nil {
			t.Fatalf("read frame ext len: %v", err)
		}
		length = int64(binary.BigEndian.Uint16(ext[:]))
	case 127:
		var ext [8]byte
		if _, err := io.ReadFull(r, ext[:]); err != nil {
			t.Fatalf("read frame ext len: %v", err)
		}
		length = int64(binary.BigEndian.Uint64(ext[:]))
	}

	payload := make([]byte, int(length))
	if _, err := io.ReadFull(r, payload); err != nil {
		t.Fatalf("read frame payload: %v", err)
	}
	return payload
}

func writeClientJSON(conn net.Conn, v any) error {
	payload, err := json.Marshal(v)
	if err != nil {
		return err
	}
	return writeClientFrame(conn, 0x1, payload)
}

func writeClientFrame(conn net.Conn, opcode byte, payload []byte) error {
	mask := [4]byte{1, 2, 3, 4}
	header := []byte{0x80 | opcode}
	length := len(payload)
	switch {
	case length < 126:
		header = append(header, 0x80|byte(length))
	case length <= 0xffff:
		header = append(header, 0x80|126, 0, 0)
		binary.BigEndian.PutUint16(header[len(header)-2:], uint16(length))
	default:
		header = append(header, 0x80|127, 0, 0, 0, 0, 0, 0, 0, 0)
		binary.BigEndian.PutUint64(header[len(header)-8:], uint64(length))
	}
	header = append(header, mask[:]...)

	frame := make([]byte, len(payload))
	copy(frame, payload)
	for i := range frame {
		frame[i] ^= mask[i%4]
	}

	if _, err := conn.Write(header); err != nil {
		return err
	}
	_, err := conn.Write(frame)
	return err
}
