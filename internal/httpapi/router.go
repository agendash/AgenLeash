package httpapi

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/agendash/AgenLeash/internal/event"
	"github.com/agendash/AgenLeash/internal/model"
	"github.com/agendash/AgenLeash/internal/store"
	"github.com/agendash/AgenLeash/internal/ws"
)

type CommandHandler = ws.CommandHandler
type StartSessionHandler func(*http.Request, StartSessionRequest) (model.Session, error)
type ListSessionsHandler func(*http.Request) ([]model.Session, error)
type GetSessionHandler func(*http.Request, string) (SessionDetailResponse, error)
type GetFilePreviewHandler func(*http.Request, string, string) (FilePreviewResponse, error)
type GetStatsHandler func(*http.Request) (StatsResponse, error)

type Router struct {
	Token                 string
	EnableWeb             bool
	Store                 store.Store
	Hub                   *event.Hub
	CommandHandler        CommandHandler
	StartSessionHandler   StartSessionHandler
	ListSessionsHandler   ListSessionsHandler
	GetSessionHandler     GetSessionHandler
	GetFilePreviewHandler GetFilePreviewHandler
	GetStatsHandler       GetStatsHandler
	Now                   func() time.Time
	NewID                 func(prefix string) string
}

func NewRouter(token string, st store.Store, hub *event.Hub) *Router {
	return &Router{
		Token: token,
		Store: st,
		Hub:   hub,
		Now: func() time.Time {
			return time.Now().UTC()
		},
		NewID: defaultID,
	}
}

func (r *Router) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	switch {
	case req.Method == http.MethodGet && req.URL.Path == "/healthz":
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
	case req.Method == http.MethodGet && req.URL.Path == "/stats":
		if !r.EnableWeb {
			http.NotFound(w, req)
			return
		}
		r.handleStatsPage(w, req)
	case req.Method == http.MethodGet && req.URL.Path == "/api/v1/stats":
		r.handleGetStats(w, req)
	case req.Method == http.MethodGet && req.URL.Path == "/api/v1/sessions":
		r.handleListSessions(w, req)
	case req.Method == http.MethodPost && strings.HasPrefix(req.URL.Path, "/api/v1/sessions/") && strings.HasSuffix(req.URL.Path, "/messages"):
		r.handlePostSessionMessage(w, req)
	case req.Method == http.MethodGet && strings.HasPrefix(req.URL.Path, "/api/v1/sessions/") && strings.HasSuffix(req.URL.Path, "/files/preview"):
		r.handleGetSessionFilePreview(w, req)
	case req.Method == http.MethodGet && strings.HasPrefix(req.URL.Path, "/api/v1/sessions/"):
		r.handleGetSession(w, req)
	case req.Method == http.MethodPost && req.URL.Path == "/api/v1/agent/start":
		r.handleStartSession(w, req)
	case req.Method == http.MethodGet && strings.HasPrefix(req.URL.Path, "/ws/v1/sessions/") && strings.HasSuffix(req.URL.Path, "/events"):
		r.handleSessionEvents(w, req)
	default:
		http.NotFound(w, req)
	}
}

