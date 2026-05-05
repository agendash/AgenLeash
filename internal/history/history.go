package history

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha1"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	_ "modernc.org/sqlite"

	"github.com/agendash/AgenLeash/internal/model"
)

type Record struct {
	Session      model.Session
	Workspace    model.Workspace
	Conversation model.Conversation
}

type DiscoverConfig struct {
	DiscoverClaude   bool
	DiscoverCodex    bool
	DiscoverOpencode bool
	ClaudeRoot       string
	CodexRoot        string
	OpencodeRoot     string
}

type MessagePageOptions struct {
	Limit  int
	Offset int
}

type MessagePage struct {
	Messages   []model.ConversationMessage
	Limit      int
	Offset     int
	Returned   int
	HasMore    bool
	NextOffset int
}

const (
	defaultMessagePageLimit = 100
	maxMessagePageLimit     = 500
	historyScannerMaxToken  = 16 * 1024 * 1024
	initialTailReadBytes    = 256 * 1024
	toolOutputMaxRunes      = 4000
)

func LoadMessages(session model.Session) []model.ConversationMessage {
	switch strings.ToLower(strings.TrimSpace(session.Adapter)) {
	case "claudecode", "claude":
		return loadClaudeMessages(session)
	case "codex":
		return loadCodexMessages(session)
	case "opencode":
		return loadOpencodeMessages(session)
	default:
		return nil
	}
}

func LoadMessagePage(session model.Session, opts MessagePageOptions) MessagePage {
	switch strings.ToLower(strings.TrimSpace(session.Adapter)) {
	case "claudecode", "claude":
		return loadClaudeMessagePage(session, opts)
	case "codex":
		return loadCodexMessagePage(session, opts)
	case "opencode":
		return loadOpencodeMessagePage(session, opts)
	default:
		opts = normalizeMessagePageOptions(opts)
		return MessagePage{Limit: opts.Limit, Offset: opts.Offset}
	}
}

func Discover(ctx context.Context, cfg DiscoverConfig, supported map[string]bool) []Record {
	out := make([]Record, 0)
	if supported["claudecode"] && cfg.DiscoverClaude && strings.TrimSpace(cfg.ClaudeRoot) != "" {
		out = append(out, discoverClaude(ctx, filepath.Clean(strings.TrimSpace(cfg.ClaudeRoot)))...)
	}
	if supported["codex"] && cfg.DiscoverCodex && strings.TrimSpace(cfg.CodexRoot) != "" {
		out = append(out, discoverCodex(ctx, filepath.Clean(strings.TrimSpace(cfg.CodexRoot)))...)
	}
	if supported["opencode"] && cfg.DiscoverOpencode && strings.TrimSpace(cfg.OpencodeRoot) != "" {
		out = append(out, discoverOpencode(ctx, filepath.Clean(strings.TrimSpace(cfg.OpencodeRoot)))...)
	}

	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Session.LastSeen.Equal(out[j].Session.LastSeen) {
			return out[i].Session.ID < out[j].Session.ID
		}
		return out[i].Session.LastSeen.After(out[j].Session.LastSeen)
	})
	return out
}

func DiscoverClaudeRecords(ctx context.Context, root string) []Record {
	root = strings.TrimSpace(root)
	if root == "" {
		return nil
	}
	return discoverClaude(ctx, filepath.Clean(root))
}

func DiscoverCodexRecords(ctx context.Context, root string) []Record {
	root = strings.TrimSpace(root)
	if root == "" {
		return nil
	}
	return discoverCodex(ctx, filepath.Clean(root))
}

func DiscoverOpencodeRecords(ctx context.Context, root string) []Record {
	root = strings.TrimSpace(root)
	if root == "" {
		return nil
	}
	return discoverOpencode(ctx, filepath.Clean(root))
}

type claudeIndex struct {
	Entries []claudeIndexEntry `json:"entries"`
}

type claudeIndexEntry struct {
	SessionID   string `json:"sessionId"`
	FullPath    string `json:"fullPath"`
	Created     string `json:"created"`
	Modified    string `json:"modified"`
	GitBranch   string `json:"gitBranch"`
	ProjectPath string `json:"projectPath"`
}

type claudeSessionMeta struct {
	CWD               string
	GitBranch         string
	Version           string
	LastOutputPreview string
}

