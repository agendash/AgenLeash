package app

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/agendash/AgenLeash/internal/adapter"
	"github.com/agendash/AgenLeash/internal/event"
	"github.com/agendash/AgenLeash/internal/history"
	"github.com/agendash/AgenLeash/internal/httpapi"
	"github.com/agendash/AgenLeash/internal/model"
	"github.com/agendash/AgenLeash/internal/runtime"
	"github.com/agendash/AgenLeash/internal/session"
	"github.com/agendash/AgenLeash/internal/store"
)

const (
	defaultRows = 24
	defaultCols = 120

	managedHistoryBackfillSkew   = 30 * time.Second
	managedHistoryBackfillWindow = 20 * time.Minute
)

var ansiRegexp = regexp.MustCompile(`\x1b\[[0-?]*[ -/]*[@-~]`)

type App struct {
	Config   Config
	Store    store.Store
	Hub      *event.Hub
	Sessions *session.Manager
	Router   *httpapi.Router
	Server   *http.Server

	adapters               map[string]adapter.AdapterSpec
	historyMu              sync.Mutex
	historyRefreshInFlight bool
	lastHistoryRefreshAt   time.Time
	persistMu              sync.Mutex
	lastSessionPersistAt   map[string]time.Time
}

func New(cfg Config) (*App, error) {
	if strings.TrimSpace(cfg.Token) == "" && !cfg.AllowNoToken {
		return nil, errors.New("AGENLEASH_TOKEN is required unless AGENLEASH_ALLOW_NO_TOKEN=true")
	}

	dataDir := cfg.DataDir
	if dataDir == "" {
		dataDir = filepath.Join("tmp", "agenleash")
	}
	cfg.DataDir = dataDir

	adapterDir := resolveAdapterDir(cfg.AdapterDir)
	cfg.AdapterDir = adapterDir

	repoSQLite, err := store.NewSQLiteStore(filepath.Join(dataDir, "state.sqlite"))
	if err != nil {
		return nil, err
	}
	if legacyRepo, err := store.NewFileStore(filepath.Join(dataDir, "state.json")); err == nil {
		legacySnapshot := legacyRepo.Snapshot()
		currentSnapshot := repoSQLite.Snapshot()
		if len(currentSnapshot.Sessions) == 0 && len(currentSnapshot.Workspaces) == 0 && len(currentSnapshot.Conversations) == 0 &&
			(len(legacySnapshot.Sessions) > 0 || len(legacySnapshot.Workspaces) > 0 || len(legacySnapshot.Conversations) > 0) {
			_ = repoSQLite.Replace(legacySnapshot)
		}
		_ = legacyRepo.Close()
	}

	specs, err := adapter.LoadDirectory(adapterDir)
	if err != nil {
		return nil, fmt.Errorf("load adapters from %s: %w", adapterDir, err)
	}

	app := &App{
		Config:               cfg,
		Store:                repoSQLite,
		Hub:                  event.NewHub(256),
		Sessions:             session.NewManager(),
		adapters:             make(map[string]adapter.AdapterSpec, len(specs)),
		lastSessionPersistAt: make(map[string]time.Time),
	}
	for _, spec := range specs {
		app.adapters[spec.Metadata.Name] = spec
	}

	router := httpapi.NewRouter(cfg.Token, repoSQLite, app.Hub)
	router.EnableWeb = cfg.EnableWeb
	router.CommandHandler = app.handleCommand
	router.StartSessionHandler = app.handleStartSession
	router.ListSessionsHandler = app.handleListSessions
	router.GetSessionHandler = app.handleGetSession
	router.GetFilePreviewHandler = app.handleGetFilePreview
	router.GetStatsHandler = app.handleGetStats
	app.Router = router
	app.Server = &http.Server{
		Addr:    cfg.Addr,
		Handler: router,
	}
	return app, nil
}

func (a *App) Run(ctx context.Context) error {
	if err := os.MkdirAll(a.Config.DataDir, 0o755); err != nil {
		return err
	}

	errCh := make(chan error, 1)
	go func() {
		err := a.Server.ListenAndServe()
		if errors.Is(err, http.ErrServerClosed) {
			err = nil
		}
		errCh <- err
	}()
	go func() {
		_ = a.refreshHistory(ctx, true)
	}()

	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = a.Server.Shutdown(shutdownCtx)
		if err := <-errCh; err != nil {
			_ = a.Store.Close()
			return err
		}
		return a.Store.Close()
	case err := <-errCh:
		_ = a.Store.Close()
		return err
	}
}

func (a *App) handleListSessions(req *http.Request) ([]model.Session, error) {
	if err := a.refreshHistory(req.Context(), false); err != nil {
		return nil, err
	}
	return a.Store.ListSessions(), nil
}

func (a *App) handleGetSession(req *http.Request, sessionID string) (httpapi.SessionDetailResponse, error) {
	if err := a.refreshHistory(req.Context(), false); err != nil {
		return httpapi.SessionDetailResponse{}, err
	}
	pageOpts, err := parseSessionMessagePageOptions(req)
	if err != nil {
		return httpapi.SessionDetailResponse{}, httpapi.WithStatus(http.StatusBadRequest, "%s", err.Error())
	}
	session, ok := a.Store.GetSession(sessionID)
	if !ok {
		return httpapi.SessionDetailResponse{}, httpapi.WithStatus(http.StatusNotFound, "session not found")
	}

	historySource := session
	if strings.TrimSpace(historySource.DetailPath) == "" {
		if discovered, ok := a.findHistorySource(session); ok {
			historySource.DetailPath = discovered.DetailPath
			if strings.TrimSpace(historySource.LastOutputPreview) == "" {
				historySource.LastOutputPreview = discovered.LastOutputPreview
			}
		} else if strings.TrimSpace(historySource.NativeConversationID) != "" {
			if err := a.refreshHistory(req.Context(), true); err == nil {
				if discovered, ok := a.findHistorySource(session); ok {
					historySource.DetailPath = discovered.DetailPath
					if strings.TrimSpace(historySource.LastOutputPreview) == "" {
						historySource.LastOutputPreview = discovered.LastOutputPreview
					}
				}
			}
		}
	}

	messagePage := history.LoadMessagePage(historySource, pageOpts)
	return httpapi.SessionDetailResponse{
		Session:  httpapi.SessionSummaryFromModel(historySource, time.Now().UTC()),
		Messages: messagePage.Messages,
		MessagesPage: &httpapi.MessagesPage{
			Limit:      messagePage.Limit,
			Offset:     messagePage.Offset,
			Returned:   messagePage.Returned,
			HasMore:    messagePage.HasMore,
			NextOffset: messagePage.NextOffset,
		},
	}, nil
}

const (
	filePreviewTextLimit  = 512 * 1024
	filePreviewImageLimit = 6 * 1024 * 1024
)