func (r *Router) handleGetSessionFilePreview(w http.ResponseWriter, req *http.Request) {
	if !r.authorized(req) {
		writeJSONError(w, http.StatusUnauthorized, "missing or invalid token")
		return
	}
	if r.Store == nil {
		writeJSONError(w, http.StatusInternalServerError, "store is not configured")
		return
	}

	sessionID := strings.TrimPrefix(req.URL.Path, "/api/v1/sessions/")
	sessionID = strings.TrimSuffix(sessionID, "/files/preview")
	sessionID = strings.TrimSpace(strings.Trim(sessionID, "/"))
	if sessionID == "" || strings.Contains(sessionID, "/") {
		http.NotFound(w, req)
		return
	}

	path := strings.TrimSpace(req.URL.Query().Get("path"))
	if path == "" {
		writeJSONError(w, http.StatusBadRequest, "path is required")
		return
	}
	if r.GetFilePreviewHandler == nil {
		writeJSONError(w, http.StatusNotImplemented, "file preview is not configured")
		return
	}

	resp, err := r.GetFilePreviewHandler(req, sessionID, path)
	if err != nil {
		writeJSONError(w, statusCode(err), err.Error())
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

func (r *Router) handleGetStats(w http.ResponseWriter, req *http.Request) {
	if !r.authorized(req) {
		writeJSONError(w, http.StatusUnauthorized, "missing or invalid token")
		return
	}
	if r.Store == nil {
		writeJSONError(w, http.StatusInternalServerError, "store is not configured")
		return
	}

	var (
		resp StatsResponse
		err  error
	)
	if r.GetStatsHandler != nil {
		resp, err = r.GetStatsHandler(req)
		if err != nil {
			writeJSONError(w, statusCode(err), err.Error())
			return
		}
	} else {
		resp = BuildStatsResponse(r.Store.ListSessions(), r.now(), ParseStatsTopN(req))
	}
	writeJSON(w, http.StatusOK, resp)
}

func (r *Router) handleListSessions(w http.ResponseWriter, req *http.Request) {
	if !r.authorized(req) {
		writeJSONError(w, http.StatusUnauthorized, "missing or invalid token")
		return
	}
	if r.Store == nil {
		writeJSONError(w, http.StatusInternalServerError, "store is not configured")
		return
	}

	var (
		sessions []model.Session
		err      error
	)
	if r.ListSessionsHandler != nil {
		sessions, err = r.ListSessionsHandler(req)
		if err != nil {
			writeJSONError(w, statusCode(err), err.Error())
			return
		}
	} else {
		sessions = r.Store.ListSessions()
	}
	now := r.now()
	sort.SliceStable(sessions, func(i, j int) bool {
		leftRank, leftUpdatedAt := sessionPriority(sessions[i], now)
		rightRank, rightUpdatedAt := sessionPriority(sessions[j], now)
		if leftRank != rightRank {
			return leftRank < rightRank
		}
		if !leftUpdatedAt.Equal(rightUpdatedAt) {
			return leftUpdatedAt.After(rightUpdatedAt)
		}
		if !sessions[i].LastSeen.Equal(sessions[j].LastSeen) {
			return sessions[i].LastSeen.After(sessions[j].LastSeen)
		}
		if sessions[i].CreatedAt.Equal(sessions[j].CreatedAt) {
			return sessions[i].ID < sessions[j].ID
		}
		return sessions[i].CreatedAt.After(sessions[j].CreatedAt)
	})

	if limit := parseSessionListLimit(req); limit > 0 && len(sessions) > limit {
		sessions = sessions[:limit]
	}

	resp := SessionListResponse{Sessions: make([]SessionSummary, 0, len(sessions))}
	for _, session := range sessions {
		resp.Sessions = append(resp.Sessions, SessionSummaryFromModel(session, now))
	}

	writeJSON(w, http.StatusOK, resp)
}

func parseSessionListLimit(req *http.Request) int {
	raw := strings.TrimSpace(req.URL.Query().Get("limit"))
	if raw == "" {
		return 0
	}
	limit, err := strconv.Atoi(raw)
	if err != nil || limit <= 0 {
		return 0
	}
	if limit > 1000 {
		return 1000
	}
	return limit
}

func (r *Router) handleGetSession(w http.ResponseWriter, req *http.Request) {
	if !r.authorized(req) {
		writeJSONError(w, http.StatusUnauthorized, "missing or invalid token")
		return
	}
	if r.Store == nil {
		writeJSONError(w, http.StatusInternalServerError, "store is not configured")
		return
	}

	sessionID := strings.TrimPrefix(req.URL.Path, "/api/v1/sessions/")
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" || strings.Contains(sessionID, "/") {
		http.NotFound(w, req)
		return
	}

	var (
		session model.Session
		ok      bool
	)
	if r.GetSessionHandler != nil {
		resp, err := r.GetSessionHandler(req, sessionID)
		if err != nil {
			writeJSONError(w, statusCode(err), err.Error())
			return
		}
		writeJSON(w, http.StatusOK, resp)
		return
	} else {
		session, ok = r.Store.GetSession(sessionID)
	}
	if !ok {
		writeJSONError(w, http.StatusNotFound, "session not found")
		return
	}

	writeJSON(w, http.StatusOK, SessionDetailResponse{
		Session: SessionSummaryFromModel(session, r.now()),
	})
}

func (r *Router) handleStartSession(w http.ResponseWriter, req *http.Request) {
	if !r.authorized(req) {
		writeJSONError(w, http.StatusUnauthorized, "missing or invalid token")
		return
	}
	if r.Store == nil {
		writeJSONError(w, http.StatusInternalServerError, "store is not configured")
		return
	}

	var in StartSessionRequest
	if err := json.NewDecoder(req.Body).Decode(&in); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if strings.TrimSpace(in.Adapter) == "" {
		writeJSONError(w, http.StatusBadRequest, "adapter is required")
		return
	}
	if strings.TrimSpace(in.CWD) == "" {
		writeJSONError(w, http.StatusBadRequest, "cwd is required")
		return
	}

	if r.StartSessionHandler != nil {
		session, err := r.StartSessionHandler(req, in)
		if err != nil {
			writeJSONError(w, statusCode(err), err.Error())
			return
		}
		writeJSON(w, http.StatusCreated, StartSessionResponse{
			SessionID: session.ID,
			Session:   SessionSummaryFromModel(session, r.now()),
		})
		return
	}

	startMode := strings.ToLower(strings.TrimSpace(in.StartMode))
	if startMode == "" {
		startMode = string(model.StartModeNew)
	}
	if startMode != string(model.StartModeNew) && startMode != string(model.StartModeResume) {
		writeJSONError(w, http.StatusBadRequest, "start_mode must be new or resume")
		return
	}
	if startMode == string(model.StartModeResume) && strings.TrimSpace(in.ConversationID) == "" {
		writeJSONError(w, http.StatusBadRequest, "conversation_id is required for resume")
		return
	}

	cwd, err := canonicalPath(in.CWD)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}

	now := r.now()
	sessionID := r.newID("sess")
	if sessionID == "" {
		sessionID = defaultID("sess")
	}

	conversationID := strings.TrimSpace(in.ConversationID)
	if conversationID == "" {
		conversationID = r.newID("conv")
		if conversationID == "" {
			conversationID = defaultID("conv")
		}
	}

	workspaceID := r.newID("ws")
	if workspaceID == "" {
		workspaceID = defaultID("ws")
	}

	resumeStrategy := model.ResumeStrategyProcessOnly
	nativeID := ""
	if startMode == string(model.StartModeResume) {
		resumeStrategy = model.ResumeStrategyNativeID
		nativeID = conversationID
	}

	session := model.Session{
		ID:                   sessionID,
		Adapter:              strings.TrimSpace(in.Adapter),
		Origin:               model.SessionOriginManaged,
		NativeConversationID: nativeID,
		StartMode:            model.StartMode(startMode),
		ResumeStrategy:       resumeStrategy,
		WorkspacePath:        cwd,
		WorkspaceRoot:        cwd,
		WorkspaceFingerprint: workspaceFingerprint(cwd, strings.TrimSpace(in.Adapter), in.AgentVersionHint, conversationID),
		GitRoot:              cwd,
		State:                model.SessionStateRunning,
		ConversationID:       conversationID,
		WorkspaceID:          workspaceID,
		CreatedAt:            now,
		LastSeen:             now,
	}

	if err := r.Store.UpsertWorkspace(model.Workspace{
		ID:          workspaceID,
		Path:        cwd,
		Root:        cwd,
		Fingerprint: session.WorkspaceFingerprint,
		GitRoot:     cwd,
		CreatedAt:   now,
		UpdatedAt:   now,
	}); err != nil {
		writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}

	if err := r.Store.UpsertConversation(model.Conversation{
		ID:          conversationID,
		SessionID:   sessionID,
		NativeID:    nativeID,
		WorkspaceID: workspaceID,
		CreatedAt:   now,
		UpdatedAt:   now,
	}); err != nil {
		writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}

	if err := r.Store.UpsertSession(session); err != nil {
		writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}

	if r.Hub != nil {
		r.Hub.Publish(sessionID, event.SessionSnapshot(session))
		r.Hub.Publish(sessionID, event.StateChanged(sessionID, session.State))
		if nativeID != "" {
			r.Hub.Publish(sessionID, event.ConversationBound(session))
		}
		r.Hub.Publish(sessionID, event.WorkspaceUpdated(session))
	}

	writeJSON(w, http.StatusCreated, StartSessionResponse{
		SessionID: sessionID,
		Session:   SessionSummaryFromModel(session, r.now()),
	})
}