func discoverClaude(ctx context.Context, root string) []Record {
	indexRoot := filepath.Join(root, "projects")
	info, err := os.Stat(indexRoot)
	if err != nil || !info.IsDir() {
		return nil
	}

	seen := make(map[string]struct{})
	records := make([]Record, 0)
	_ = filepath.WalkDir(indexRoot, func(path string, d os.DirEntry, err error) error {
		if err != nil || d == nil || d.IsDir() {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		switch {
		case d.Name() == "sessions-index.json":
			records = append(records, discoverClaudeIndexFile(path, seen)...)
		case filepath.Ext(d.Name()) == ".jsonl":
			if record, ok := discoverClaudeProjectFile(path, seen); ok {
				records = append(records, record)
			}
		}
		return nil
	})
	return records
}

func discoverClaudeIndexFile(path string, seen map[string]struct{}) []Record {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var index claudeIndex
	if err := json.Unmarshal(data, &index); err != nil {
		return nil
	}

	records := make([]Record, 0, len(index.Entries))
	for _, entry := range index.Entries {
		record, ok := newClaudeRecord(
			strings.TrimSpace(entry.SessionID),
			strings.TrimSpace(entry.FullPath),
			strings.TrimSpace(entry.ProjectPath),
			strings.TrimSpace(entry.GitBranch),
			parseTime(entry.Created),
			parseTime(entry.Modified),
			seen,
		)
		if ok {
			records = append(records, record)
		}
	}
	return records
}

func discoverClaudeProjectFile(path string, seen map[string]struct{}) (Record, bool) {
	info, err := os.Stat(path)
	if err != nil || info.IsDir() {
		return Record{}, false
	}
	rawID := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	return newClaudeRecord(rawID, path, "", "", info.ModTime().UTC(), info.ModTime().UTC(), seen)
}

func newClaudeRecord(rawID, detailPath, projectPath, gitBranch string, createdAt, modifiedAt time.Time, seen map[string]struct{}) (Record, bool) {
	rawID = strings.TrimSpace(rawID)
	detailPath = strings.TrimSpace(detailPath)
	if rawID == "" || detailPath == "" {
		return Record{}, false
	}
	if _, ok := seen[rawID]; ok {
		return Record{}, false
	}

	meta := parseClaudeSessionFile(detailPath)
	workspacePath := firstNonEmpty(projectPath, meta.CWD)
	if workspacePath == "" {
		return Record{}, false
	}
	workspacePath = filepath.Clean(workspacePath)
	gitBranch = firstNonEmpty(gitBranch, meta.GitBranch)

	if modifiedAt.IsZero() {
		modifiedAt = createdAt
	}
	if createdAt.IsZero() {
		createdAt = modifiedAt
	}

	seen[rawID] = struct{}{}
	workspaceID := stableID("ws", "claudecode", workspacePath)
	sessionID := stableID("sess", "claudecode", rawID)
	conversationID := stableID("conv", "claudecode", rawID)
	isSubagent := strings.HasPrefix(strings.ToLower(rawID), "agent-") ||
		strings.Contains(strings.ToLower(filepath.ToSlash(detailPath)), "/subagents/")

	capabilities := model.Capabilities{}
	if !isSubagent {
		capabilities.SupportsResume = true
		capabilities.SupportsNativeConversation = true
	}

	return Record{
		Session: model.Session{
			ID:                   sessionID,
			Adapter:              "claudecode",
			Origin:               model.SessionOriginDiscovered,
			NativeConversationID: rawID,
			StartMode:            model.StartModeNew,
			ResumeStrategy:       model.ResumeStrategyNativeID,
			WorkspacePath:        workspacePath,
			WorkspaceRoot:        workspacePath,
			WorkspaceFingerprint: strings.Join([]string{workspacePath, "claudecode", rawID}, "|"),
			GitRoot:              workspacePath,
			GitBranch:            gitBranch,
			State:                model.SessionStateStopped,
			ConversationID:       conversationID,
			WorkspaceID:          workspaceID,
			DetailPath:           detailPath,
			CreatedAt:            createdAt,
			LastSeen:             modifiedAt,
			LastOutputPreview:    meta.LastOutputPreview,
			Capabilities:         capabilities,
		},
		Workspace: model.Workspace{
			ID:              workspaceID,
			Path:            workspacePath,
			Root:            workspacePath,
			Fingerprint:     strings.Join([]string{workspacePath, "claudecode"}, "|"),
			GitRoot:         workspacePath,
			GitBranch:       gitBranch,
			ActiveSessionID: sessionID,
			CreatedAt:       createdAt,
			UpdatedAt:       modifiedAt,
		},
		Conversation: model.Conversation{
			ID:          conversationID,
			SessionID:   sessionID,
			NativeID:    rawID,
			WorkspaceID: workspaceID,
			State:       string(model.SessionStateStopped),
			CreatedAt:   createdAt,
			UpdatedAt:   modifiedAt,
		},
	}, true
}

func loadClaudeMessages(session model.Session) []model.ConversationMessage {
	path := strings.TrimSpace(session.DetailPath)
	if path == "" {
		return nil
	}
	return loadMessagesFromPath(path, session, parseClaudeMessageLine)
}

func parseClaudeSessionFile(path string) claudeSessionMeta {
	if strings.TrimSpace(path) == "" {
		return claudeSessionMeta{}
	}
	meta := claudeSessionMeta{}
	for _, line := range readTailLines(path, 128*1024, 256) {
		var row map[string]json.RawMessage
		if err := json.Unmarshal([]byte(line), &row); err != nil {
			continue
		}
		meta.CWD = firstNonEmpty(meta.CWD, unmarshalString(row["cwd"]))
		meta.Version = firstNonEmpty(meta.Version, unmarshalString(row["version"]))
		meta.GitBranch = firstNonEmpty(unmarshalString(row["gitBranch"]), meta.GitBranch)
		if preview := extractClaudeAssistantPreview(row); preview != "" {
			meta.LastOutputPreview = preview
		}
	}
	return meta
}

func discoverCodex(ctx context.Context, root string) []Record {
	statePaths := listMatches(filepath.Join(root, "state_*.sqlite"))
	if len(statePaths) == 0 {
		return nil
	}

	best := make(map[string]Record)
	for _, statePath := range statePaths {
		records := discoverCodexStateFile(ctx, statePath)
		for _, record := range records {
			current, ok := best[record.Session.ID]
			if !ok || record.Session.LastSeen.After(current.Session.LastSeen) {
				best[record.Session.ID] = record
			}
		}
	}

	records := make([]Record, 0, len(best))
	for _, record := range best {
		records = append(records, record)
	}
	return records
}

func discoverCodexStateFile(ctx context.Context, statePath string) []Record {
	db, err := openReadOnlySQLite(statePath)
	if err != nil {
		return nil
	}
	defer db.Close()

	query, err := codexThreadsQuery(db)
	if err != nil {
		return nil
	}
	rows, err := db.QueryContext(ctx, query)
	if err != nil {
		return nil
	}
	defer rows.Close()

	records := make([]Record, 0)
	for rows.Next() {
		var (
			rawID       string
			cwd         string
			gitBranch   string
			cliVersion  string
			rolloutPath string
			createdMS   int64
			updatedMS   int64
		)
		if err := rows.Scan(&rawID, &cwd, &gitBranch, &cliVersion, &rolloutPath, &createdMS, &updatedMS); err != nil {
			continue
		}
		cwd = filepath.Clean(strings.TrimSpace(cwd))
		if cwd == "" {
			continue
		}
		createdAt := unixMillis(createdMS)
		updatedAt := unixMillis(updatedMS)
		if updatedAt.IsZero() {
			updatedAt = createdAt
		}
		if createdAt.IsZero() {
			createdAt = updatedAt
		}

		workspaceID := stableID("ws", "codex", cwd)
		sessionID := stableID("sess", "codex", rawID)
		conversationID := stableID("conv", "codex", rawID)

		records = append(records, Record{
			Session: model.Session{
				ID:                   sessionID,
				Adapter:              "codex",
				Origin:               model.SessionOriginDiscovered,
				NativeConversationID: rawID,
				StartMode:            model.StartModeNew,
				ResumeStrategy:       model.ResumeStrategyNativeID,
				WorkspacePath:        cwd,
				WorkspaceRoot:        cwd,
				WorkspaceFingerprint: strings.Join([]string{cwd, "codex", rawID}, "|"),
				GitRoot:              cwd,
				GitBranch:            gitBranch,
				State:                model.SessionStateStopped,
				ConversationID:       conversationID,
				WorkspaceID:          workspaceID,
				DetailPath:           statePath + "#threads/" + rawID,
				CreatedAt:            createdAt,
				LastSeen:             updatedAt,
				LastOutputPreview:    parseCodexRolloutPreview(resolveCodexMountedPath(statePath, rolloutPath)),
				Capabilities: model.Capabilities{
					SupportsResume:             true,
					SupportsNativeConversation: true,
				},
				Features: model.FeatureSet{
					"jsonEventStream": true,
					"streamingText":   true,
				},
			},
			Workspace: model.Workspace{
				ID:              workspaceID,
				Path:            cwd,
				Root:            cwd,
				Fingerprint:     strings.Join([]string{cwd, "codex"}, "|"),
				GitRoot:         cwd,
				GitBranch:       gitBranch,
				ActiveSessionID: sessionID,
				CreatedAt:       createdAt,
				UpdatedAt:       updatedAt,
			},
			Conversation: model.Conversation{
				ID:          conversationID,
				SessionID:   sessionID,
				NativeID:    rawID,
				WorkspaceID: workspaceID,
				State:       string(model.SessionStateStopped),
				CreatedAt:   createdAt,
				UpdatedAt:   updatedAt,
			},
		})
	}
	return records
}

func discoverOpencode(ctx context.Context, root string) []Record {
	dbPath := resolveOpencodeDBPath(root)
	if dbPath == "" {
		return nil
	}

	db, err := openReadOnlySQLite(dbPath)
	if err != nil {
		return nil
	}
	defer db.Close()

	rows, err := db.QueryContext(ctx, `
		SELECT
			id,
			IFNULL(directory, ''),
			IFNULL(title, ''),
			IFNULL(version, ''),
			IFNULL(parent_id, ''),
			IFNULL(time_created, 0),
			IFNULL(time_updated, 0)
		FROM session
		WHERE IFNULL(time_archived, 0) = 0
	`)
	if err != nil {
		return nil
	}
	defer rows.Close()

	records := make([]Record, 0)
	for rows.Next() {
		select {
		case <-ctx.Done():
			return records
		default:
		}

		var (
			rawID     string
			directory string
			title     string
			version   string
			parentID  string
			createdMS int64
			updatedMS int64
		)
		if err := rows.Scan(&rawID, &directory, &title, &version, &parentID, &createdMS, &updatedMS); err != nil {
			continue
		}

		directory = filepath.Clean(strings.TrimSpace(directory))
		rawID = strings.TrimSpace(rawID)
		if rawID == "" || directory == "" {
			continue
		}

		createdAt := unixMillis(createdMS)
		updatedAt := unixMillis(updatedMS)
		if updatedAt.IsZero() {
			updatedAt = createdAt
		}
		if createdAt.IsZero() {
			createdAt = updatedAt
		}

		workspaceID := stableID("ws", "opencode", directory)
		sessionID := stableID("sess", "opencode", rawID)
		conversationID := stableID("conv", "opencode", rawID)

		preview := queryOpencodeAssistantPreview(db, rawID)
		if preview == "" {
			preview = strings.TrimSpace(title)
		}

		record := Record{
			Session: model.Session{
				ID:                   sessionID,
				Adapter:              "opencode",
				Origin:               model.SessionOriginDiscovered,
				NativeConversationID: rawID,
				StartMode:            model.StartModeResume,
				ResumeStrategy:       model.ResumeStrategyNativeID,
				WorkspacePath:        directory,
				WorkspaceRoot:        directory,
				WorkspaceFingerprint: strings.Join([]string{directory, "opencode", rawID}, "|"),
				GitRoot:              directory,
				State:                model.SessionStateStopped,
				ConversationID:       conversationID,
				WorkspaceID:          workspaceID,
				DetailPath:           dbPath + "#sessions/" + rawID,
				CreatedAt:            createdAt,
				LastSeen:             updatedAt,
				LastOutputPreview:    preview,
				Capabilities: model.Capabilities{
					SupportsResume:             true,
					SupportsNativeConversation: true,
				},
			},
			Workspace: model.Workspace{
				ID:              workspaceID,
				Path:            directory,
				Root:            directory,
				Fingerprint:     strings.Join([]string{directory, "opencode"}, "|"),
				GitRoot:         directory,
				ActiveSessionID: sessionID,
				CreatedAt:       createdAt,
				UpdatedAt:       updatedAt,
			},
			Conversation: model.Conversation{
				ID:          conversationID,
				SessionID:   sessionID,
				NativeID:    rawID,
				WorkspaceID: workspaceID,
				State:       string(model.SessionStateStopped),
				CreatedAt:   createdAt,
				UpdatedAt:   updatedAt,
			},
		}
		if strings.TrimSpace(parentID) != "" {
			record.Session.Features = model.FeatureSet{"hasParentSession": true}
		}
		if strings.TrimSpace(version) != "" {
			if record.Session.Features == nil {
				record.Session.Features = model.FeatureSet{}
			}
			record.Session.Features["opencode.version."+version] = true
		}
		records = append(records, record)
	}
	return records
}

func codexThreadsQuery(db *sql.DB) (string, error) {
	columns, err := tableColumns(db, "threads")
	if err != nil {
		return "", err
	}
	if len(columns) == 0 || !columns["id"] {
		return "", errors.New("threads table is missing required id column")
	}

	stringExpr := func(column string) string {
		if columns[column] {
			return fmt.Sprintf("IFNULL(%s, '')", column)
		}
		return "''"
	}
	timeExpr := func(msColumn, secColumn string) string {
		switch {
		case columns[msColumn]:
			return fmt.Sprintf("IFNULL(%s, 0)", msColumn)
		case columns[secColumn]:
			return fmt.Sprintf("CAST(IFNULL(%s, 0) * 1000 AS INTEGER)", secColumn)
		default:
			return "0"
		}
	}

	return fmt.Sprintf(`
		SELECT
			id,
			%s,
			%s,
			%s,
			%s,
			%s,
			%s
		FROM threads
	`, stringExpr("cwd"), stringExpr("git_branch"), stringExpr("cli_version"), stringExpr("rollout_path"), timeExpr("created_at_ms", "created_at"), timeExpr("updated_at_ms", "updated_at")), nil
}

func loadCodexMessages(session model.Session) []model.ConversationMessage {
	rolloutPath := resolveCodexRolloutPath(strings.TrimSpace(session.DetailPath))
	if rolloutPath == "" {
		return nil
	}
	return loadMessagesFromPath(rolloutPath, session, parseCodexMessageLine)
}

func loadOpencodeMessages(session model.Session) []model.ConversationMessage {
	dbPath, nativeID := resolveOpencodeSessionPath(strings.TrimSpace(session.DetailPath))
	if dbPath == "" || nativeID == "" {
		return nil
	}
	return loadOpencodeMessagesFromDB(dbPath, session, nativeID)
}

func loadClaudeMessagePage(session model.Session, opts MessagePageOptions) MessagePage {
	path := strings.TrimSpace(session.DetailPath)
	return loadMessagePageFromPath(path, session, opts, parseClaudeMessageLine)
}

func loadCodexMessagePage(session model.Session, opts MessagePageOptions) MessagePage {
	rolloutPath := resolveCodexRolloutPath(strings.TrimSpace(session.DetailPath))
	return loadMessagePageFromPath(rolloutPath, session, opts, parseCodexMessageLine)
}

func loadOpencodeMessagePage(session model.Session, opts MessagePageOptions) MessagePage {
	return paginateMessages(loadOpencodeMessages(session), opts)
}

func parseTime(raw string) time.Time {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return time.Time{}
	}
	parsed, err := time.Parse(time.RFC3339Nano, raw)
	if err != nil {
		return time.Time{}
	}
	return parsed.UTC()
}

