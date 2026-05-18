package session

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/agendash/AgenLeash/internal/model"
	"github.com/agendash/AgenLeash/internal/runtime"
)

type Manager struct {
	mu             sync.RWMutex
	sessions       map[string]*Session
	bufferSize     int
	runtimeFactory RuntimeFactory
}

func NewManager() *Manager {
	return &Manager{
		sessions:       map[string]*Session{},
		bufferSize:     256,
		runtimeFactory: defaultRuntimeFactory,
	}
}

func NewManagerWithFactory(bufferSize int, factory RuntimeFactory) *Manager {
	if bufferSize <= 0 {
		bufferSize = 256
	}
	if factory == nil {
		factory = defaultRuntimeFactory
	}
	return &Manager{
		sessions:       map[string]*Session{},
		bufferSize:     bufferSize,
		runtimeFactory: factory,
	}
}

func (m *Manager) Start(ctx context.Context, req StartRequest) (*Session, error) {
	rt, err := m.runtimeFactory(req.Runtime)
	if err != nil {
		return nil, err
	}
	if rt == nil {
		return nil, errors.New("runtime factory returned nil")
	}

	sess, err := New(ctx, req, rt, m.bufferSize)
	if err != nil {
		return nil, err
	}

	m.mu.Lock()
	m.sessions[sess.ID()] = sess
	m.mu.Unlock()

	return sess, nil
}

func defaultRuntimeFactory(spec runtime.Spec) (runtime.Runtime, error) {
	switch spec.Mode {
	case "", "pty":
		return runtime.NewPTYRuntime(), nil
	case "stdio":
		return runtime.NewSTDIORuntime(), nil
	case runtime.PromptArgMode:
		return runtime.NewPromptArgRuntime(), nil
	default:
		return nil, fmt.Errorf("unsupported runtime mode %q", spec.Mode)
	}
}

func (m *Manager) Get(id string) (*Session, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	sess, ok := m.sessions[id]
	return sess, ok
}

func (m *Manager) List() []model.Session {
	m.mu.RLock()
	defer m.mu.RUnlock()

	out := make([]model.Session, 0, len(m.sessions))
	for _, sess := range m.sessions {
		out = append(out, sess.Snapshot())
	}
	return out
}

func (m *Manager) Write(id string, data []byte) (int, error) {
	sess, ok := m.Get(id)
	if !ok {
		return 0, fmt.Errorf("session %s not found", id)
	}
	return sess.Write(data)
}

func (m *Manager) CloseInput(id string) error {
	sess, ok := m.Get(id)
	if !ok {
		return fmt.Errorf("session %s not found", id)
	}
	return sess.CloseInput()
}

func (m *Manager) Interrupt(id string) error {
	sess, ok := m.Get(id)
	if !ok {
		return fmt.Errorf("session %s not found", id)
	}
	return sess.Interrupt()
}

func (m *Manager) Stop(id string) error {
	sess, ok := m.Get(id)
	if !ok {
		return fmt.Errorf("session %s not found", id)
	}
	return sess.Stop()
}

func (m *Manager) Resize(id string, rows, cols uint16) error {
	sess, ok := m.Get(id)
	if !ok {
		return fmt.Errorf("session %s not found", id)
	}
	return sess.Resize(rows, cols)
}

func (m *Manager) EventsSince(id string, sequence uint64) ([]runtime.Event, error) {
	sess, ok := m.Get(id)
	if !ok {
		return nil, fmt.Errorf("session %s not found", id)
	}
	return sess.EventsSince(sequence), nil
}