func (a *App) handleGetFilePreview(req *http.Request, sessionID string, requestedPath string) (httpapi.FilePreviewResponse, error) {
	if err := a.refreshHistory(req.Context(), false); err != nil {
		return httpapi.FilePreviewResponse{}, err
	}
	session, ok := a.Store.GetSession(sessionID)
	if !ok {
		return httpapi.FilePreviewResponse{}, httpapi.WithStatus(http.StatusNotFound, "session not found")
	}

	path, err := resolveSessionFilePreviewPath(requestedPath, session)
	if err != nil {
		return httpapi.FilePreviewResponse{}, httpapi.WithStatus(http.StatusBadRequest, "%s", err.Error())
	}
	temporaryImagePreview := false
	if !filePreviewPathAllowed(path, session, a.Config.AllowedWorkspaceRoots) {
		if !filePreviewTemporaryImagePathAllowed(path) {
			return httpapi.FilePreviewResponse{}, httpapi.WithStatus(http.StatusForbidden, "file is outside the session workspace")
		}
		temporaryImagePreview = true
	}

	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return httpapi.FilePreviewResponse{}, httpapi.WithStatus(http.StatusNotFound, "file not found")
		}
		return httpapi.FilePreviewResponse{}, err
	}
	if info.IsDir() {
		return httpapi.FilePreviewResponse{}, httpapi.WithStatus(http.StatusBadRequest, "path is a directory")
	}

	mimeType := mime.TypeByExtension(strings.ToLower(filepath.Ext(path)))
	limit := filePreviewTextLimit
	if strings.HasPrefix(mimeType, "image/") || looksLikeImagePath(path) {
		limit = filePreviewImageLimit
	}
	data, truncated, err := readFilePreviewBytes(path, limit)
	if err != nil {
		return httpapi.FilePreviewResponse{}, err
	}
	if temporaryImagePreview && !filePreviewDataLooksImage(path, data) {
		return httpapi.FilePreviewResponse{}, httpapi.WithStatus(http.StatusForbidden, "file is outside the session workspace")
	}
	if mimeType == "" && len(data) > 0 {
		mimeType = http.DetectContentType(data)
	}

	kind := filePreviewKind(path, mimeType, data)
	resp := httpapi.FilePreviewResponse{
		Path:      path,
		Name:      filepath.Base(path),
		MIMEType:  mimeType,
		Kind:      kind,
		Language:  languageForPreviewPath(path),
		Size:      info.Size(),
		Truncated: truncated,
	}
	switch kind {
	case "image":
		resp.Encoding = "base64"
		resp.Content = base64.StdEncoding.EncodeToString(data)
	case "text":
		resp.Encoding = "utf-8"
		if !utf8.Valid(data) {
			resp.Content = strings.ToValidUTF8(string(data), "\uFFFD")
		} else {
			resp.Content = string(data)
		}
	}
	return resp, nil
}

func (a *App) handleGetStats(req *http.Request) (httpapi.StatsResponse, error) {
	if err := a.refreshHistory(req.Context(), false); err != nil {
		return httpapi.StatsResponse{}, err
	}
	return httpapi.BuildStatsResponse(a.Store.ListSessions(), time.Now().UTC(), httpapi.ParseStatsTopN(req)), nil
}

func (a *App) findHistorySource(session model.Session) (model.Session, bool) {
	if strings.TrimSpace(session.NativeConversationID) == "" {
		return model.Session{}, false
	}

	for _, candidate := range a.Store.ListSessions() {
		if candidate.ID == session.ID {
			continue
		}
		if strings.TrimSpace(candidate.Adapter) != strings.TrimSpace(session.Adapter) {
			continue
		}
		if strings.TrimSpace(candidate.NativeConversationID) != strings.TrimSpace(session.NativeConversationID) {
			continue
		}
		if strings.TrimSpace(candidate.DetailPath) == "" {
			continue
		}
		if strings.TrimSpace(candidate.WorkspacePath) != "" &&
			strings.TrimSpace(session.WorkspacePath) != "" &&
			filepath.Clean(candidate.WorkspacePath) != filepath.Clean(session.WorkspacePath) {
			continue
		}
		return candidate, true
	}
	return model.Session{}, false
}

func (a *App) refreshHistory(ctx context.Context, force bool) error {
	a.historyMu.Lock()
	if a.historyRefreshInFlight {
		a.historyMu.Unlock()
		return nil
	}
	if !force && !a.lastHistoryRefreshAt.IsZero() &&
		a.Config.HistoryRefreshInterval > 0 &&
		time.Since(a.lastHistoryRefreshAt) < a.Config.HistoryRefreshInterval {
		a.historyMu.Unlock()
		return nil
	}
	a.historyRefreshInFlight = true
	a.historyMu.Unlock()

	defer func() {
		a.historyMu.Lock()
		a.historyRefreshInFlight = false
		a.historyMu.Unlock()
	}()

	supported := make(map[string]bool, len(a.adapters))
	for name := range a.adapters {
		supported[strings.ToLower(strings.TrimSpace(name))] = true
	}
	if supported["codex"] && a.Config.DiscoverCodex && strings.TrimSpace(a.Config.CodexHome) != "" {
		if err := a.reconcileHistoryRecords("codex", history.DiscoverCodexRecords(ctx, a.Config.CodexHome)); err != nil {
			return err
		}
	}
	if supported["opencode"] && a.Config.DiscoverOpencode && strings.TrimSpace(a.Config.OpencodeHome) != "" {
		if err := a.reconcileHistoryRecords("opencode", history.DiscoverOpencodeRecords(ctx, a.Config.OpencodeHome)); err != nil {
			return err
		}
	}
	if supported["claudecode"] && a.Config.DiscoverClaude && strings.TrimSpace(a.Config.ClaudeHome) != "" {
		if err := a.reconcileHistoryRecords("claudecode", history.DiscoverClaudeRecords(ctx, a.Config.ClaudeHome)); err != nil {
			return err
		}
	}
	a.historyMu.Lock()
	a.lastHistoryRefreshAt = time.Now().UTC()
	a.historyMu.Unlock()
	return nil
}

func (a *App) upsertHistoryRecords(records []history.Record) error {
	for _, record := range records {
		if err := a.Store.UpsertWorkspace(record.Workspace); err != nil {
			return err
		}
		if err := a.Store.UpsertConversation(record.Conversation); err != nil {
			return err
		}
		if err := a.Store.UpsertSession(record.Session); err != nil {
			return err
		}
	}
	return nil
}

func (a *App) reconcileHistoryRecords(adapterName string, records []history.Record) error {
	if err := a.upsertHistoryRecords(records); err != nil {
		return err
	}
	if err := a.backfillManagedHistoryRecords(adapterName, records); err != nil {
		return err
	}

	keepSessionIDs := make(map[string]struct{}, len(records))
	for _, record := range records {
		if id := strings.TrimSpace(record.Session.ID); id != "" {
			keepSessionIDs[id] = struct{}{}
		}
	}

	for _, session := range a.Store.ListSessions() {
		if session.Origin != model.SessionOriginDiscovered {
			continue
		}
		if !strings.EqualFold(strings.TrimSpace(session.Adapter), strings.TrimSpace(adapterName)) {
			continue
		}
		if _, ok := keepSessionIDs[strings.TrimSpace(session.ID)]; ok {
			continue
		}
		if err := a.Store.DeleteSession(session.ID); err != nil {
			return err
		}
	}

	return a.pruneOrphanHistoryEntries()
}