func resolveCodexRolloutPath(detailPath string) string {
	detailPath = strings.TrimSpace(detailPath)
	if detailPath == "" {
		return ""
	}
	if strings.HasSuffix(detailPath, ".jsonl") {
		return detailPath
	}

	dbPath, threadID, ok := strings.Cut(detailPath, "#threads/")
	if !ok {
		return ""
	}
	dbPath = strings.TrimSpace(dbPath)
	threadID = strings.TrimSpace(threadID)
	if dbPath == "" || threadID == "" {
		return ""
	}

	db, err := openReadOnlySQLite(dbPath)
	if err != nil {
		return ""
	}
	defer db.Close()

	var rolloutPath string
	if err := db.QueryRow(`SELECT IFNULL(rollout_path, '') FROM threads WHERE id = ?`, threadID).Scan(&rolloutPath); err != nil {
		return ""
	}
	return resolveCodexMountedPath(dbPath, rolloutPath)
}

func resolveOpencodeDBPath(root string) string {
	root = strings.TrimSpace(root)
	if root == "" {
		return ""
	}

	candidates := []string{filepath.Clean(root)}
	if filepath.Ext(root) != ".db" {
		candidates = append(candidates, filepath.Join(filepath.Clean(root), "opencode.db"))
	}
	for _, candidate := range candidates {
		info, err := os.Stat(candidate)
		if err != nil || info.IsDir() {
			continue
		}
		return candidate
	}
	return ""
}

