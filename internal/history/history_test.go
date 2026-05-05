package history

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite"

	"github.com/agendash/AgenLeash/internal/model"
)

func TestParseClaudeSessionFilePreview(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "claude.jsonl")
	body := `{"type":"assistant","cwd":"/tmp/work","gitBranch":"main","version":"2.1.39","message":{"role":"assistant","content":[{"type":"text","text":"Need your approval for the migration."}]}}
{"type":"assistant","cwd":"/tmp/work","gitBranch":"main","version":"2.1.39","message":{"role":"assistant","content":[{"type":"tool_use","name":"Bash"}]}}
`
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	meta := parseClaudeSessionFile(path)
	if meta.CWD != "/tmp/work" {
		t.Fatalf("cwd = %q, want /tmp/work", meta.CWD)
	}
	if meta.GitBranch != "main" {
		t.Fatalf("git branch = %q, want main", meta.GitBranch)
	}
	if meta.LastOutputPreview != "Need your approval for the migration." {
		t.Fatalf("preview = %q", meta.LastOutputPreview)
	}
}

func TestParseCodexRolloutPreview(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "rollout.jsonl")
	body := `{"type":"event_msg","payload":{"type":"token_count"}}
{"type":"response_item","payload":{"type":"message","role":"assistant","content":[{"type":"output_text","text":"Completed the sync and queued the next review."}]}}
`
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	preview := parseCodexRolloutPreview(path)
	if preview != "Completed the sync and queued the next review." {
		t.Fatalf("preview = %q", preview)
	}
}

func TestSummarizeHistoryTextTrim(t *testing.T) {
	t.Parallel()

	preview := summarizeHistoryText("Line one.\n\nLine two.", "Ignored")
	if preview != "Line one. Line two." {
		t.Fatalf("preview = %q", preview)
	}
}

func TestLoadClaudeMessages(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "claude.jsonl")
	body := `{"type":"user","uuid":"user-1","timestamp":"2026-04-20T02:15:44.254Z","message":{"role":"user","content":"Reply with exactly READY and nothing else."}}
{"type":"assistant","uuid":"assistant-think","timestamp":"2026-04-20T02:15:50.502Z","message":{"id":"msg_think","type":"message","role":"assistant","content":[{"type":"thinking","thinking":"internal"}]}}
{"type":"assistant","uuid":"assistant-1","timestamp":"2026-04-20T02:15:50.658Z","message":{"id":"msg_ready","type":"message","role":"assistant","content":[{"type":"text","text":"READY"}]}}
`
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	messages := LoadMessages(model.Session{
		Adapter:        "claudecode",
		ConversationID: "conv_claude",
		DetailPath:     path,
	})
	if len(messages) != 2 {
		t.Fatalf("messages = %d, want 2", len(messages))
	}
	if messages[0].Role != "user" || messages[0].Content != "Reply with exactly READY and nothing else." {
		t.Fatalf("first message = %#v", messages[0])
	}
	if messages[1].Role != "assistant" || messages[1].Content != "READY" {
		t.Fatalf("second message = %#v", messages[1])
	}
}

func TestLoadClaudeMessagesWithToolUses(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "claude-tools.jsonl")
	body := `{"type":"assistant","uuid":"assistant-tool","timestamp":"2026-04-20T02:16:00.000Z","message":{"role":"assistant","content":[{"type":"text","text":"I will inspect the repo."},{"type":"tool_use","id":"toolu_123","name":"Bash","input":{"command":"git status --short","description":"Check worktree"}}]}}
{"type":"user","uuid":"tool-result","timestamp":"2026-04-20T02:16:01.000Z","message":{"role":"user","content":[{"type":"tool_result","tool_use_id":"toolu_123","content":" M README.md","is_error":false}]}}
`
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	messages := LoadMessages(model.Session{
		Adapter:        "claudecode",
		ConversationID: "conv_claude",
		DetailPath:     path,
	})
	if len(messages) != 1 {
		t.Fatalf("messages = %d, want 1", len(messages))
	}
	if messages[0].Role != "assistant" || messages[0].Content != "I will inspect the repo." {
		t.Fatalf("assistant message = %#v", messages[0])
	}
	if len(messages[0].ToolUses) != 1 {
		t.Fatalf("assistant tool uses = %#v", messages[0].ToolUses)
	}
	if messages[0].ToolUses[0].ID != "toolu_123" || messages[0].ToolUses[0].Name != "Bash" {
		t.Fatalf("assistant tool use = %#v", messages[0].ToolUses[0])
	}
	if messages[0].ToolUses[0].Input["command"] != "git status --short" {
		t.Fatalf("tool input = %#v", messages[0].ToolUses[0].Input)
	}
	if messages[0].ToolUses[0].State != "success" || messages[0].ToolUses[0].Output["content"] != "M README.md" {
		t.Fatalf("tool result use = %#v", messages[0].ToolUses[0])
	}
}