func (a *App) backfillManagedHistoryRecords(adapterName string, records []history.Record) error {
	normalizedAdapter := strings.ToLower(strings.TrimSpace(adapterName))
	if normalizedAdapter != "codex" && normalizedAdapter != "opencode" {
		return nil
	}

	snapshot := a.Store.Snapshot()
	if err := a.backfillManagedHistoryByNativeID(snapshot, adapterName, records); err != nil {
		return err
	}
	snapshot = a.Store.Snapshot()

	usedNativeIDs := make(map[string]struct{})
	for _, existing := range snapshot.Sessions {
		if existing.Origin != model.SessionOriginManaged {
			continue
		}
		if !strings.EqualFold(strings.TrimSpace(existing.Adapter), strings.TrimSpace(adapterName)) {
			continue
		}
		nativeID := strings.TrimSpace(existing.NativeConversationID)
		if nativeID == "" {
			continue
		}
		usedNativeIDs[nativeID] = struct{}{}
	}

	candidatesByWorkspace := make(map[string][]history.Record)
	for _, record := range records {
		candidate := record.Session
		if candidate.Origin != model.SessionOriginDiscovered {
			continue
		}
		if !strings.EqualFold(strings.TrimSpace(candidate.Adapter), strings.TrimSpace(adapterName)) {
			continue
		}
		workspacePath := normalizedWorkspacePath(candidate.WorkspacePath)
		nativeID := strings.TrimSpace(candidate.NativeConversationID)
		if workspacePath == "" || nativeID == "" || strings.TrimSpace(candidate.DetailPath) == "" || candidate.CreatedAt.IsZero() {
			continue
		}
		if _, inUse := usedNativeIDs[nativeID]; inUse {
			continue
		}
		candidatesByWorkspace[workspacePath] = append(candidatesByWorkspace[workspacePath], record)
	}
	for workspacePath := range candidatesByWorkspace {
		sort.SliceStable(candidatesByWorkspace[workspacePath], func(i, j int) bool {
			left := candidatesByWorkspace[workspacePath][i].Session
			right := candidatesByWorkspace[workspacePath][j].Session
			if !left.CreatedAt.Equal(right.CreatedAt) {
				return left.CreatedAt.Before(right.CreatedAt)
			}
			if !left.LastSeen.Equal(right.LastSeen) {
				return left.LastSeen.Before(right.LastSeen)
			}
			return left.ID < right.ID
		})
	}

	managedByWorkspace := make(map[string][]model.Session)
	for _, existing := range snapshot.Sessions {
		if existing.Origin != model.SessionOriginManaged {
			continue
		}
		if !strings.EqualFold(strings.TrimSpace(existing.Adapter), strings.TrimSpace(adapterName)) {
			continue
		}
		if strings.TrimSpace(existing.NativeConversationID) != "" {
			continue
		}
		if existing.CreatedAt.IsZero() {
			continue
		}
		if liveSession, live := a.Sessions.Get(existing.ID); live {
			switch liveSession.Snapshot().State {
			case model.SessionStatePending, model.SessionStateRunning, model.SessionStatePaused:
				continue
			}
		}
		workspacePath := normalizedWorkspacePath(existing.WorkspacePath)
		if workspacePath == "" {
			continue
		}
		managedByWorkspace[workspacePath] = append(managedByWorkspace[workspacePath], existing)
	}
	for workspacePath := range managedByWorkspace {
		sort.SliceStable(managedByWorkspace[workspacePath], func(i, j int) bool {
			left := managedByWorkspace[workspacePath][i]
			right := managedByWorkspace[workspacePath][j]
			if !left.CreatedAt.Equal(right.CreatedAt) {
				return left.CreatedAt.Before(right.CreatedAt)
			}
			if !left.LastSeen.Equal(right.LastSeen) {
				return left.LastSeen.Before(right.LastSeen)
			}
			return left.ID < right.ID
		})
	}

	usedCandidateIDs := make(map[string]struct{})
	for workspacePath, managedSessions := range managedByWorkspace {
		candidates := candidatesByWorkspace[workspacePath]
		if len(candidates) == 0 {
			continue
		}
		candidateIndex := 0
		for _, managed := range managedSessions {
			matchIndex := -1
			for idx := candidateIndex; idx < len(candidates); idx++ {
				candidate := candidates[idx].Session
				if _, consumed := usedCandidateIDs[candidate.ID]; consumed {
					continue
				}
				if !canBackfillManagedHistory(managed, candidate) {
					if candidate.CreatedAt.After(managed.CreatedAt.Add(managedHistoryBackfillWindow)) {
						break
					}
					continue
				}
				matchIndex = idx
				break
			}
			if matchIndex < 0 {
				continue
			}

			record := candidates[matchIndex]
			candidate := record.Session
			updated := managed
			updated.NativeConversationID = candidate.NativeConversationID
			updated.ResumeStrategy = model.ResumeStrategyNativeID
			updated.DetailPath = candidate.DetailPath
			if strings.TrimSpace(candidate.LastOutputPreview) != "" {
				updated.LastOutputPreview = candidate.LastOutputPreview
			}
			updated.Capabilities.SupportsResume = updated.Capabilities.SupportsResume || candidate.Capabilities.SupportsResume
			updated.Capabilities.SupportsNativeConversation = true
			updated.State = candidate.State
			if candidate.LastSeen.After(updated.LastSeen) {
				updated.LastSeen = candidate.LastSeen
			}
			if err := a.Store.UpsertSession(updated); err != nil {
				return err
			}
			if err := a.upsertManagedConversationHistoryBinding(snapshot, updated, record.Conversation); err != nil {
				return err
			}

			usedNativeIDs[updated.NativeConversationID] = struct{}{}
			usedCandidateIDs[candidate.ID] = struct{}{}
			candidateIndex = matchIndex + 1
		}
	}

	return nil
}

func (a *App) backfillManagedHistoryByNativeID(snapshot store.Catalog, adapterName string, records []history.Record) error {
	candidatesByNativeID := make(map[string]history.Record)
	for _, record := range records {
		candidate := record.Session
		if candidate.Origin != model.SessionOriginDiscovered {
			continue
		}
		if !strings.EqualFold(strings.TrimSpace(candidate.Adapter), strings.TrimSpace(adapterName)) {
			continue
		}
		nativeID := strings.TrimSpace(candidate.NativeConversationID)
		if nativeID == "" || strings.TrimSpace(candidate.DetailPath) == "" {
			continue
		}

		current, ok := candidatesByNativeID[nativeID]
		if !ok || candidate.LastSeen.After(current.Session.LastSeen) {
			candidatesByNativeID[nativeID] = record
		}
	}

	for _, existing := range snapshot.Sessions {
		if existing.Origin != model.SessionOriginManaged {
			continue
		}
		if !strings.EqualFold(strings.TrimSpace(existing.Adapter), strings.TrimSpace(adapterName)) {
			continue
		}
		nativeID := strings.TrimSpace(existing.NativeConversationID)
		if nativeID == "" {
			continue
		}
		if liveSession, live := a.Sessions.Get(existing.ID); live {
			switch liveSession.Snapshot().State {
			case model.SessionStatePending, model.SessionStateRunning, model.SessionStatePaused:
				continue
			}
		}

		record, ok := candidatesByNativeID[nativeID]
		if !ok {
			continue
		}
		candidate := record.Session
		if strings.TrimSpace(candidate.WorkspacePath) != "" &&
			strings.TrimSpace(existing.WorkspacePath) != "" &&
			filepath.Clean(candidate.WorkspacePath) != filepath.Clean(existing.WorkspacePath) {
			continue
		}

		updated := existing
		changed := false
		if updated.ResumeStrategy != model.ResumeStrategyNativeID {
			updated.ResumeStrategy = model.ResumeStrategyNativeID
			changed = true
		}
		if updated.DetailPath != candidate.DetailPath {
			updated.DetailPath = candidate.DetailPath
			changed = true
		}
		if strings.TrimSpace(candidate.LastOutputPreview) != "" && updated.LastOutputPreview != candidate.LastOutputPreview {
			updated.LastOutputPreview = candidate.LastOutputPreview
			changed = true
		}
		if candidate.LastSeen.After(updated.LastSeen) {
			updated.LastSeen = candidate.LastSeen
			changed = true
		}
		if updated.State != candidate.State {
			updated.State = candidate.State
			changed = true
		}
		if updated.Capabilities.SupportsResume != (updated.Capabilities.SupportsResume || candidate.Capabilities.SupportsResume) {
			updated.Capabilities.SupportsResume = updated.Capabilities.SupportsResume || candidate.Capabilities.SupportsResume
			changed = true
		}
		if updated.Capabilities.SupportsNativeConversation != (updated.Capabilities.SupportsNativeConversation || candidate.Capabilities.SupportsNativeConversation) {
			updated.Capabilities.SupportsNativeConversation = updated.Capabilities.SupportsNativeConversation || candidate.Capabilities.SupportsNativeConversation
			changed = true
		}
		if !changed {
			continue
		}

		if err := a.Store.UpsertSession(updated); err != nil {
			return err
		}
		if err := a.upsertManagedConversationHistoryBinding(snapshot, updated, record.Conversation); err != nil {
			return err
		}
	}

	return nil
}

func canBackfillManagedHistory(managed model.Session, candidate model.Session) bool {
	if managed.CreatedAt.IsZero() || candidate.CreatedAt.IsZero() {
		return false
	}
	if strings.TrimSpace(candidate.NativeConversationID) == "" || strings.TrimSpace(candidate.DetailPath) == "" {
		return false
	}
	if candidate.CreatedAt.Before(managed.CreatedAt.Add(-managedHistoryBackfillSkew)) {
		return false
	}
	if candidate.CreatedAt.After(managed.CreatedAt.Add(managedHistoryBackfillWindow)) {
		return false
	}
	return true
}