func resolveOpencodeSessionPath(detailPath string) (string, string) {
	detailPath = strings.TrimSpace(detailPath)
	if detailPath == "" {
		return "", ""
	}
	dbPath, sessionID, ok := strings.Cut(detailPath, "#sessions/")
	if !ok {
		return "", ""
	}
	dbPath = resolveOpencodeDBPath(dbPath)
	sessionID = strings.TrimSpace(sessionID)
	if dbPath == "" || sessionID == "" {
		return "", ""
	}
	return dbPath, sessionID
}

func openReadOnlySQLite(path string) (*sql.DB, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil, errors.New("sqlite path is required")
	}
	uri := url.URL{
		Scheme:   "file",
		Path:     filepath.Clean(path),
		RawQuery: "mode=ro&immutable=1",
	}
	return sql.Open("sqlite", uri.String())
}

func tableColumns(db *sql.DB, table string) (map[string]bool, error) {
	rows, err := db.Query(`PRAGMA table_info(` + table + `)`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	columns := make(map[string]bool)
	for rows.Next() {
		var (
			cid        int
			name       string
			typeName   string
			notNull    int
			defaultV   any
			primaryKey int
		)
		if err := rows.Scan(&cid, &name, &typeName, &notNull, &defaultV, &primaryKey); err != nil {
			return nil, err
		}
		columns[strings.TrimSpace(name)] = true
	}
	return columns, rows.Err()
}

func normalizeMessagePageOptions(opts MessagePageOptions) MessagePageOptions {
	if opts.Limit <= 0 {
		opts.Limit = defaultMessagePageLimit
	}
	if opts.Limit > maxMessagePageLimit {
		opts.Limit = maxMessagePageLimit
	}
	if opts.Offset < 0 {
		opts.Offset = 0
	}
	return opts
}

func paginateMessages(messages []model.ConversationMessage, opts MessagePageOptions) MessagePage {
	opts = normalizeMessagePageOptions(opts)
	page := MessagePage{
		Limit:  opts.Limit,
		Offset: opts.Offset,
	}
	if len(messages) == 0 || opts.Offset >= len(messages) {
		return page
	}

	end := len(messages) - opts.Offset
	if end < 0 {
		end = 0
	}
	start := end - opts.Limit
	if start < 0 {
		start = 0
	}
	page.Messages = append(page.Messages, messages[start:end]...)
	page.Returned = len(page.Messages)
	page.HasMore = start > 0
	if page.HasMore {
		page.NextOffset = opts.Offset + page.Returned
	}
	return page
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			return value
		}
	}
	return ""
}

func stableID(prefix string, parts ...string) string {
	hasher := sha1.New()
	for _, part := range parts {
		_, _ = hasher.Write([]byte(strings.TrimSpace(part)))
		_, _ = hasher.Write([]byte{0})
	}
	return prefix + "_" + hex.EncodeToString(hasher.Sum(nil))[:12]
}

func listMatches(pattern string) []string {
	matches, err := filepath.Glob(pattern)
	if err != nil || len(matches) == 0 {
		return nil
	}
	sort.SliceStable(matches, func(i, j int) bool {
		left, errLeft := os.Stat(matches[i])
		right, errRight := os.Stat(matches[j])
		if errLeft != nil || errRight != nil {
			return matches[i] > matches[j]
		}
		return left.ModTime().After(right.ModTime())
	})
	return matches
}

func unixMillis(v int64) time.Time {
	if v <= 0 {
		return time.Time{}
	}
	return time.UnixMilli(v).UTC()
}

func unmarshalString(raw json.RawMessage) string {
	var value string
	if err := json.Unmarshal(raw, &value); err != nil {
		return ""
	}
	return strings.TrimSpace(value)
}

func readTailChunk(path string, maxBytes int64) []byte {
	if strings.TrimSpace(path) == "" || maxBytes <= 0 {
		return nil
	}

	file, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer file.Close()

	info, err := file.Stat()
	if err != nil || info.Size() <= 0 {
		return nil
	}

	start := int64(0)
	if info.Size() > maxBytes {
		start = info.Size() - maxBytes
	}
	if _, err := file.Seek(start, io.SeekStart); err != nil {
		return nil
	}

	chunk, err := io.ReadAll(file)
	if err != nil || len(chunk) == 0 {
		return nil
	}
	if start > 0 {
		if idx := bytes.IndexByte(chunk, '\n'); idx >= 0 && idx+1 < len(chunk) {
			chunk = chunk[idx+1:]
		}
	}
	return chunk
}

