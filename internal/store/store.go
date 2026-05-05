package store

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/agendash/AgenLeash/internal/model"
)

type Store interface {
	Snapshot() Catalog
	Replace(Catalog) error
	UpsertSession(model.Session) error
	UpsertWorkspace(model.Workspace) error
	UpsertConversation(model.Conversation) error
	DeleteSession(string) error
	DeleteWorkspace(string) error
	DeleteConversation(string) error
	GetSession(string) (model.Session, bool)
	ListSessions() []model.Session
	Close() error
}

type Catalog struct {
	Sessions      map[string]model.Session      `json:"sessions"`
	Workspaces    map[string]model.Workspace    `json:"workspaces"`
	Conversations map[string]model.Conversation `json:"conversations"`
	UpdatedAt     time.Time                     `json:"updated_at"`
}

func (c Catalog) normalize() Catalog {
	if c.Sessions == nil {
		c.Sessions = map[string]model.Session{}
	}
	if c.Workspaces == nil {
		c.Workspaces = map[string]model.Workspace{}
	}
	if c.Conversations == nil {
		c.Conversations = map[string]model.Conversation{}
	}
	return c
}

type MemoryStore struct {
	mu   sync.RWMutex
	data Catalog
}

func NewMemoryStore() *MemoryStore {
	return &MemoryStore{data: Catalog{}.normalize()}
}

func (s *MemoryStore) Snapshot() Catalog {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return cloneCatalog(s.data)
}

func (s *MemoryStore) Replace(c Catalog) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.data = cloneCatalog(c.normalize())
	s.data.UpdatedAt = time.Now().UTC()
	return nil
}

func (s *MemoryStore) UpsertSession(session model.Session) error {
	if session.ID == "" {
		return errors.New("session id is required")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	s.data.Sessions[session.ID] = session
	s.data.UpdatedAt = time.Now().UTC()
	return nil
}

func (s *MemoryStore) UpsertWorkspace(workspace model.Workspace) error {
	if workspace.ID == "" {
		return errors.New("workspace id is required")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	s.data.Workspaces[workspace.ID] = workspace
	s.data.UpdatedAt = time.Now().UTC()
	return nil
}

func (s *MemoryStore) UpsertConversation(conversation model.Conversation) error {
	if conversation.ID == "" {
		return errors.New("conversation id is required")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	s.data.Conversations[conversation.ID] = conversation
	s.data.UpdatedAt = time.Now().UTC()
	return nil
}

func (s *MemoryStore) DeleteSession(id string) error {
	if id == "" {
		return errors.New("session id is required")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.data.Sessions[id]; !ok {
		return nil
	}
	delete(s.data.Sessions, id)
	s.data.UpdatedAt = time.Now().UTC()
	return nil
}

func (s *MemoryStore) DeleteWorkspace(id string) error {
	if id == "" {
		return errors.New("workspace id is required")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.data.Workspaces[id]; !ok {
		return nil
	}
	delete(s.data.Workspaces, id)
	s.data.UpdatedAt = time.Now().UTC()
	return nil
}

func (s *MemoryStore) DeleteConversation(id string) error {
	if id == "" {
		return errors.New("conversation id is required")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.data.Conversations[id]; !ok {
		return nil
	}
	delete(s.data.Conversations, id)
	s.data.UpdatedAt = time.Now().UTC()
	return nil
}

func (s *MemoryStore) GetSession(id string) (model.Session, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	session, ok := s.data.Sessions[id]
	return session, ok
}

func (s *MemoryStore) ListSessions() []model.Session {
	s.mu.RLock()
	defer s.mu.RUnlock()

	sessions := make([]model.Session, 0, len(s.data.Sessions))
	for _, session := range s.data.Sessions {
		sessions = append(sessions, session)
	}
	return sessions
}

func (s *MemoryStore) Close() error {
	return nil
}

type FileStore struct {
	path string
	mem  *MemoryStore
	mu   sync.Mutex
}

func NewFileStore(path string) (*FileStore, error) {
	if path == "" {
		return nil, errors.New("path is required")
	}

	fs := &FileStore{
		path: path,
		mem:  NewMemoryStore(),
	}

	if err := fs.load(); err != nil {
		return nil, err
	}

	return fs, nil
}

func (s *FileStore) Snapshot() Catalog {
	return s.mem.Snapshot()
}

func (s *FileStore) Replace(c Catalog) error {
	if err := s.mem.Replace(c); err != nil {
		return err
	}
	return s.persist()
}

func (s *FileStore) UpsertSession(session model.Session) error {
	if err := s.mem.UpsertSession(session); err != nil {
		return err
	}
	return s.persist()
}

func (s *FileStore) UpsertWorkspace(workspace model.Workspace) error {
	if err := s.mem.UpsertWorkspace(workspace); err != nil {
		return err
	}
	return s.persist()
}

func (s *FileStore) UpsertConversation(conversation model.Conversation) error {
	if err := s.mem.UpsertConversation(conversation); err != nil {
		return err
	}
	return s.persist()
}

func (s *FileStore) DeleteSession(id string) error {
	if err := s.mem.DeleteSession(id); err != nil {
		return err
	}
	return s.persist()
}

func (s *FileStore) DeleteWorkspace(id string) error {
	if err := s.mem.DeleteWorkspace(id); err != nil {
		return err
	}
	return s.persist()
}

func (s *FileStore) DeleteConversation(id string) error {
	if err := s.mem.DeleteConversation(id); err != nil {
		return err
	}
	return s.persist()
}

func (s *FileStore) GetSession(id string) (model.Session, bool) {
	return s.mem.GetSession(id)
}

func (s *FileStore) ListSessions() []model.Session {
	return s.mem.ListSessions()
}

func (s *FileStore) Close() error {
	return s.persist()
}

func (s *FileStore) load() error {
	data, err := os.ReadFile(s.path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}

	var catalog Catalog
	if err := json.Unmarshal(data, &catalog); err != nil {
		return err
	}

	return s.mem.Replace(catalog)
}

func (s *FileStore) persist() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	catalog := s.mem.Snapshot().normalize()
	catalog.UpdatedAt = time.Now().UTC()

	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return err
	}

	payload, err := json.MarshalIndent(catalog, "", "  ")
	if err != nil {
		return err
	}

	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, payload, 0o644); err != nil {
		return err
	}

	return os.Rename(tmp, s.path)
}

func cloneCatalog(c Catalog) Catalog {
	c = c.normalize()

	sessions := make(map[string]model.Session, len(c.Sessions))
	for id, session := range c.Sessions {
		sessions[id] = session
		if session.Features != nil {
			session.Features = session.Features.Clone()
			sessions[id] = session
		}
	}

	workspaces := make(map[string]model.Workspace, len(c.Workspaces))
	for id, workspace := range c.Workspaces {
		workspaces[id] = workspace
	}

	conversations := make(map[string]model.Conversation, len(c.Conversations))
	for id, conversation := range c.Conversations {
		conversations[id] = conversation
	}

	return Catalog{
		Sessions:      sessions,
		Workspaces:    workspaces,
		Conversations: conversations,
		UpdatedAt:     c.UpdatedAt,
	}
}