func (a *App) upsertManagedConversationHistoryBinding(snapshot store.Catalog, session model.Session, discovered model.Conversation) error {
	if strings.TrimSpace(session.ConversationID) == "" {
		return nil
	}

	conversation, ok := snapshot.Conversations[session.ConversationID]
	if !ok {
		conversation = model.Conversation{
			ID:          session.ConversationID,
			SessionID:   session.ID,
			WorkspaceID: session.WorkspaceID,
			CreatedAt:   session.CreatedAt,
		}
	}
	conversation.NativeID = session.NativeConversationID
	if conversation.WorkspaceID == "" {
		conversation.WorkspaceID = session.WorkspaceID
	}
	if conversation.CreatedAt.IsZero() {
		conversation.CreatedAt = session.CreatedAt
	}
	if conversation.CreatedAt.IsZero() {
		conversation.CreatedAt = discovered.CreatedAt
	}
	conversation.UpdatedAt = time.Now().UTC()
	return a.Store.UpsertConversation(conversation)
}

func normalizedWorkspacePath(path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return ""
	}
	return filepath.Clean(path)
}

func (a *App) pruneOrphanHistoryEntries() error {
	snapshot := a.Store.Snapshot()

	referencedConversations := make(map[string]struct{}, len(snapshot.Sessions))
	referencedWorkspaces := make(map[string]struct{}, len(snapshot.Sessions))
	for _, session := range snapshot.Sessions {
		if id := strings.TrimSpace(session.ConversationID); id != "" {
			referencedConversations[id] = struct{}{}
		}
		if id := strings.TrimSpace(session.WorkspaceID); id != "" {
			referencedWorkspaces[id] = struct{}{}
		}
	}

	for id := range snapshot.Conversations {
		if _, ok := referencedConversations[id]; ok {
			continue
		}
		if err := a.Store.DeleteConversation(id); err != nil {
			return err
		}
	}

	for id := range snapshot.Workspaces {
		if _, ok := referencedWorkspaces[id]; ok {
			continue
		}
		if err := a.Store.DeleteWorkspace(id); err != nil {
			return err
		}
	}

	return nil
}

func parseSessionMessagePageOptions(req *http.Request) (history.MessagePageOptions, error) {
	query := req.URL.Query()

	limit, err := parseNonNegativeIntQuery(query.Get("limit"))
	if err != nil {
		return history.MessagePageOptions{}, fmt.Errorf("invalid limit: %w", err)
	}
	offset, err := parseNonNegativeIntQuery(query.Get("offset"))
	if err != nil {
		return history.MessagePageOptions{}, fmt.Errorf("invalid offset: %w", err)
	}

	return history.MessagePageOptions{
		Limit:  limit,
		Offset: offset,
	}, nil
}

func parseNonNegativeIntQuery(raw string) (int, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0, nil
	}
	value, err := strconv.Atoi(raw)
	if err != nil {
		return 0, err
	}
	if value < 0 {
		return 0, fmt.Errorf("must be >= 0")
	}
	return value, nil
}

func (a *App) handleStartSession(req *http.Request, in httpapi.StartSessionRequest) (model.Session, error) {
	spec, ok := a.adapters[strings.TrimSpace(in.Adapter)]
	if !ok {
		return model.Session{}, httpapi.WithStatus(http.StatusBadRequest, "unknown adapter %q", in.Adapter)
	}

	startMode := model.StartMode(strings.ToLower(strings.TrimSpace(in.StartMode)))
	switch startMode {
	case "":
		startMode = model.StartModeNew
	case model.StartModeNew, model.StartModeResume:
	default:
		return model.Session{}, httpapi.WithStatus(http.StatusBadRequest, "unsupported start_mode %q", in.StartMode)
	}

	cwd, err := filepath.Abs(strings.TrimSpace(in.CWD))
	if err != nil {
		return model.Session{}, httpapi.WithStatus(http.StatusBadRequest, "resolve cwd: %v", err)
	}
	cwd = filepath.Clean(cwd)
	info, err := os.Stat(cwd)
	if err != nil || !info.IsDir() {
		return model.Session{}, httpapi.WithStatus(http.StatusBadRequest, "cwd does not exist: %s", cwd)
	}
	if !pathWithinAllowedRoots(cwd, a.Config.AllowedWorkspaceRoots) {
		return model.Session{}, httpapi.WithStatus(
			http.StatusBadRequest,
			"cwd %q is outside AGENLEASH_ALLOWED_WORKSPACE_ROOTS (%s)",
			cwd,
			strings.Join(a.Config.AllowedWorkspaceRoots, ", "),
		)
	}

	versionHint := strings.TrimSpace(in.AgentVersionHint)
	if versionHint == "" {
		versionHint = detectAdapterVersion(req.Context(), spec)
	}

	effective, err := adapter.Resolve(spec, versionHint)
	if err != nil {
		return model.Session{}, httpapi.WithStatus(http.StatusBadRequest, "resolve adapter %q: %v", spec.Metadata.Name, err)
	}
	if startMode == model.StartModeResume && !effective.Capabilities.SupportsResume {
		return model.Session{}, httpapi.WithStatus(http.StatusBadRequest, "adapter %q does not support resume for agent version %q", spec.Metadata.Name, versionHint)
	}

	entrypoint := resolveCommandBinary(stringValue(effective.Runtime.Entrypoint), adapterBinaryCandidates(spec))
	if entrypoint == "" {
		return model.Session{}, httpapi.WithStatus(http.StatusBadRequest, "adapter %q runtime entrypoint is empty", spec.Metadata.Name)
	}
	if _, err := exec.LookPath(entrypoint); err != nil {
		return model.Session{}, httpapi.WithStatus(http.StatusBadRequest, "adapter %q runtime entrypoint %q not found in PATH", spec.Metadata.Name, entrypoint)
	}

	now := time.Now().UTC()
	sessionID := newID("sess")
	conversationID := strings.TrimSpace(in.ConversationID)
	if conversationID == "" {
		conversationID = newID("conv")
	}
	workspaceID := newID("ws")

	gitRoot, gitBranch := detectGitContext(req.Context(), cwd)
	workspaceRoot := firstNonEmpty(gitRoot, cwd)
	resumeStrategy := model.ResumeStrategyProcessOnly
	nativeConversationID := ""
	if startMode == model.StartModeResume && effective.Capabilities.SupportsNativeConversation {
		nativeConversationID = conversationID
		resumeStrategy = model.ResumeStrategyNativeID
	}

	runtimeSpec := runtime.Spec{
		Mode:    strings.TrimSpace(stringValue(effective.Runtime.Mode)),
		Command: entrypoint,
		Args:    buildRuntimeArgs(spec, effective, startMode, conversationID, in.Args),
		Dir:     cwd,
		Env: mergeEnv(os.Environ(), effective.Runtime.Env, map[string]string{
			"AGENLEASH_SESSION_ID":      sessionID,
			"AGENLEASH_CONVERSATION_ID": conversationID,
			"AGENLEASH_WORKSPACE_ID":    workspaceID,
			"AGENLEASH_ADAPTER":         spec.Metadata.Name,
			"AGENLEASH_AGENT_VERSION":   versionHint,
		}),
		Rows: defaultRows,
		Cols: defaultCols,
	}

	// Detach the agent runtime from the HTTP request lifetime so sessions
	// continue running after the start request returns or the client disconnects.
	sess, err := a.Sessions.Start(context.Background(), session.StartRequest{
		ID:                   sessionID,
		Adapter:              spec.Metadata.Name,
		Origin:               model.SessionOriginManaged,
		RuntimeMode:          firstNonEmpty(runtimeSpec.Mode, "pty"),
		NativeConversationID: nativeConversationID,
		ConversationID:       conversationID,
		WorkspaceID:          workspaceID,
		StartMode:            startMode,
		ResumeStrategy:       resumeStrategy,
		WorkspacePath:        cwd,
		WorkspaceRoot:        workspaceRoot,
		WorkspaceFingerprint: workspaceFingerprintFor(cwd, spec.Metadata.Name, versionHint, conversationID),
		GitRoot:              gitRoot,
		GitBranch:            gitBranch,
		Capabilities:         effective.Capabilities,
		Features:             effective.Features,
		Runtime:              runtimeSpec,
	})
	if err != nil {
		return model.Session{}, err
	}

	snapshot := sess.Snapshot()
	if err := a.Store.UpsertWorkspace(model.Workspace{
		ID:              workspaceID,
		Path:            cwd,
		Root:            workspaceRoot,
		Fingerprint:     snapshot.WorkspaceFingerprint,
		GitRoot:         gitRoot,
		GitBranch:       gitBranch,
		ActiveSessionID: snapshot.ID,
		CreatedAt:       now,
		UpdatedAt:       now,
	}); err != nil {
		return model.Session{}, err
	}

	if err := a.Store.UpsertConversation(model.Conversation{
		ID:          conversationID,
		SessionID:   snapshot.ID,
		NativeID:    nativeConversationID,
		WorkspaceID: workspaceID,
		CreatedAt:   now,
		UpdatedAt:   now,
	}); err != nil {
		return model.Session{}, err
	}

	if err := a.Store.UpsertSession(snapshot); err != nil {
		return model.Session{}, err
	}

	a.Hub.Publish(snapshot.ID, event.SessionSnapshot(snapshot))
	a.Hub.Publish(snapshot.ID, event.StateChanged(snapshot.ID, snapshot.State))
	a.Hub.Publish(snapshot.ID, event.WorkspaceUpdated(snapshot))
	if nativeConversationID != "" {
		a.Hub.Publish(snapshot.ID, event.ConversationBound(snapshot))
	}

	go a.pumpSession(sess, effective)
	return snapshot, nil
}