func readTailLines(path string, maxBytes int64, maxLines int) []string {
	if maxLines <= 0 {
		return nil
	}
	chunk := readTailChunk(path, maxBytes)
	if len(chunk) == 0 {
		return nil
	}

	scanner := bufio.NewScanner(bytes.NewReader(chunk))
	scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	lines := make([]string, 0, maxLines)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		lines = append(lines, line)
		if len(lines) > maxLines {
			lines = lines[len(lines)-maxLines:]
		}
	}
	return lines
}

func resolveCodexMountedPath(dbPath, rolloutPath string) string {
	rolloutPath = strings.TrimSpace(rolloutPath)
	if rolloutPath == "" {
		return ""
	}
	rolloutPath = filepath.Clean(rolloutPath)
	if _, err := os.Stat(rolloutPath); err == nil {
		return rolloutPath
	}

	codexRoot := filepath.Dir(strings.TrimSpace(dbPath))
	codexRoot = filepath.Clean(codexRoot)
	if codexRoot == "" || codexRoot == "." {
		return rolloutPath
	}

	slashed := filepath.ToSlash(rolloutPath)
	for _, marker := range []string{"/.codex/", "/sessions/"} {
		if idx := strings.Index(slashed, marker); idx >= 0 {
			trimmed := strings.TrimPrefix(slashed[idx+1:], ".codex/")
			trimmed = strings.TrimPrefix(trimmed, "sessions/")
			if marker == "/sessions/" {
				trimmed = strings.TrimPrefix(slashed[idx+1:], "sessions/")
			}
			candidate := filepath.Join(codexRoot, "sessions", filepath.FromSlash(trimmed))
			if marker == "/.codex/" {
				candidate = filepath.Join(codexRoot, filepath.FromSlash(trimmed))
			}
			if _, err := os.Stat(candidate); err == nil {
				return candidate
			}
		}
	}

	return rolloutPath
}

func extractClaudeUserText(raw json.RawMessage) string {
	var message struct {
		Role    string          `json:"role"`
		Content json.RawMessage `json:"content"`
	}
	if err := json.Unmarshal(raw, &message); err != nil {
		return ""
	}
	if strings.TrimSpace(message.Role) != "user" {
		return ""
	}
	return flattenClaudeContent(message.Content)
}

func extractClaudeUserMessage(raw json.RawMessage) (string, string, []model.ToolUse) {
	var message struct {
		Role    string          `json:"role"`
		Content json.RawMessage `json:"content"`
	}
	if err := json.Unmarshal(raw, &message); err != nil {
		return "", "", nil
	}
	if strings.TrimSpace(message.Role) != "user" {
		return "", "", nil
	}

	text := flattenClaudeContent(message.Content)
	toolResults := extractClaudeToolResults(message.Content)
	if len(toolResults) > 0 && strings.TrimSpace(text) == "" {
		return "tool", "", toolResults
	}
	return "user", text, toolResults
}

func extractClaudeAssistantText(raw json.RawMessage) string {
	var message struct {
		Role    string `json:"role"`
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
	}
	if err := json.Unmarshal(raw, &message); err != nil {
		return ""
	}
	if strings.TrimSpace(message.Role) != "assistant" {
		return ""
	}

	parts := make([]string, 0, len(message.Content))
	for _, item := range message.Content {
		switch strings.TrimSpace(item.Type) {
		case "text", "output_text":
			if text := strings.TrimSpace(item.Text); text != "" {
				parts = append(parts, text)
			}
		}
	}
	return strings.TrimSpace(strings.Join(parts, "\n\n"))
}

func extractClaudeAssistantMessage(raw json.RawMessage) (string, []model.ToolUse) {
	var message struct {
		Role    string          `json:"role"`
		Content json.RawMessage `json:"content"`
	}
	if err := json.Unmarshal(raw, &message); err != nil {
		return "", nil
	}
	if strings.TrimSpace(message.Role) != "assistant" {
		return "", nil
	}
	return extractClaudeAssistantText(raw), extractClaudeToolUses(message.Content)
}

func flattenClaudeContent(raw json.RawMessage) string {
	var plain string
	if err := json.Unmarshal(raw, &plain); err == nil {
		return strings.TrimSpace(plain)
	}

	var parts []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	if err := json.Unmarshal(raw, &parts); err != nil {
		return ""
	}

	fragments := make([]string, 0, len(parts))
	for _, item := range parts {
		switch strings.TrimSpace(item.Type) {
		case "text", "input_text", "output_text":
			if text := strings.TrimSpace(item.Text); text != "" {
				fragments = append(fragments, text)
			}
		}
	}
	return strings.TrimSpace(strings.Join(fragments, "\n\n"))
}

func extractClaudeToolUses(raw json.RawMessage) []model.ToolUse {
	var parts []struct {
		Type  string         `json:"type"`
		ID    string         `json:"id"`
		Name  string         `json:"name"`
		Input map[string]any `json:"input"`
	}
	if err := json.Unmarshal(raw, &parts); err != nil {
		return nil
	}

	out := make([]model.ToolUse, 0, len(parts))
	for _, item := range parts {
		if strings.TrimSpace(item.Type) != "tool_use" {
			continue
		}
		id := strings.TrimSpace(item.ID)
		name := strings.TrimSpace(item.Name)
		if id == "" && name == "" {
			continue
		}
		out = append(out, model.ToolUse{
			ID:          id,
			Name:        firstNonEmptyString(name, "Tool call"),
			State:       "running",
			Description: toolUseDescription(name, item.Input),
			Input:       compactAnyMap(item.Input),
		})
	}
	return out
}

func extractClaudeToolResults(raw json.RawMessage) []model.ToolUse {
	var parts []struct {
		Type      string          `json:"type"`
		ToolUseID string          `json:"tool_use_id"`
		Content   json.RawMessage `json:"content"`
		IsError   bool            `json:"is_error"`
	}
	if err := json.Unmarshal(raw, &parts); err != nil {
		return nil
	}

	out := make([]model.ToolUse, 0, len(parts))
	for _, item := range parts {
		if strings.TrimSpace(item.Type) != "tool_result" {
			continue
		}
		id := strings.TrimSpace(item.ToolUseID)
		if id == "" {
			id = stableID("tool", string(item.Content))
		}
		state := "success"
		if item.IsError {
			state = "failed"
		}
		out = append(out, model.ToolUse{
			ID:          id,
			Name:        "Tool result",
			State:       state,
			Description: "Tool returned output",
			Output: map[string]any{
				"content": claudeToolResultContent(item.Content),
			},
		})
	}
	return out
}