func TestLoadClaudeMessagePage(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "claude.jsonl")
	body := `{"type":"user","uuid":"user-1","timestamp":"2026-04-20T02:15:44.254Z","message":{"role":"user","content":"first"}}
{"type":"assistant","uuid":"assistant-1","timestamp":"2026-04-20T02:15:45.254Z","message":{"role":"assistant","content":[{"type":"text","text":"FIRST"}]}}
{"type":"user","uuid":"user-2","timestamp":"2026-04-20T02:15:46.254Z","message":{"role":"user","content":"second"}}
{"type":"assistant","uuid":"assistant-2","timestamp":"2026-04-20T02:15:47.254Z","message":{"role":"assistant","content":[{"type":"text","text":"SECOND"}]}}
{"type":"user","uuid":"user-3","timestamp":"2026-04-20T02:15:48.254Z","message":{"role":"user","content":"third"}}
{"type":"assistant","uuid":"assistant-3","timestamp":"2026-04-20T02:15:49.254Z","message":{"role":"assistant","content":[{"type":"text","text":"THIRD"}]}}
`
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	page := LoadMessagePage(model.Session{
		Adapter:        "claudecode",
		ConversationID: "conv_claude",
		DetailPath:     path,
	}, MessagePageOptions{Limit: 2})
	if len(page.Messages) != 2 {
		t.Fatalf("messages = %d, want 2", len(page.Messages))
	}
	if page.Messages[0].Content != "third" || page.Messages[1].Content != "THIRD" {
		t.Fatalf("latest page = %#v", page.Messages)
	}
	if !page.HasMore || page.NextOffset != 2 {
		t.Fatalf("page info = %#v", page)
	}

	olderPage := LoadMessagePage(model.Session{
		Adapter:        "claudecode",
		ConversationID: "conv_claude",
		DetailPath:     path,
	}, MessagePageOptions{Limit: 2, Offset: 2})
	if len(olderPage.Messages) != 2 {
		t.Fatalf("older messages = %d, want 2", len(olderPage.Messages))
	}
	if olderPage.Messages[0].Content != "second" || olderPage.Messages[1].Content != "SECOND" {
		t.Fatalf("older page = %#v", olderPage.Messages)
	}
}