func (a *App) pumpSession(sess *session.Session, spec adapter.EffectiveSpec) {
	sessionID := sess.ID()
	extractors := compileConversationExtractors(spec.Conversation)
	messageID := ""
	codexJSONRemainder := ""
	opencodeJSONRemainder := ""

	for evt := range sess.Events() {
		snapshot := sess.Snapshot()

		switch evt.Kind {
		case runtime.EventKindOutput:
			if len(evt.Data) == 0 {
				_ = a.persistSessionSnapshot(snapshot, false)
				continue
			}

			a.Hub.Publish(sessionID, event.RawChunk(sessionID, rawSource(snapshot), evt.Data))
			if isCodexExecJSON(spec) {
				var consumed bool
				codexJSONRemainder, consumed = a.consumeCodexJSONOutput(
					sess,
					sessionID,
					evt.Data,
					codexJSONRemainder,
				)
				snapshot = sess.Snapshot()
				_ = a.persistSessionSnapshot(snapshot, false)
				if consumed {
					continue
				}
			}
			if isOpencodeRunJSON(spec) {
				var consumed bool
				opencodeJSONRemainder, consumed = a.consumeOpencodeJSONOutput(
					sess,
					sessionID,
					evt.Data,
					opencodeJSONRemainder,
				)
				snapshot = sess.Snapshot()
				_ = a.persistSessionSnapshot(snapshot, false)
				if consumed {
					continue
				}
			}
			if suppressUnstructuredRuntimeOutput(spec) {
				_ = a.persistSessionSnapshot(snapshot, false)
				continue
			}

			text := sanitizeOutputText(evt.Data)
			if preview := outputPreview(text); preview != "" {
				if sess.SetLastOutputPreview(preview) {
					snapshot = sess.Snapshot()
				}
			}
			_ = a.persistSessionSnapshot(snapshot, false)
			if text == "" {
				continue
			}

			if snapshot.State == model.SessionStatePaused && !looksLikeInputRequest(evt.Data, text) {
				if sess.Resume("runtime resumed output") {
					snapshot = sess.Snapshot()
					_ = a.persistSessionSnapshot(snapshot, true)
					a.Hub.Publish(sessionID, event.SessionSnapshot(snapshot))
					a.Hub.Publish(sessionID, event.StateChanged(sessionID, snapshot.State))
				}
			}

			if nativeID := detectConversationID(text, extractors); nativeID != "" {
				if sess.BindConversation(nativeID, model.ResumeStrategyNativeID) {
					snapshot = sess.Snapshot()
					_ = a.persistSessionSnapshot(snapshot, true)
					a.upsertConversationBinding(snapshot, nativeID)
					a.Hub.Publish(sessionID, event.ConversationBound(snapshot))
				}
			}

			if messageID == "" {
				messageID = newID("msg")
				a.Hub.Publish(sessionID, event.MessageStarted(sessionID, messageID, "assistant"))
			}
			a.Hub.Publish(sessionID, event.MessageDelta(sessionID, messageID, "assistant", text))

			if looksLikeInputRequest(evt.Data, text) {
				if sess.Pause("awaiting user input") {
					snapshot = sess.Snapshot()
					_ = a.persistSessionSnapshot(snapshot, true)
					a.Hub.Publish(sessionID, event.SessionSnapshot(snapshot))
					a.Hub.Publish(sessionID, event.StateChanged(sessionID, snapshot.State))
				}
				a.Hub.Publish(sessionID, event.InputRequested(sessionID, "tty_wait"))
				a.Hub.Publish(sessionID, event.MessageCompleted(sessionID, messageID, "assistant"))
				messageID = ""
			}
		case runtime.EventKindExited:
			if isCodexExecJSON(spec) && strings.TrimSpace(codexJSONRemainder) != "" {
				codexJSONRemainder, _ = a.consumeCodexJSONOutput(
					sess,
					sessionID,
					[]byte("\n"),
					codexJSONRemainder,
				)
				snapshot = sess.Snapshot()
			}
			if isOpencodeRunJSON(spec) && strings.TrimSpace(opencodeJSONRemainder) != "" {
				opencodeJSONRemainder, _ = a.consumeOpencodeJSONOutput(
					sess,
					sessionID,
					[]byte("\n"),
					opencodeJSONRemainder,
				)
				snapshot = sess.Snapshot()
			}
			_ = a.persistSessionSnapshot(snapshot, true)
			if messageID != "" {
				a.Hub.Publish(sessionID, event.MessageCompleted(sessionID, messageID, "assistant"))
				messageID = ""
			}
			a.Hub.Publish(sessionID, event.StateChanged(sessionID, snapshot.State))
		case runtime.EventKindStateChanged, runtime.EventKindStopped, runtime.EventKindInterrupted:
			_ = a.persistSessionSnapshot(snapshot, true)
			a.Hub.Publish(sessionID, event.StateChanged(sessionID, snapshot.State))
		default:
			_ = a.persistSessionSnapshot(snapshot, false)
		}
	}
}

func (a *App) persistSessionSnapshot(snapshot model.Session, force bool) error {
	if snapshot.ID == "" {
		return nil
	}

	if !force {
		a.persistMu.Lock()
		if a.lastSessionPersistAt == nil {
			a.lastSessionPersistAt = make(map[string]time.Time)
		}
		last := a.lastSessionPersistAt[snapshot.ID]
		if !last.IsZero() && a.Config.SessionPersistInterval > 0 &&
			time.Since(last) < a.Config.SessionPersistInterval {
			a.persistMu.Unlock()
			return nil
		}
		a.lastSessionPersistAt[snapshot.ID] = time.Now().UTC()
		a.persistMu.Unlock()
	} else {
		a.persistMu.Lock()
		if a.lastSessionPersistAt == nil {
			a.lastSessionPersistAt = make(map[string]time.Time)
		}
		a.lastSessionPersistAt[snapshot.ID] = time.Now().UTC()
		a.persistMu.Unlock()
	}

	return a.Store.UpsertSession(snapshot)
}

func (a *App) upsertConversationBinding(snapshot model.Session, nativeID string) {
	if snapshot.ConversationID == "" {
		return
	}

	catalog := a.Store.Snapshot()
	conversation, ok := catalog.Conversations[snapshot.ConversationID]
	if !ok {
		conversation = model.Conversation{
			ID:          snapshot.ConversationID,
			SessionID:   snapshot.ID,
			WorkspaceID: snapshot.WorkspaceID,
			CreatedAt:   snapshot.CreatedAt,
		}
	}
	conversation.NativeID = nativeID
	conversation.UpdatedAt = time.Now().UTC()
	_ = a.Store.UpsertConversation(conversation)
}