func claudeToolResultContent(raw json.RawMessage) string {
	var text string
	if err := json.Unmarshal(raw, &text); err == nil {
		return strings.TrimSpace(text)
	}
	if flattened := flattenClaudeContent(raw); flattened != "" {
		return flattened
	}
	var value any
	if err := json.Unmarshal(raw, &value); err != nil {
		return strings.TrimSpace(string(raw))
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		return strings.TrimSpace(string(raw))
	}
	return string(encoded)
}

func toolUseDescription(name string, input map[string]any) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return "Tool call requested"
	}
	if command, ok := input["command"].(string); ok && strings.TrimSpace(command) != "" {
		return strings.TrimSpace(command)
	}
	if pattern, ok := input["pattern"].(string); ok && strings.TrimSpace(pattern) != "" {
		return name + ": " + strings.TrimSpace(pattern)
	}
	return name + " requested"
}

func compactAnyMap(input map[string]any) map[string]any {
	if len(input) == 0 {
		return nil
	}
	out := make(map[string]any, len(input))
	for key, value := range input {
		key = strings.TrimSpace(key)
		if key == "" || value == nil {
			continue
		}
		out[key] = value
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func firstNonEmptyString(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func parseCodexToolArguments(raw string) map[string]any {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	var decoded map[string]any
	if err := json.Unmarshal([]byte(raw), &decoded); err == nil {
		return compactAnyMap(decoded)
	}
	var value any
	if err := json.Unmarshal([]byte(raw), &value); err == nil {
		return compactAnyMap(map[string]any{"arguments": value})
	}
	return map[string]any{"arguments": raw}
}

func compactToolOutput(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	runes := []rune(raw)
	if len(runes) <= toolOutputMaxRunes {
		return raw
	}
	return strings.TrimSpace(string(runes[:toolOutputMaxRunes])) + "\n... output truncated ..."
}

func codexToolState(exitCode *int, status string) string {
	status = strings.ToLower(strings.TrimSpace(status))
	if strings.Contains(status, "fail") || strings.Contains(status, "error") {
		return "failed"
	}
	if exitCode != nil && *exitCode != 0 {
		return "failed"
	}
	return "success"
}

func toolUseStableKey(toolUses []model.ToolUse) string {
	if len(toolUses) == 0 {
		return ""
	}
	parts := make([]string, 0, len(toolUses))
	for _, toolUse := range toolUses {
		parts = append(parts, strings.TrimSpace(toolUse.ID)+"="+strings.TrimSpace(toolUse.Name))
	}
	return strings.Join(parts, "|")
}

func extractClaudeAssistantPreview(row map[string]json.RawMessage) string {
	if strings.TrimSpace(unmarshalString(row["type"])) != "assistant" {
		return ""
	}

	var message struct {
		Role    string `json:"role"`
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
	}
	if err := json.Unmarshal(row["message"], &message); err != nil {
		return ""
	}
	if strings.TrimSpace(message.Role) != "assistant" {
		return ""
	}

	parts := make([]string, 0, len(message.Content))
	for _, item := range message.Content {
		if strings.TrimSpace(item.Type) != "text" {
			continue
		}
		parts = append(parts, item.Text)
	}
	return summarizeHistoryText(parts...)
}

func loadMessagesFromPath(path string, session model.Session, parser func(string, model.Session) (model.ConversationMessage, bool)) []model.ConversationMessage {
	file, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer file.Close()

	return parseMessages(file, session, parser)
}

func loadMessagePageFromPath(path string, session model.Session, opts MessagePageOptions, parser func(string, model.Session) (model.ConversationMessage, bool)) MessagePage {
	opts = normalizeMessagePageOptions(opts)
	page := MessagePage{
		Limit:  opts.Limit,
		Offset: opts.Offset,
	}
	path = strings.TrimSpace(path)
	if path == "" {
		return page
	}

	info, err := os.Stat(path)
	if err != nil || info.IsDir() || info.Size() <= 0 {
		return page
	}

	need := opts.Offset + opts.Limit + 1
	readBytes := int64(initialTailReadBytes)
	if info.Size() < readBytes {
		readBytes = info.Size()
	}

	var messages []model.ConversationMessage
	for {
		chunk := readTailChunk(path, readBytes)
		if len(chunk) == 0 {
			break
		}
		messages = parseMessages(bytes.NewReader(chunk), session, parser)
		if len(messages) >= need || readBytes >= info.Size() {
			break
		}
		readBytes *= 2
		if readBytes > info.Size() {
			readBytes = info.Size()
		}
	}

	return paginateMessages(messages, opts)
}

func parseMessages(reader io.Reader, session model.Session, parser func(string, model.Session) (model.ConversationMessage, bool)) []model.ConversationMessage {
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 0, 64*1024), historyScannerMaxToken)
	out := make([]model.ConversationMessage, 0, 128)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		message, ok := parser(line, session)
		if !ok {
			continue
		}
		out = append(out, message)
	}
	return mergeToolUseResultMessages(out)
}

func mergeToolUseResultMessages(messages []model.ConversationMessage) []model.ConversationMessage {
	if len(messages) == 0 {
		return messages
	}

	type toolRef struct {
		messageIndex int
		toolIndex    int
	}
	toolRefs := make(map[string]toolRef)
	merged := make([]model.ConversationMessage, 0, len(messages))
	for _, message := range messages {
		if strings.TrimSpace(message.Role) == "tool" && len(message.ToolUses) > 0 {
			unmerged := make([]model.ToolUse, 0, len(message.ToolUses))
			for _, result := range message.ToolUses {
				id := strings.TrimSpace(result.ID)
				ref, ok := toolRefs[id]
				if id == "" || !ok {
					unmerged = append(unmerged, result)
					continue
				}
				target := &merged[ref.messageIndex].ToolUses[ref.toolIndex]
				if strings.TrimSpace(result.State) != "" {
					target.State = result.State
				}
				if len(target.Output) == 0 && len(result.Output) > 0 {
					target.Output = result.Output
				}
				if target.CompletedAt.IsZero() && !result.CompletedAt.IsZero() {
					target.CompletedAt = result.CompletedAt
				}
				if strings.TrimSpace(target.Description) == "" {
					target.Description = result.Description
				}
				if len(target.ArtifactIDs) == 0 && len(result.ArtifactIDs) > 0 {
					target.ArtifactIDs = result.ArtifactIDs
				}
			}
			if len(unmerged) == 0 {
				continue
			}
			message.ToolUses = unmerged
		}

		merged = append(merged, message)
		messageIndex := len(merged) - 1
		if strings.TrimSpace(message.Role) == "tool" {
			continue
		}
		for toolIndex, toolUse := range message.ToolUses {
			if id := strings.TrimSpace(toolUse.ID); id != "" {
				toolRefs[id] = toolRef{
					messageIndex: messageIndex,
					toolIndex:    toolIndex,
				}
			}
		}
	}
	return merged
}