func (r *Router) handleSessionEvents(w http.ResponseWriter, req *http.Request) {
	if !r.authorized(req) {
		writeJSONError(w, http.StatusUnauthorized, "missing or invalid token")
		return
	}
	if r.Store == nil || r.Hub == nil {
		writeJSONError(w, http.StatusInternalServerError, "router is not fully configured")
		return
	}

	sessionID := strings.TrimPrefix(req.URL.Path, "/ws/v1/sessions/")
	sessionID = strings.TrimSuffix(sessionID, "/events")
	sessionID = strings.Trim(sessionID, "/")
	if sessionID == "" {
		writeJSONError(w, http.StatusNotFound, "session not found")
		return
	}

	session, ok := r.Store.GetSession(sessionID)
	if !ok {
		writeJSONError(w, http.StatusNotFound, "session not found")
		return
	}

	snapshot := func(string) (event.Event, bool) {
		return event.SessionSnapshot(session), true
	}

	_ = ws.ServeSessionEvents(req.Context(), w, req, sessionID, r.Hub, snapshot, r.CommandHandler)
}

func (r *Router) handlePostSessionMessage(w http.ResponseWriter, req *http.Request) {
	if !r.authorized(req) {
		writeJSONError(w, http.StatusUnauthorized, "missing or invalid token")
		return
	}
	if r.Store == nil {
		writeJSONError(w, http.StatusInternalServerError, "store is not configured")
		return
	}
	if r.CommandHandler == nil {
		writeJSONError(w, http.StatusServiceUnavailable, "command handler is not configured")
		return
	}

	sessionID := strings.TrimPrefix(req.URL.Path, "/api/v1/sessions/")
	sessionID = strings.TrimSuffix(sessionID, "/messages")
	sessionID = strings.Trim(sessionID, "/")
	if sessionID == "" {
		writeJSONError(w, http.StatusNotFound, "session not found")
		return
	}
	if _, ok := r.Store.GetSession(sessionID); !ok {
		writeJSONError(w, http.StatusNotFound, "session not found")
		return
	}

	var input SessionMessageRequest
	if err := json.NewDecoder(req.Body).Decode(&input); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid json body")
		return
	}
	content := strings.TrimSpace(input.Content)
	if content == "" {
		writeJSONError(w, http.StatusBadRequest, "content is required")
		return
	}

	cmd := event.Command{
		MsgType:   event.CommandUserMessage,
		MessageID: strings.TrimSpace(input.MessageID),
		Content:   content,
	}
	if err := r.CommandHandler(req.Context(), sessionID, cmd); err != nil {
		writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusAccepted, SessionMessageResponse{
		OK:        true,
		SessionID: sessionID,
		MessageID: cmd.MessageID,
	})
}