func (a *App) handleCommand(ctx context.Context, sessionID string, cmd event.Command) error {
	switch cmd.MsgType {
	case event.CommandUserMessage:
		content := strings.TrimSpace(cmd.Content)
		if content == "" {
			return nil
		}
		messageID := strings.TrimSpace(cmd.MessageID)
		if messageID == "" {
			messageID = newID("user")
		}
		a.Hub.Publish(sessionID, event.MessageStarted(sessionID, messageID, "user"))
		a.Hub.Publish(sessionID, event.MessageDelta(sessionID, messageID, "user", content))
		a.Hub.Publish(sessionID, event.MessageCompleted(sessionID, messageID, "user"))

		payload := []byte(content)
		payload = append(payload, sessionSubmitByte(a.Sessions, sessionID))
		if sess, ok := a.Sessions.Get(sessionID); ok {
			if sess.Resume("user input received") {
				snapshot := sess.Snapshot()
				_ = a.Store.UpsertSession(snapshot)
				a.Hub.Publish(sessionID, event.SessionSnapshot(snapshot))
				a.Hub.Publish(sessionID, event.StateChanged(sessionID, snapshot.State))
			}
		}
		if _, err := a.Sessions.Write(sessionID, payload); err != nil {
			return err
		}
		if a.shouldCloseInputAfterMessage(sessionID) {
			return a.Sessions.CloseInput(sessionID)
		}
		return nil
	case event.CommandInterrupt:
		return a.Sessions.Interrupt(sessionID)
	case event.CommandRuntimeResize:
		rows := cmd.Height
		cols := cmd.Width
		if rows <= 0 {
			rows = defaultRows
		}
		if cols <= 0 {
			cols = defaultCols
		}
		return a.Sessions.Resize(sessionID, uint16(rows), uint16(cols))
	case event.CommandHeartbeat, event.CommandRequestRawStream:
		return nil
	default:
		return httpapi.WithStatus(http.StatusBadRequest, "unsupported command %q", cmd.MsgType)
	}
}

func sessionSubmitByte(manager *session.Manager, sessionID string) byte {
	if manager != nil {
		if sess, ok := manager.Get(sessionID); ok {
			if sess.Snapshot().Capabilities.RequiresTTY {
				return '\r'
			}
		}
	}
	return '\n'
}

func detectAdapterVersion(ctx context.Context, spec adapter.AdapterSpec) string {
	strategy := spec.Spec.Detection.VersionStrategy
	if strategy.Type != "command" || len(strategy.Command) == 0 {
		return ""
	}

	timeoutCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	command := append([]string(nil), strategy.Command...)
	command[0] = resolveCommandBinary(command[0], adapterBinaryCandidates(spec))
	cmd := exec.CommandContext(timeoutCtx, command[0], command[1:]...)
	output, err := cmd.CombinedOutput()
	if err != nil && len(output) == 0 {
		return ""
	}

	pattern := strings.TrimSpace(strategy.Regex)
	if pattern == "" {
		return strings.TrimSpace(string(output))
	}

	re, err := regexp.Compile(pattern)
	if err != nil {
		return ""
	}
	match := re.FindStringSubmatch(string(output))
	if len(match) == 0 {
		return ""
	}
	if idx := re.SubexpIndex("version"); idx > 0 && idx < len(match) {
		return strings.TrimSpace(match[idx])
	}
	if len(match) > 1 {
		return strings.TrimSpace(match[1])
	}
	return strings.TrimSpace(match[0])
}

func detectGitContext(ctx context.Context, cwd string) (string, string) {
	timeoutCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()

	run := func(args ...string) string {
		cmd := exec.CommandContext(timeoutCtx, "git", args...)
		cmd.Dir = cwd
		out, err := cmd.Output()
		if err != nil {
			return ""
		}
		return strings.TrimSpace(string(out))
	}

	root := run("rev-parse", "--show-toplevel")
	branch := run("rev-parse", "--abbrev-ref", "HEAD")
	return root, branch
}

func compileConversationExtractors(spec adapter.ConversationSpec) []*regexp.Regexp {
	var compiled []*regexp.Regexp
	for _, extractor := range spec.Extractors {
		if extractor.Type != "regex" || strings.TrimSpace(extractor.Pattern) == "" {
			continue
		}
		re, err := regexp.Compile(extractor.Pattern)
		if err != nil {
			continue
		}
		compiled = append(compiled, re)
	}
	return compiled
}

func adapterBinaryCandidates(spec adapter.AdapterSpec) []string {
	candidates := make([]string, 0, len(spec.Spec.Detection.BinaryNames)+2)
	candidates = append(candidates, spec.Spec.Detection.BinaryNames...)
	if entrypoint := stringValue(spec.Spec.Runtime.Entrypoint); entrypoint != "" {
		candidates = append(candidates, entrypoint)
	}
	if command := spec.Spec.Detection.VersionStrategy.Command; len(command) > 0 {
		candidates = append(candidates, command[0])
	}
	return candidates
}

func resolveCommandBinary(preferred string, candidates []string) string {
	seen := make(map[string]struct{}, len(candidates)+1)
	for _, candidate := range append([]string{preferred}, candidates...) {
		candidate = strings.TrimSpace(candidate)
		if candidate == "" {
			continue
		}
		if _, ok := seen[candidate]; ok {
			continue
		}
		seen[candidate] = struct{}{}
		if path, err := exec.LookPath(candidate); err == nil {
			return path
		}
	}
	return strings.TrimSpace(preferred)
}

func buildRuntimeArgs(spec adapter.AdapterSpec, effective adapter.EffectiveSpec, startMode model.StartMode, conversationID string, inputArgs []string) []string {
	baseArgs := append([]string(nil), effective.Runtime.Args...)
	switch spec.Spec.AgentFamily {
	case "claudecode":
		baseArgs = stripClaudeResumeArgs(baseArgs)
		if startMode == model.StartModeResume && strings.TrimSpace(conversationID) != "" {
			baseArgs = append(baseArgs, "--resume", strings.TrimSpace(conversationID))
		}
	case "codex":
		baseArgs = buildCodexRuntimeArgs(baseArgs, startMode, conversationID)
	case "opencode":
		baseArgs = buildOpencodeRuntimeArgs(baseArgs, startMode, conversationID)
	}
	return mergeArgs(baseArgs, inputArgs)
}

func buildCodexRuntimeArgs(baseArgs []string, startMode model.StartMode, conversationID string) []string {
	trimmedID := strings.TrimSpace(conversationID)
	if len(baseArgs) == 0 {
		if startMode == model.StartModeResume && trimmedID != "" {
			return []string{"exec", "resume", trimmedID}
		}
		return []string{"exec"}
	}

	leading := strings.TrimSpace(baseArgs[0])
	if leading != "exec" {
		return baseArgs
	}

	tail := append([]string(nil), baseArgs[1:]...)
	out := []string{"exec"}
	if startMode == model.StartModeResume && trimmedID != "" {
		out = append(out, "resume")
		out = append(out, tail...)
		out = append(out, trimmedID)
		return out
	}
	out = append(out, tail...)
	return out
}

func buildOpencodeRuntimeArgs(baseArgs []string, startMode model.StartMode, conversationID string) []string {
	trimmedID := strings.TrimSpace(conversationID)
	if len(baseArgs) == 0 {
		if startMode == model.StartModeResume && trimmedID != "" {
			return []string{"run", "--session", trimmedID}
		}
		return []string{"run"}
	}

	leading := strings.TrimSpace(baseArgs[0])
	if leading != "run" {
		return baseArgs
	}

	tail := append([]string(nil), baseArgs[1:]...)
	out := []string{"run"}
	if startMode == model.StartModeResume && trimmedID != "" {
		out = append(out, "--session", trimmedID)
	}
	out = append(out, tail...)
	return out
}

func stripClaudeResumeArgs(args []string) []string {
	out := make([]string, 0, len(args))
	skipValue := false
	for _, arg := range args {
		if skipValue {
			skipValue = false
			continue
		}
		switch strings.TrimSpace(arg) {
		case "-r", "--resume":
			skipValue = true
			continue
		}
		out = append(out, arg)
	}
	return out
}