func loadOpencodeMessagesFromDB(dbPath string, session model.Session, nativeID string) []model.ConversationMessage {
	db, err := openReadOnlySQLite(dbPath)
	if err != nil {
		return nil
	}
	defer db.Close()

	rows, err := db.Query(`
		SELECT
			m.id,
			IFNULL(m.time_created, 0),
			IFNULL(m.data, ''),
			IFNULL(p.data, ''),
			IFNULL(p.time_created, 0)
		FROM message m
		LEFT JOIN part p ON p.message_id = m.id
		WHERE m.session_id = ?
		ORDER BY m.time_created ASC, p.time_created ASC, p.id ASC
	`, nativeID)
	if err != nil {
		return nil
	}
	defer rows.Close()

	type messageAccumulator struct {
		id        string
		role      string
		createdAt time.Time
		parts     []string
	}

	order := make([]string, 0, 64)
	accumulators := make(map[string]*messageAccumulator)
	for rows.Next() {
		var (
			messageID     string
			messageTimeMS int64
			messageData   string
			partData      string
			partTimeMS    int64
		)
		if err := rows.Scan(&messageID, &messageTimeMS, &messageData, &partData, &partTimeMS); err != nil {
			continue
		}
		if strings.TrimSpace(messageID) == "" {
			continue
		}

		acc := accumulators[messageID]
		if acc == nil {
			acc = &messageAccumulator{
				id:        messageID,
				role:      parseOpencodeMessageRole(messageData),
				createdAt: unixMillis(messageTimeMS),
			}
			accumulators[messageID] = acc
			order = append(order, messageID)
		}
		if acc.createdAt.IsZero() {
			acc.createdAt = unixMillis(messageTimeMS)
		}
		if text := parseOpencodePartText(partData); text != "" {
			acc.parts = append(acc.parts, text)
			if acc.createdAt.IsZero() {
				acc.createdAt = unixMillis(partTimeMS)
			}
		}
	}

	out := make([]model.ConversationMessage, 0, len(order))
	for _, messageID := range order {
		acc := accumulators[messageID]
		if acc == nil {
			continue
		}
		role := strings.TrimSpace(acc.role)
		if role != "user" && role != "assistant" {
			continue
		}
		text := strings.TrimSpace(strings.Join(acc.parts, "\n\n"))
		if text == "" {
			continue
		}
		out = append(out, newHistoryMessage(session, messageID, role, text, acc.createdAt))
	}
	return out
}

func parseOpencodeMessageRole(raw string) string {
	if strings.TrimSpace(raw) == "" {
		return ""
	}
	var row struct {
		Role string `json:"role"`
	}
	if err := json.Unmarshal([]byte(raw), &row); err != nil {
		return ""
	}
	return strings.TrimSpace(row.Role)
}

func parseOpencodePartText(raw string) string {
	if strings.TrimSpace(raw) == "" {
		return ""
	}
	var row struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	if err := json.Unmarshal([]byte(raw), &row); err != nil {
		return ""
	}
	if !strings.EqualFold(strings.TrimSpace(row.Type), "text") {
		return ""
	}
	return strings.TrimSpace(row.Text)
}

func queryOpencodeAssistantPreview(db *sql.DB, nativeID string) string {
	rows, err := db.Query(`
		SELECT
			IFNULL(m.data, ''),
			IFNULL(p.data, '')
		FROM message m
		LEFT JOIN part p ON p.message_id = m.id
		WHERE m.session_id = ?
		ORDER BY m.time_created DESC, p.time_created DESC, p.id DESC
		LIMIT 256
	`, nativeID)
	if err != nil {
		return ""
	}
	defer rows.Close()

	for rows.Next() {
		var (
			messageData string
			partData    string
		)
		if err := rows.Scan(&messageData, &partData); err != nil {
			continue
		}
		if !strings.EqualFold(parseOpencodeMessageRole(messageData), "assistant") {
			continue
		}
		if preview := summarizeHistoryText(parseOpencodePartText(partData)); preview != "" {
			return preview
		}
	}
	return ""
}

func parseClaudeMessageLine(line string, session model.Session) (model.ConversationMessage, bool) {
	var row struct {
		Type      string          `json:"type"`
		UUID      string          `json:"uuid"`
		Timestamp string          `json:"timestamp"`
		Message   json.RawMessage `json:"message"`
	}
	if err := json.Unmarshal([]byte(line), &row); err != nil {
		return model.ConversationMessage{}, false
	}

	role := ""
	text := ""
	var toolUses []model.ToolUse
	switch strings.TrimSpace(row.Type) {
	case "user":
		role, text, toolUses = extractClaudeUserMessage(row.Message)
	case "assistant":
		role = "assistant"
		text, toolUses = extractClaudeAssistantMessage(row.Message)
	default:
		return model.ConversationMessage{}, false
	}
	text = strings.TrimSpace(text)
	if text == "" && len(toolUses) == 0 {
		return model.ConversationMessage{}, false
	}

	messageID := strings.TrimSpace(row.UUID)
	if messageID == "" {
		messageID = stableID(
			"msg",
			session.Adapter,
			session.ConversationID,
			row.Timestamp,
			role,
			text,
			toolUseStableKey(toolUses),
		)
	}
	createdAt := parseTime(row.Timestamp)
	message := newHistoryMessage(session, messageID, role, text, createdAt)
	message.ToolUses = toolUses
	return message, true
}