func TestLoadCodexMessages(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	rolloutPath := filepath.Join(dir, "rollout.jsonl")
	body := `{"timestamp":"2026-04-14T04:01:28.286Z","type":"event_msg","payload":{"type":"user_message","message":"Say only: smoke-ok","images":[],"local_images":[],"text_elements":[]}}
{"timestamp":"2026-04-14T04:01:30.141Z","type":"response_item","payload":{"type":"message","role":"assistant","content":[{"type":"output_text","text":"smoke-ok"}],"phase":"final_answer"}}
`
	if err := os.WriteFile(rolloutPath, []byte(body), 0o644); err != nil {
		t.Fatalf("write rollout: %v", err)
	}

	dbPath := filepath.Join(dir, "state.sqlite")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	defer db.Close()

	if _, err := db.Exec(`CREATE TABLE threads (id TEXT PRIMARY KEY, rollout_path TEXT NOT NULL);`); err != nil {
		t.Fatalf("create threads: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO threads(id, rollout_path) VALUES(?, ?)`, "thread-1", rolloutPath); err != nil {
		t.Fatalf("insert thread: %v", err)
	}

	messages := LoadMessages(model.Session{
		Adapter:        "codex",
		ConversationID: "conv_codex",
		DetailPath:     dbPath + "#threads/thread-1",
	})
	if len(messages) != 2 {
		t.Fatalf("messages = %d, want 2", len(messages))
	}
	if messages[0].Role != "user" || messages[0].Content != "Say only: smoke-ok" {
		t.Fatalf("first message = %#v", messages[0])
	}
	if messages[1].Role != "assistant" || messages[1].Content != "smoke-ok" {
		t.Fatalf("second message = %#v", messages[1])
	}
}

func TestLoadCodexMessagesWithToolUses(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	rolloutPath := filepath.Join(dir, "rollout-tools.jsonl")
	body := `{"timestamp":"2026-04-14T04:01:28.286Z","type":"response_item","payload":{"type":"function_call","name":"exec_command","arguments":"{\"cmd\":\"git status --short\",\"workdir\":\"/workspace/demo\"}","call_id":"call_123"}}
{"timestamp":"2026-04-14T04:01:28.500Z","type":"event_msg","payload":{"type":"exec_command_end","call_id":"call_123","command":["/bin/zsh","-lc","git status --short"],"cwd":"/workspace/demo","aggregated_output":" M README.md","exit_code":0,"status":"completed"}}
`
	if err := os.WriteFile(rolloutPath, []byte(body), 0o644); err != nil {
		t.Fatalf("write rollout: %v", err)
	}

	messages := LoadMessages(model.Session{
		Adapter:        "codex",
		ConversationID: "conv_codex",
		DetailPath:     rolloutPath,
	})
	if len(messages) != 1 {
		t.Fatalf("messages = %d, want 1", len(messages))
	}
	if messages[0].Role != "assistant" || messages[0].Content != "" {
		t.Fatalf("tool message = %#v", messages[0])
	}
	if len(messages[0].ToolUses) != 1 {
		t.Fatalf("tool uses = %#v", messages[0].ToolUses)
	}
	toolUse := messages[0].ToolUses[0]
	if toolUse.ID != "call_123" || toolUse.Name != "exec_command" {
		t.Fatalf("tool use = %#v", toolUse)
	}
	if toolUse.State != "success" {
		t.Fatalf("tool state = %q", toolUse.State)
	}
	if toolUse.Input["cmd"] != "git status --short" || toolUse.Output["output"] != "M README.md" {
		t.Fatalf("tool io = %#v / %#v", toolUse.Input, toolUse.Output)
	}
}

func TestLoadOpencodeMessages(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	dbPath := filepath.Join(dir, "opencode.db")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	defer db.Close()

	for _, stmt := range []string{
		`CREATE TABLE session (id TEXT PRIMARY KEY, directory TEXT NOT NULL, title TEXT NOT NULL, version TEXT NOT NULL, time_created INTEGER NOT NULL, time_updated INTEGER NOT NULL, time_archived INTEGER);`,
		`CREATE TABLE message (id TEXT PRIMARY KEY, session_id TEXT NOT NULL, time_created INTEGER NOT NULL, time_updated INTEGER NOT NULL, data TEXT NOT NULL);`,
		`CREATE TABLE part (id TEXT PRIMARY KEY, message_id TEXT NOT NULL, session_id TEXT NOT NULL, time_created INTEGER NOT NULL, time_updated INTEGER NOT NULL, data TEXT NOT NULL);`,
	} {
		if _, err := db.Exec(stmt); err != nil {
			t.Fatalf("exec %q: %v", stmt, err)
		}
	}

	if _, err := db.Exec(`INSERT INTO session(id, directory, title, version, time_created, time_updated, time_archived) VALUES(?, ?, ?, ?, ?, ?, 0)`,
		"ses_opencode_1",
		"/Users/tester/Workspace/demo",
		"Greeting",
		"1.14.20",
		int64(1776841045565),
		int64(1776841115517),
	); err != nil {
		t.Fatalf("insert session: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO message(id, session_id, time_created, time_updated, data) VALUES(?, ?, ?, ?, ?)`,
		"msg_user_1",
		"ses_opencode_1",
		int64(1776841045593),
		int64(1776841045593),
		`{"role":"user"}`,
	); err != nil {
		t.Fatalf("insert user message: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO part(id, message_id, session_id, time_created, time_updated, data) VALUES(?, ?, ?, ?, ?, ?)`,
		"prt_user_1",
		"msg_user_1",
		"ses_opencode_1",
		int64(1776841045594),
		int64(1776841045594),
		`{"type":"text","text":"hello opencode"}`,
	); err != nil {
		t.Fatalf("insert user part: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO message(id, session_id, time_created, time_updated, data) VALUES(?, ?, ?, ?, ?)`,
		"msg_assistant_1",
		"ses_opencode_1",
		int64(1776841045607),
		int64(1776841046174),
		`{"role":"assistant"}`,
	); err != nil {
		t.Fatalf("insert assistant message: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO part(id, message_id, session_id, time_created, time_updated, data) VALUES(?, ?, ?, ?, ?, ?)`,
		"prt_assistant_step",
		"msg_assistant_1",
		"ses_opencode_1",
		int64(1776841045608),
		int64(1776841045608),
		`{"type":"step-start"}`,
	); err != nil {
		t.Fatalf("insert assistant step part: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO part(id, message_id, session_id, time_created, time_updated, data) VALUES(?, ?, ?, ?, ?, ?)`,
		"prt_assistant_text",
		"msg_assistant_1",
		"ses_opencode_1",
		int64(1776841046172),
		int64(1776841046172),
		`{"type":"text","text":"OPENCODE-OK"}`,
	); err != nil {
		t.Fatalf("insert assistant text part: %v", err)
	}

	messages := LoadMessages(model.Session{
		Adapter:        "opencode",
		ConversationID: "conv_opencode",
		DetailPath:     dbPath + "#sessions/ses_opencode_1",
	})
	if len(messages) != 2 {
		t.Fatalf("messages = %d, want 2", len(messages))
	}
	if messages[0].Role != "user" || messages[0].Content != "hello opencode" {
		t.Fatalf("first message = %#v", messages[0])
	}
	if messages[1].Role != "assistant" || messages[1].Content != "OPENCODE-OK" {
		t.Fatalf("second message = %#v", messages[1])
	}
}

func TestDiscoverOpencodeRecords(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	dbPath := filepath.Join(root, "opencode.db")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	defer db.Close()

	for _, stmt := range []string{
		`CREATE TABLE session (id TEXT PRIMARY KEY, directory TEXT NOT NULL, title TEXT NOT NULL, version TEXT NOT NULL, parent_id TEXT, time_created INTEGER NOT NULL, time_updated INTEGER NOT NULL, time_archived INTEGER);`,
		`CREATE TABLE message (id TEXT PRIMARY KEY, session_id TEXT NOT NULL, time_created INTEGER NOT NULL, time_updated INTEGER NOT NULL, data TEXT NOT NULL);`,
		`CREATE TABLE part (id TEXT PRIMARY KEY, message_id TEXT NOT NULL, session_id TEXT NOT NULL, time_created INTEGER NOT NULL, time_updated INTEGER NOT NULL, data TEXT NOT NULL);`,
	} {
		if _, err := db.Exec(stmt); err != nil {
			t.Fatalf("exec %q: %v", stmt, err)
		}
	}

	if _, err := db.Exec(`INSERT INTO session(id, directory, title, version, parent_id, time_created, time_updated, time_archived) VALUES(?, ?, ?, ?, ?, ?, ?, 0)`,
		"ses_hist_1",
		"/Users/tester/Workspace/demo",
		"Demo Session",
		"1.14.20",
		"",
		int64(1776841045565),
		int64(1776841115517),
	); err != nil {
		t.Fatalf("insert session: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO message(id, session_id, time_created, time_updated, data) VALUES(?, ?, ?, ?, ?)`,
		"msg_assistant_1",
		"ses_hist_1",
		int64(1776841045607),
		int64(1776841046174),
		`{"role":"assistant"}`,
	); err != nil {
		t.Fatalf("insert assistant message: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO part(id, message_id, session_id, time_created, time_updated, data) VALUES(?, ?, ?, ?, ?, ?)`,
		"prt_assistant_text",
		"msg_assistant_1",
		"ses_hist_1",
		int64(1776841046172),
		int64(1776841046172),
		`{"type":"text","text":"review ready"}`,
	); err != nil {
		t.Fatalf("insert assistant part: %v", err)
	}

	records := DiscoverOpencodeRecords(context.Background(), root)
	if len(records) != 1 {
		t.Fatalf("records = %d, want 1", len(records))
	}
	record := records[0]
	if record.Session.Adapter != "opencode" {
		t.Fatalf("adapter = %q, want opencode", record.Session.Adapter)
	}
	if record.Session.NativeConversationID != "ses_hist_1" {
		t.Fatalf("native id = %q, want ses_hist_1", record.Session.NativeConversationID)
	}
	if record.Session.WorkspacePath != "/Users/tester/Workspace/demo" {
		t.Fatalf("workspace path = %q", record.Session.WorkspacePath)
	}
	if record.Session.LastOutputPreview != "review ready" {
		t.Fatalf("preview = %q, want review ready", record.Session.LastOutputPreview)
	}
	if record.Session.DetailPath != dbPath+"#sessions/ses_hist_1" {
		t.Fatalf("detail path = %q", record.Session.DetailPath)
	}
}

func TestDiscoverClaudeProjectFilesWithoutIndex(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	projectDir := filepath.Join(root, "projects", "-Company-agents-aprs-expert")
	if err := os.MkdirAll(projectDir, 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}

	sessionPath := filepath.Join(projectDir, "bc0e9bea-3108-4895-9d44-a5c056c28965.jsonl")
	body := `{"type":"user","timestamp":"2026-03-12T14:45:50.979Z","message":{"role":"user","content":"hello"},"cwd":"/Company/agents/aprs-expert","sessionId":"bc0e9bea-3108-4895-9d44-a5c056c28965","version":"2.1.74","gitBranch":"HEAD"}
{"type":"assistant","timestamp":"2026-03-12T14:45:54.259Z","message":{"role":"assistant","content":[{"type":"text","text":"ready for review"}]},"cwd":"/Company/agents/aprs-expert","sessionId":"bc0e9bea-3108-4895-9d44-a5c056c28965","version":"2.1.74","gitBranch":"HEAD"}
`
	if err := os.WriteFile(sessionPath, []byte(body), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	records := discoverClaude(context.Background(), root)
	if len(records) != 1 {
		t.Fatalf("records = %d, want 1", len(records))
	}
	record := records[0]
	if record.Session.NativeConversationID != "bc0e9bea-3108-4895-9d44-a5c056c28965" {
		t.Fatalf("native id = %q", record.Session.NativeConversationID)
	}
	if record.Session.WorkspacePath != "/Company/agents/aprs-expert" {
		t.Fatalf("workspace = %q", record.Session.WorkspacePath)
	}
	if record.Session.LastOutputPreview != "ready for review" {
		t.Fatalf("preview = %q", record.Session.LastOutputPreview)
	}
	if record.Session.DetailPath != sessionPath {
		t.Fatalf("detail path = %q, want %q", record.Session.DetailPath, sessionPath)
	}
}

func TestDiscoverCodexLegacyThreadTimestamps(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	rolloutDir := filepath.Join(root, "sessions", "2026", "03", "18")
	if err := os.MkdirAll(rolloutDir, 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	rolloutPath := filepath.Join(rolloutDir, "rollout-2026-03-18T00-42-04-thread-1.jsonl")
	rolloutBody := `{"type":"response_item","payload":{"type":"message","role":"assistant","content":[{"type":"output_text","text":"completed review sync"}]}}
`
	if err := os.WriteFile(rolloutPath, []byte(rolloutBody), 0o644); err != nil {
		t.Fatalf("WriteFile rollout error = %v", err)
	}

	dbPath := filepath.Join(root, "state_5.sqlite")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	defer db.Close()

	if _, err := db.Exec(`CREATE TABLE threads (
		id TEXT PRIMARY KEY,
		cwd TEXT,
		git_branch TEXT,
		cli_version TEXT,
		rollout_path TEXT,
		created_at INTEGER,
		updated_at INTEGER
	);`); err != nil {
		t.Fatalf("create threads: %v", err)
	}
	if _, err := db.Exec(
		`INSERT INTO threads(id, cwd, git_branch, cli_version, rollout_path, created_at, updated_at) VALUES(?, ?, ?, ?, ?, ?, ?)`,
		"thread-1",
		"/home/tester/Workspace/project",
		"main",
		"0.114.0",
		rolloutPath,
		int64(1710722524),
		int64(1710726124),
	); err != nil {
		t.Fatalf("insert thread: %v", err)
	}

	records := discoverCodex(context.Background(), root)
	if len(records) != 1 {
		t.Fatalf("records = %d, want 1", len(records))
	}
	record := records[0]
	if record.Session.WorkspacePath != "/home/tester/Workspace/project" {
		t.Fatalf("workspace = %q", record.Session.WorkspacePath)
	}
	if record.Session.LastOutputPreview != "completed review sync" {
		t.Fatalf("preview = %q", record.Session.LastOutputPreview)
	}
	if record.Session.CreatedAt.IsZero() || record.Session.LastSeen.IsZero() {
		t.Fatalf("timestamps should not be zero: %#v", record.Session)
	}
}

func TestDiscoverCodexTranslatesMountedRolloutPath(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	rolloutDir := filepath.Join(root, "sessions", "2026", "03", "18")
	if err := os.MkdirAll(rolloutDir, 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	rolloutPath := filepath.Join(rolloutDir, "rollout-2026-03-18T00-42-04-thread-1.jsonl")
	rolloutBody := `{"timestamp":"2026-04-14T04:01:28.286Z","type":"event_msg","payload":{"type":"user_message","message":"Say only: smoke-ok"}}
{"timestamp":"2026-04-14T04:01:30.141Z","type":"response_item","payload":{"type":"message","role":"assistant","content":[{"type":"output_text","text":"smoke-ok"}]}}
`
	if err := os.WriteFile(rolloutPath, []byte(rolloutBody), 0o644); err != nil {
		t.Fatalf("write rollout: %v", err)
	}

	dbPath := filepath.Join(root, "state_5.sqlite")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	defer db.Close()

	if _, err := db.Exec(`CREATE TABLE threads (
		id TEXT PRIMARY KEY,
		cwd TEXT,
		git_branch TEXT,
		cli_version TEXT,
		rollout_path TEXT,
		created_at INTEGER,
		updated_at INTEGER
	);`); err != nil {
		t.Fatalf("create threads: %v", err)
	}
	hostRolloutPath := "/home/tester/.codex/sessions/2026/03/18/rollout-2026-03-18T00-42-04-thread-1.jsonl"
	if _, err := db.Exec(
		`INSERT INTO threads(id, cwd, git_branch, cli_version, rollout_path, created_at, updated_at) VALUES(?, ?, ?, ?, ?, ?, ?)`,
		"thread-1",
		"/home/tester/Workspace/project",
		"main",
		"0.114.0",
		hostRolloutPath,
		int64(1710722524),
		int64(1710726124),
	); err != nil {
		t.Fatalf("insert thread: %v", err)
	}

	records := discoverCodex(context.Background(), root)
	if len(records) != 1 {
		t.Fatalf("records = %d, want 1", len(records))
	}
	if records[0].Session.LastOutputPreview != "smoke-ok" {
		t.Fatalf("preview = %q, want smoke-ok", records[0].Session.LastOutputPreview)
	}

	page := LoadMessagePage(model.Session{
		Adapter:        "codex",
		ConversationID: "conv_codex",
		DetailPath:     dbPath + "#threads/thread-1",
	}, MessagePageOptions{Limit: 2})
	if len(page.Messages) != 2 {
		t.Fatalf("messages = %d, want 2", len(page.Messages))
	}
	if page.Messages[0].Content != "Say only: smoke-ok" || page.Messages[1].Content != "smoke-ok" {
		t.Fatalf("messages = %#v", page.Messages)
	}
}