func detectConversationID(text string, extractors []*regexp.Regexp) string {
	for _, re := range extractors {
		match := re.FindStringSubmatch(text)
		if len(match) == 0 {
			continue
		}
		if idx := re.SubexpIndex("id"); idx > 0 && idx < len(match) {
			return strings.TrimSpace(match[idx])
		}
		if len(match) > 1 {
			return strings.TrimSpace(match[1])
		}
	}
	return ""
}

func (a *App) shouldCloseInputAfterMessage(sessionID string) bool {
	if a.Sessions == nil {
		return false
	}
	sess, ok := a.Sessions.Get(sessionID)
	if !ok {
		return false
	}
	snapshot := sess.Snapshot()
	adapterName := strings.ToLower(strings.TrimSpace(snapshot.Adapter))
	return (adapterName == "codex" || adapterName == "opencode") &&
		strings.EqualFold(strings.TrimSpace(snapshot.RuntimeMode), "stdio")
}

func isCodexExecJSON(spec adapter.EffectiveSpec) bool {
	return strings.EqualFold(strings.TrimSpace(spec.AdapterName), "codex") &&
		strings.EqualFold(strings.TrimSpace(stringValue(spec.EventParser.Type)), "jsonl_events") &&
		strings.EqualFold(strings.TrimSpace(stringValue(spec.EventParser.Profile)), "codex_exec")
}

func isOpencodeRunJSON(spec adapter.EffectiveSpec) bool {
	return strings.EqualFold(strings.TrimSpace(spec.AdapterName), "opencode") &&
		strings.EqualFold(strings.TrimSpace(stringValue(spec.EventParser.Type)), "jsonl_events") &&
		strings.EqualFold(strings.TrimSpace(stringValue(spec.EventParser.Profile)), "opencode_run")
}

func suppressUnstructuredRuntimeOutput(spec adapter.EffectiveSpec) bool {
	return isCodexExecJSON(spec) || isOpencodeRunJSON(spec)
}