func parseCodexMessageLine(line string, session model.Session) (model.ConversationMessage, bool) {
	var row struct {
		Timestamp string          `json:"timestamp"`
		Type      string          `json:"type"`
		Payload   json.RawMessage `json:"payload"`
	}
	if err := json.Unmarshal([]byte(line), &row); err != nil {
		return model.ConversationMessage{}, false
	}

	switch strings.TrimSpace(row.Type) {
	case "event_msg":
		var payload struct {
			Type             string   `json:"type"`
			Message          string   `json:"message"`
			CallID           string   `json:"call_id"`
			Command          []string `json:"command"`
			CWD              string   `json:"cwd"`
			AggregatedOutput string   `json:"aggregated_output"`
			ExitCode         *int     `json:"exit_code"`
			Status           string   `json:"status"`
		}
		if err := json.Unmarshal(row.Payload, &payload); err != nil {
			return model.ConversationMessage{}, false
		}
		switch strings.TrimSpace(payload.Type) {
		case "user_message":
			text := strings.TrimSpace(payload.Message)
			if text == "" {
				return model.ConversationMessage{}, false
			}
			createdAt := parseTime(row.Timestamp)
			return newHistoryMessage(session, stableID("msg", session.Adapter, session.ConversationID, row.Timestamp, "user", text), "user", text, createdAt), true
		case "exec_command_end":
			return codexToolResultMessage(
				session,
				row.Timestamp,
				payload.CallID,
				"exec_command",
				map[string]any{
					"command": strings.Join(payload.Command, " "),
					"cwd":     payload.CWD,
				},
				map[string]any{
					"output":    compactToolOutput(payload.AggregatedOutput),
					"exit_code": payload.ExitCode,
					"status":    payload.Status,
				},
				codexToolState(payload.ExitCode, payload.Status),
			)
		default:
			return model.ConversationMessage{}, false
		}
	case "response_item":
		var payload struct {
			Type      string `json:"type"`
			Role      string `json:"role"`
			Name      string `json:"name"`
			Arguments string `json:"arguments"`
			CallID    string `json:"call_id"`
			Output    string `json:"output"`
			Content   []struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"content"`
		}
		if err := json.Unmarshal(row.Payload, &payload); err != nil {
			return model.ConversationMessage{}, false
		}
		switch strings.TrimSpace(payload.Type) {
		case "message":
			if strings.TrimSpace(payload.Role) != "assistant" {
				return model.ConversationMessage{}, false
			}
			text := extractCodexAssistantText(payload.Content)
			if text == "" {
				return model.ConversationMessage{}, false
			}
			createdAt := parseTime(row.Timestamp)
			return newHistoryMessage(session, stableID("msg", session.Adapter, session.ConversationID, row.Timestamp, "assistant", text), "assistant", text, createdAt), true
		case "function_call":
			return codexToolCallMessage(
				session,
				row.Timestamp,
				payload.CallID,
				payload.Name,
				parseCodexToolArguments(payload.Arguments),
			)
		case "function_call_output":
			return codexToolResultMessage(
				session,
				row.Timestamp,
				payload.CallID,
				"Tool call",
				nil,
				map[string]any{"output": compactToolOutput(payload.Output)},
				"success",
			)
		default:
			return model.ConversationMessage{}, false
		}
	default:
		return model.ConversationMessage{}, false
	}
}

func codexToolCallMessage(session model.Session, timestamp, callID, name string, input map[string]any) (model.ConversationMessage, bool) {
	callID = strings.TrimSpace(callID)
	name = firstNonEmptyString(strings.TrimSpace(name), "Tool call")
	if callID == "" && name == "" {
		return model.ConversationMessage{}, false
	}
	createdAt := parseTime(timestamp)
	message := newHistoryMessage(
		session,
		stableID("msg", session.Adapter, session.ConversationID, timestamp, "tool_call", callID, name),
		"assistant",
		"",
		createdAt,
	)
	message.ToolUses = []model.ToolUse{{
		ID:          callID,
		Name:        name,
		State:       "running",
		Description: toolUseDescription(name, input),
		Input:       compactAnyMap(input),
		StartedAt:   createdAt,
	}}
	return message, true
}

func codexToolResultMessage(session model.Session, timestamp, callID, name string, input, output map[string]any, state string) (model.ConversationMessage, bool) {
	callID = strings.TrimSpace(callID)
	name = firstNonEmptyString(strings.TrimSpace(name), "Tool call")
	if callID == "" && len(output) == 0 {
		return model.ConversationMessage{}, false
	}
	createdAt := parseTime(timestamp)
	message := newHistoryMessage(
		session,
		stableID("msg", session.Adapter, session.ConversationID, timestamp, "tool_result", callID, name),
		"tool",
		"",
		createdAt,
	)
	message.ToolUses = []model.ToolUse{{
		ID:          callID,
		Name:        name,
		State:       firstNonEmptyString(strings.TrimSpace(state), "success"),
		Description: toolUseDescription(name, input),
		Input:       compactAnyMap(input),
		Output:      compactAnyMap(output),
		CompletedAt: createdAt,
	}}
	return message, true
}

func newHistoryMessage(session model.Session, id, role, text string, createdAt time.Time) model.ConversationMessage {
	return model.ConversationMessage{
		ID:             id,
		ConversationID: session.ConversationID,
		Role:           role,
		Content:        text,
		Status:         "complete",
		CreatedAt:      createdAt,
		UpdatedAt:      createdAt,
		Metadata: map[string]string{
			"adapter": session.Adapter,
			"source":  "native_history",
		},
	}
}

func extractCodexAssistantText(content []struct {
	Type string `json:"type"`
	Text string `json:"text"`
}) string {
	parts := make([]string, 0, len(content))
	for _, item := range content {
		switch strings.TrimSpace(item.Type) {
		case "output_text", "text":
			if text := strings.TrimSpace(item.Text); text != "" {
				parts = append(parts, text)
			}
		}
	}
	return strings.TrimSpace(strings.Join(parts, "\n\n"))
}

func parseCodexRolloutPreview(path string) string {
	lines := readTailLines(path, 2*1024*1024, 256)
	for idx := len(lines) - 1; idx >= 0; idx-- {
		line := lines[idx]
		var row struct {
			Type    string          `json:"type"`
			Payload json.RawMessage `json:"payload"`
		}
		if err := json.Unmarshal([]byte(line), &row); err != nil {
			continue
		}
		if row.Type != "response_item" {
			continue
		}

		var payload struct {
			Type    string `json:"type"`
			Role    string `json:"role"`
			Content []struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"content"`
		}
		if err := json.Unmarshal(row.Payload, &payload); err != nil {
			continue
		}
		if payload.Type != "message" || payload.Role != "assistant" {
			continue
		}

		parts := make([]string, 0, len(payload.Content))
		for _, item := range payload.Content {
			if strings.TrimSpace(item.Type) != "output_text" {
				continue
			}
			parts = append(parts, item.Text)
		}
		if preview := summarizeHistoryText(parts...); preview != "" {
			return preview
		}
	}
	return ""
}

func summarizeHistoryText(parts ...string) string {
	fragments := make([]string, 0, len(parts))
	for _, part := range parts {
		for _, line := range strings.Split(strings.ReplaceAll(part, "\r\n", "\n"), "\n") {
			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}
			fragments = append(fragments, strings.Join(strings.Fields(line), " "))
			if len(fragments) >= 2 {
				break
			}
		}
		if len(fragments) >= 2 {
			break
		}
	}
	if len(fragments) == 0 {
		return ""
	}

	text := strings.TrimSpace(strings.Join(fragments, " "))
	runes := []rune(text)
	if len(runes) <= 180 {
		return text
	}
	return strings.TrimSpace(string(runes[:177])) + "..."
}