func (r *Router) authorized(req *http.Request) bool {
	if strings.TrimSpace(r.Token) == "" {
		return true
	}

	if got := strings.TrimSpace(req.Header.Get("X-AgenLeash-Token")); got != "" && got == r.Token {
		return true
	}
	if got := strings.TrimSpace(req.URL.Query().Get("token")); got != "" && got == r.Token {
		return true
	}
	if got := strings.TrimSpace(bearerToken(req.Header.Get("Authorization"))); got != "" && got == r.Token {
		return true
	}
	return false
}

func bearerToken(raw string) string {
	raw = strings.TrimSpace(raw)
	if !strings.HasPrefix(strings.ToLower(raw), "bearer ") {
		return ""
	}
	return strings.TrimSpace(raw[7:])
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func writeJSONError(w http.ResponseWriter, code int, msg string) {
	writeJSON(w, code, ErrorResponse{Error: msg})
}

func (r *Router) now() time.Time {
	if r.Now != nil {
		return r.Now()
	}
	return time.Now().UTC()
}

func (r *Router) newID(prefix string) string {
	if r.NewID != nil {
		return r.NewID(prefix)
	}
	return defaultID(prefix)
}

func defaultID(prefix string) string {
	var b [6]byte
	if _, err := rand.Read(b[:]); err != nil {
		return ""
	}
	return prefix + "_" + hex.EncodeToString(b[:])
}

func canonicalPath(raw string) (string, error) {
	path := strings.TrimSpace(raw)
	if path == "" {
		return "", errors.New("cwd is required")
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	return filepath.Clean(abs), nil
}

func workspaceFingerprint(cwd, adapter, versionHint, conversationID string) string {
	return strings.Join([]string{cwd, adapter, versionHint, conversationID}, "|")
}

func SessionSummaryFromModel(session model.Session, now time.Time) SessionSummary {
	return SessionSummary{
		ID:                session.ID,
		Adapter:           session.Adapter,
		Origin:            session.Origin,
		RuntimeMode:       session.RuntimeMode,
		State:             session.State,
		DetailPath:        session.DetailPath,
		CreatedAt:         session.CreatedAt,
		LastSeen:          session.LastSeen,
		Capabilities:      session.Capabilities,
		Features:          session.Features.Clone(),
		Conversation:      ConversationSummary{NativeID: session.NativeConversationID, StartMode: session.StartMode, ResumeStrategy: session.ResumeStrategy},
		Workspace:         WorkspaceSummary{CWD: session.WorkspacePath, Root: session.WorkspaceRoot, Fingerprint: session.WorkspaceFingerprint, GitRoot: session.GitRoot, GitBranch: session.GitBranch},
		Highlight:         model.HighlightForSession(session, now),
		LastOutputPreview: displayOutputPreview(session.LastOutputPreview),
	}
}

func displayOutputPreview(preview string) string {
	trimmed := strings.TrimSpace(preview)
	if isRuntimeDiagnosticPreview(trimmed) {
		return ""
	}
	return trimmed
}

func isRuntimeDiagnosticPreview(preview string) bool {
	if preview == "" {
		return false
	}
	lower := strings.ToLower(preview)
	if lower == "reading prompt from stdin..." ||
		lower == "reading additional input from stdin..." ||
		strings.HasPrefix(lower, "reading prompt from stdin...") ||
		strings.HasPrefix(lower, "reading additional input from stdin...") {
		return true
	}
	if strings.Contains(lower, "codex_core_skills::manager") ||
		strings.Contains(lower, "codex_models_manager::manager") {
		return true
	}
	return strings.HasPrefix(lower, "202") &&
		(strings.Contains(lower, " error codex_") || strings.Contains(lower, " warn codex_"))
}

func sessionPriority(session model.Session, now time.Time) (int, time.Time) {
	updatedAt := session.LastSeen
	if updatedAt.IsZero() {
		updatedAt = session.CreatedAt
	}

	highlight := model.HighlightForSession(session, now)
	if highlight == nil {
		return 99, updatedAt
	}
	if !highlight.UpdatedAt.IsZero() {
		updatedAt = highlight.UpdatedAt
	}
	return highlight.SortRank, updatedAt
}

type StatusError struct {
	Code int
	Err  error
}

func (e *StatusError) Error() string {
	if e == nil || e.Err == nil {
		return ""
	}
	return e.Err.Error()
}

func (e *StatusError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

func WithStatus(code int, format string, args ...any) error {
	return &StatusError{
		Code: code,
		Err:  fmt.Errorf(format, args...),
	}
}

func statusCode(err error) int {
	var statusErr *StatusError
	if errors.As(err, &statusErr) && statusErr.Code > 0 {
		return statusErr.Code
	}
	return http.StatusInternalServerError
}