type codexExecEnvelope struct {
	Type     string `json:"type"`
	ThreadID string `json:"thread_id"`
	Item     *struct {
		ID   string `json:"id"`
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"item"`
	Delta string `json:"delta"`
	Text  string `json:"text"`
}

type opencodeRunEnvelope struct {
	Type      string `json:"type"`
	SessionID string `json:"sessionID"`
	Part      *struct {
		ID        string `json:"id"`
		Type      string `json:"type"`
		Text      string `json:"text"`
		MessageID string `json:"messageID"`
		SessionID string `json:"sessionID"`
	} `json:"part"`
}

func (a *App) consumeCodexJSONOutput(sess *session.Session, sessionID string, data []byte, remainder string) (string, bool) {
	buffer := remainder + string(data)
	lines := strings.Split(buffer, "\n")
	nextRemainder := ""
	if len(lines) > 0 && !strings.HasSuffix(buffer, "\n") {
		nextRemainder = lines[len(lines)-1]
		lines = lines[:len(lines)-1]
	}

	consumed := false
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || !strings.HasPrefix(line, "{") {
			continue
		}

		var envelope codexExecEnvelope
		if err := json.Unmarshal([]byte(line), &envelope); err != nil {
			continue
		}
		consumed = true

		if nativeID := strings.TrimSpace(envelope.ThreadID); nativeID != "" {
			if sess.BindConversation(nativeID, model.ResumeStrategyNativeID) {
				snapshot := sess.Snapshot()
				_ = a.persistSessionSnapshot(snapshot, true)
				a.upsertConversationBinding(snapshot, nativeID)
				a.Hub.Publish(sessionID, event.ConversationBound(snapshot))
			}
		}

		text := codexAssistantText(envelope)
		if text == "" {
			continue
		}
		if sess.SetLastOutputPreview(outputPreview(text)) {
			snapshot := sess.Snapshot()
			_ = a.persistSessionSnapshot(snapshot, false)
		}

		messageID := newID("msg")
		a.Hub.Publish(sessionID, event.MessageStarted(sessionID, messageID, "assistant"))
		a.Hub.Publish(sessionID, event.MessageDelta(sessionID, messageID, "assistant", text))
		a.Hub.Publish(sessionID, event.MessageCompleted(sessionID, messageID, "assistant"))
	}

	return nextRemainder, consumed
}

func (a *App) consumeOpencodeJSONOutput(sess *session.Session, sessionID string, data []byte, remainder string) (string, bool) {
	buffer := remainder + string(data)
	lines := strings.Split(buffer, "\n")
	nextRemainder := ""
	if len(lines) > 0 && !strings.HasSuffix(buffer, "\n") {
		nextRemainder = lines[len(lines)-1]
		lines = lines[:len(lines)-1]
	}

	consumed := false
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || !strings.HasPrefix(line, "{") {
			continue
		}

		var envelope opencodeRunEnvelope
		if err := json.Unmarshal([]byte(line), &envelope); err != nil {
			continue
		}
		consumed = true

		if nativeID := strings.TrimSpace(envelope.SessionID); nativeID != "" {
			if sess.BindConversation(nativeID, model.ResumeStrategyNativeID) {
				snapshot := sess.Snapshot()
				_ = a.persistSessionSnapshot(snapshot, true)
				a.upsertConversationBinding(snapshot, nativeID)
				a.Hub.Publish(sessionID, event.ConversationBound(snapshot))
			}
		}

		if envelope.Part == nil || !strings.EqualFold(strings.TrimSpace(envelope.Part.Type), "text") {
			continue
		}
		text := strings.TrimSpace(envelope.Part.Text)
		if text == "" {
			continue
		}

		if sess.SetLastOutputPreview(outputPreview(text)) {
			snapshot := sess.Snapshot()
			_ = a.persistSessionSnapshot(snapshot, false)
		}

		messageID := strings.TrimSpace(envelope.Part.MessageID)
		if messageID == "" {
			messageID = newID("msg")
		}
		a.Hub.Publish(sessionID, event.MessageStarted(sessionID, messageID, "assistant"))
		a.Hub.Publish(sessionID, event.MessageDelta(sessionID, messageID, "assistant", text))
		a.Hub.Publish(sessionID, event.MessageCompleted(sessionID, messageID, "assistant"))
	}

	return nextRemainder, consumed
}

func codexAssistantText(envelope codexExecEnvelope) string {
	switch strings.TrimSpace(envelope.Type) {
	case "item.completed":
		if envelope.Item != nil && isCodexAssistantItemType(envelope.Item.Type) {
			return strings.TrimSpace(envelope.Item.Text)
		}
	case "item.delta":
		if envelope.Item != nil && isCodexAssistantItemType(envelope.Item.Type) {
			return strings.TrimSpace(firstNonEmpty(envelope.Delta, envelope.Text, envelope.Item.Text))
		}
	}
	return ""
}

func isCodexAssistantItemType(itemType string) bool {
	switch strings.TrimSpace(itemType) {
	case "agent_message", "assistant_message":
		return true
	default:
		return false
	}
}

func sanitizeOutputText(data []byte) string {
	text := ansiRegexp.ReplaceAllString(string(data), "")
	text = strings.ReplaceAll(text, "\r\n", "\n")
	text = strings.ReplaceAll(text, "\r", "\n")
	lines := strings.Split(text, "\n")
	filtered := make([]string, 0, len(lines))
	for _, line := range lines {
		line = strings.Map(func(r rune) rune {
			if r == '\t' {
				return r
			}
			if r < 0x20 || r == 0x7f {
				return -1
			}
			return r
		}, line)
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		filtered = append(filtered, line)
	}
	return strings.Join(filtered, "\n")
}

func outputPreview(text string) string {
	if strings.TrimSpace(text) == "" {
		return ""
	}

	lines := strings.Split(text, "\n")
	collected := make([]string, 0, 2)
	for i := len(lines) - 1; i >= 0 && len(collected) < 2; i-- {
		line := strings.TrimSpace(lines[i])
		if line == "" {
			continue
		}
		collected = append([]string{line}, collected...)
	}
	if len(collected) == 0 {
		return ""
	}

	preview := strings.Join(collected, " ")
	if len(preview) > 180 {
		preview = strings.TrimSpace(preview[:177]) + "..."
	}
	return preview
}

func rawSource(snapshot model.Session) string {
	mode := strings.TrimSpace(snapshot.RuntimeMode)
	if mode != "" {
		return mode
	}
	if snapshot.Capabilities.RequiresTTY {
		return "pty"
	}
	return "stdio"
}

func looksLikeInputRequest(raw []byte, text string) bool {
	lower := strings.ToLower(text)
	if strings.Contains(string(raw), "[?25h") {
		return true
	}
	for _, fragment := range []string{
		"waiting for input",
		"press enter",
		"continue?",
		"approve",
		"select an option",
		"choose an option",
	} {
		if strings.Contains(lower, fragment) {
			return true
		}
	}
	return false
}

func mergeArgs(base, extra []string) []string {
	args := make([]string, 0, len(base)+len(extra))
	args = append(args, base...)
	args = append(args, extra...)
	return args
}

func mergeEnv(base []string, env map[string]string, extra map[string]string) []string {
	merged := make(map[string]string, len(base)+len(env)+len(extra))
	order := make([]string, 0, len(base)+len(env)+len(extra))

	addKV := func(key, value string) {
		if key == "" {
			return
		}
		if _, seen := merged[key]; !seen {
			order = append(order, key)
		}
		merged[key] = value
	}

	for _, item := range base {
		parts := strings.SplitN(item, "=", 2)
		if len(parts) != 2 {
			continue
		}
		addKV(parts[0], parts[1])
	}
	for key, value := range env {
		addKV(key, value)
	}
	for key, value := range extra {
		if strings.TrimSpace(value) == "" {
			continue
		}
		addKV(key, value)
	}

	out := make([]string, 0, len(order))
	for _, key := range order {
		out = append(out, key+"="+merged[key])
	}
	return out
}

func stringValue(value *string) string {
	if value == nil {
		return ""
	}
	return strings.TrimSpace(*value)
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func resolveSessionFilePreviewPath(rawPath string, session model.Session) (string, error) {
	path := strings.TrimSpace(rawPath)
	if path == "" {
		return "", errors.New("path is required")
	}
	path = stripPreviewLineSuffix(path)
	if !filepath.IsAbs(path) {
		base := firstNonEmpty(session.WorkspacePath, session.WorkspaceRoot, session.GitRoot)
		if base == "" {
			return "", errors.New("relative paths require a session workspace")
		}
		path = filepath.Join(base, path)
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	abs = filepath.Clean(abs)
	if resolved, err := filepath.EvalSymlinks(abs); err == nil {
		abs = resolved
	} else if !os.IsNotExist(err) {
		return "", err
	}
	return filepath.Clean(abs), nil
}

func stripPreviewLineSuffix(path string) string {
	path = strings.TrimSpace(path)
	for {
		index := strings.LastIndex(path, ":")
		if index <= 0 || index == len(path)-1 {
			return path
		}
		if _, err := strconv.Atoi(path[index+1:]); err != nil {
			return path
		}
		path = path[:index]
	}
}

func filePreviewPathAllowed(path string, session model.Session, allowedRoots []string) bool {
	roots := make([]string, 0, len(allowedRoots)+3)
	for _, root := range []string{session.WorkspacePath, session.WorkspaceRoot, session.GitRoot} {
		if strings.TrimSpace(root) != "" {
			roots = append(roots, root)
		}
	}
	for _, root := range allowedRoots {
		if strings.TrimSpace(root) != "" {
			roots = append(roots, root)
		}
	}
	if len(roots) == 0 {
		return false
	}
	return pathWithinAllowedRoots(path, roots)
}

func filePreviewTemporaryImagePathAllowed(path string) bool {
	if !looksLikeImagePath(path) {
		return false
	}
	roots := []string{os.TempDir(), "/tmp", "/private/tmp"}
	return pathWithinAllowedRoots(path, roots)
}

func filePreviewDataLooksImage(path string, data []byte) bool {
	if len(data) == 0 {
		return false
	}
	if strings.HasPrefix(strings.ToLower(http.DetectContentType(data)), "image/") {
		return true
	}
	extensionMIME := strings.ToLower(strings.TrimSpace(mime.TypeByExtension(strings.ToLower(filepath.Ext(path)))))
	if extensionMIME == "image/svg+xml" && utf8.Valid(data) {
		return strings.Contains(strings.ToLower(string(data)), "<svg")
	}
	return false
}

func readFilePreviewBytes(path string, limit int) ([]byte, bool, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, false, err
	}
	defer file.Close()

	buffer := make([]byte, limit+1)
	n, err := io.ReadFull(file, buffer)
	if err != nil && !errors.Is(err, io.EOF) && !errors.Is(err, io.ErrUnexpectedEOF) {
		return nil, false, err
	}
	if n > limit {
		return buffer[:limit], true, nil
	}
	return buffer[:n], false, nil
}

func filePreviewKind(path string, mimeType string, data []byte) string {
	mimeType = strings.ToLower(strings.TrimSpace(mimeType))
	if strings.HasPrefix(mimeType, "image/") {
		return "image"
	}
	if strings.HasPrefix(mimeType, "text/") {
		return "text"
	}
	if languageForPreviewPath(path) != "" {
		return "text"
	}
	if len(data) == 0 {
		return "text"
	}
	if !utf8.Valid(data) {
		return "binary"
	}
	return "text"
}

func looksLikeImagePath(path string) bool {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".png", ".jpg", ".jpeg", ".gif", ".webp", ".svg":
		return true
	default:
		return false
	}
}

func languageForPreviewPath(path string) string {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".yaml", ".yml":
		return "yaml"
	case ".json":
		return "json"
	case ".go":
		return "go"
	case ".dart":
		return "dart"
	case ".js", ".jsx":
		return "javascript"
	case ".ts", ".tsx":
		return "typescript"
	case ".py":
		return "python"
	case ".sh", ".bash", ".zsh":
		return "shell"
	case ".toml":
		return "toml"
	case ".md", ".markdown":
		return "markdown"
	case ".html", ".htm":
		return "html"
	case ".css", ".scss":
		return "css"
	case ".xml", ".svg":
		return "xml"
	default:
		return ""
	}
}

func resolveAdapterDir(configured string) string {
	configured = strings.TrimSpace(configured)
	candidates := make([]string, 0, 6)
	if configured != "" {
		candidates = append(candidates, configured)
	}
	candidates = append(candidates, "adapters")

	if exe, err := os.Executable(); err == nil {
		exeDir := filepath.Dir(exe)
		candidates = append(candidates,
			filepath.Join(exeDir, "adapters"),
			filepath.Join(exeDir, "..", "adapters"),
			filepath.Join(exeDir, "..", "share", "agenleash", "adapters"),
			filepath.Join(exeDir, "..", "lib", "agenleash", "adapters"),
		)
	}

	for _, candidate := range normalizePathList(candidates) {
		info, err := os.Stat(candidate)
		if err != nil || !info.IsDir() {
			continue
		}
		return candidate
	}
	if configured == "" {
		return "adapters"
	}
	return filepath.Clean(configured)
}

func pathWithinAllowedRoots(path string, allowedRoots []string) bool {
	if len(allowedRoots) == 0 {
		return true
	}
	target := normalizePathForComparison(path)
	if target == "" {
		return false
	}
	for _, root := range allowedRoots {
		root = normalizePathForComparison(root)
		if root == "" {
			continue
		}
		rel, err := filepath.Rel(root, target)
		if err != nil {
			continue
		}
		if rel == "." || (rel != ".." && !strings.HasPrefix(rel, ".."+string(os.PathSeparator))) {
			return true
		}
	}
	return false
}

func normalizePathForComparison(path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return ""
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		abs = path
	}
	if resolved, err := filepath.EvalSymlinks(abs); err == nil {
		abs = resolved
	}
	return filepath.Clean(abs)
}

func newID(prefix string) string {
	var b [6]byte
	if _, err := rand.Read(b[:]); err != nil {
		return prefix + "_" + fmt.Sprintf("%d", time.Now().UTC().UnixNano())
	}
	return prefix + "_" + hex.EncodeToString(b[:])
}

func workspaceFingerprintFor(cwd, adapterName, versionHint, conversationID string) string {
	return strings.Join([]string{cwd, adapterName, versionHint, conversationID}, "|")
}
