package session

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/agendash/AgenLeash/internal/model"
	"github.com/agendash/AgenLeash/internal/runtime"
)

type RuntimeFactory func(runtime.Spec) (runtime.Runtime, error)

type StartRequest struct {
	ID                   string
	Adapter              string
	Origin               model.SessionOrigin
	RuntimeMode          string
	NativeConversationID string
	ConversationID       string
	WorkspaceID          string
	DetailPath           string
	StartMode            model.StartMode
	ResumeStrategy       model.ResumeStrategy
	WorkspacePath        string
	WorkspaceRoot        string
	WorkspaceFingerprint string
	GitRoot              string
	GitBranch            string
	Capabilities         model.Capabilities
	Features             model.FeatureSet
	Runtime              runtime.Spec
}

type Session struct {
	mu      sync.RWMutex
	model   model.Session
	runtime runtime.Runtime
	buffer  *runtime.EventBuffer
	state   *StateMachine
	events  chan runtime.Event
	closed  bool
}

func New(ctx context.Context, req StartRequest, rt runtime.Runtime, bufferSize int) (*Session, error) {
	if rt == nil {
		return nil, errors.New("runtime is required")
	}
	if req.ID == "" {
		req.ID = fmt.Sprintf("sess_%d", time.Now().UTC().UnixNano())
	}
	if req.Adapter == "" {
		return nil, errors.New("adapter is required")
	}
	if req.StartMode == "" {
		req.StartMode = model.StartModeNew
	}
	if req.Origin == "" {
		req.Origin = model.SessionOriginManaged
	}
	if req.ResumeStrategy == "" {
		req.ResumeStrategy = model.ResumeStrategyUnknown
	}
	if req.Features == nil {
		req.Features = model.FeatureSet{}
	}
	if bufferSize <= 0 {
		bufferSize = 256
	}

	now := time.Now().UTC()
	sess := &Session{
		model: model.Session{
			ID:                   req.ID,
			Adapter:              req.Adapter,
			Origin:               req.Origin,
			RuntimeMode:          req.RuntimeMode,
			NativeConversationID: req.NativeConversationID,
			ConversationID:       req.ConversationID,
			WorkspaceID:          req.WorkspaceID,
			DetailPath:           req.DetailPath,
			StartMode:            req.StartMode,
			ResumeStrategy:       req.ResumeStrategy,
			WorkspacePath:        req.WorkspacePath,
			WorkspaceRoot:        req.WorkspaceRoot,
			WorkspaceFingerprint: req.WorkspaceFingerprint,
			GitRoot:              req.GitRoot,
			GitBranch:            req.GitBranch,
			State:                model.SessionStatePending,
			Capabilities:         req.Capabilities,
			Features:             req.Features.Clone(),
			CreatedAt:            now,
			LastSeen:             now,
		},
		runtime: rt,
		buffer:  runtime.NewEventBuffer(bufferSize),
		state:   NewStateMachine(model.SessionStatePending),
		events:  make(chan runtime.Event, bufferSize),
	}

	if err := sess.runtime.Start(ctx, req.Runtime); err != nil {
		sess.model.State = model.SessionStateErrored
		_ = sess.state.Transition(model.SessionStateErrored)
		return nil, err
	}

	sess.model.State = model.SessionStateRunning
	sess.model.LastSeen = time.Now().UTC()
	_ = sess.state.Transition(model.SessionStateRunning)
	sess.record(runtime.Event{Kind: runtime.EventKindStateChanged, State: runtime.StateRunning, PID: sess.runtime.PID(), Message: "session started"})
	go sess.drainRuntimeEvents()
	return sess, nil
}

func (s *Session) drainRuntimeEvents() {
	defer func() {
		s.mu.Lock()
		if !s.closed {
			close(s.events)
			s.closed = true
		}
		s.mu.Unlock()
	}()
	for event := range s.runtime.Events() {
		s.record(event)
		s.touch()
		if event.Kind == runtime.EventKindExited {
			switch event.State {
			case runtime.StateStopped:
				_ = s.setState(model.SessionStateStopped, "runtime exited")
			case runtime.StateErrored:
				_ = s.setState(model.SessionStateErrored, "runtime failed")
			}
		}
	}
}

func (s *Session) record(event runtime.Event) runtime.Event {
	recorded := s.buffer.Append(event)
	s.mu.RLock()
	closed := s.closed
	events := s.events
	s.mu.RUnlock()
	if !closed {
		select {
		case events <- recorded:
		default:
		}
	}
	s.touch()
	return recorded
}

func (s *Session) touch() {
	s.mu.Lock()
	s.model.LastSeen = time.Now().UTC()
	s.mu.Unlock()
}

func (s *Session) setState(next model.SessionState, message string) error {
	if err := s.state.Transition(next); err != nil {
		return err
	}

	s.mu.Lock()
	s.model.State = next
	s.model.LastSeen = time.Now().UTC()
	pid := s.runtime.PID()
	s.mu.Unlock()

	s.record(runtime.Event{
		Kind:    runtime.EventKindStateChanged,
		State:   runtimeState(next),
		PID:     pid,
		Message: message,
	})
	return nil
}

func runtimeState(state model.SessionState) runtime.State {
	switch state {
	case model.SessionStatePending:
		return runtime.StatePending
	case model.SessionStateRunning:
		return runtime.StateRunning
	case model.SessionStateStopped:
		return runtime.StateStopped
	case model.SessionStateErrored:
		return runtime.StateErrored
	default:
		return runtime.StateUnknown
	}
}

func (s *Session) Snapshot() model.Session {
	s.mu.RLock()
	defer s.mu.RUnlock()

	out := s.model
	out.Features = s.model.Features.Clone()
	return out
}

func (s *Session) ID() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.model.ID
}

func (s *Session) EventsSince(sequence uint64) []runtime.Event {
	return s.buffer.Since(sequence)
}

func (s *Session) LatestSequence() uint64 {
	return s.buffer.LatestSequence()
}

func (s *Session) Write(p []byte) (int, error) {
	return s.runtime.Write(p)
}

func (s *Session) CloseInput() error {
	closer, ok := s.runtime.(interface{ CloseInput() error })
	if !ok {
		return errors.New("runtime does not support closing stdin")
	}
	return closer.CloseInput()
}

func (s *Session) Interrupt() error {
	if err := s.runtime.Interrupt(); err != nil {
		return err
	}
	s.record(runtime.Event{Kind: runtime.EventKindInterrupted, State: runtime.StateRunning, PID: s.runtime.PID(), Message: "interrupt sent"})
	return nil
}

func (s *Session) Stop() error {
	if err := s.runtime.Stop(); err != nil {
		return err
	}
	s.record(runtime.Event{Kind: runtime.EventKindStopped, State: runtime.StateRunning, PID: s.runtime.PID(), Message: "stop sent"})
	return nil
}

func (s *Session) Resize(rows, cols uint16) error {
	if err := s.runtime.Resize(rows, cols); err != nil {
		return err
	}
	s.mu.Lock()
	s.model.LastSeen = time.Now().UTC()
	s.mu.Unlock()
	return nil
}

func (s *Session) Pause(message string) bool {
	if s.State() != model.SessionStateRunning {
		return false
	}
	if err := s.setState(model.SessionStatePaused, message); err != nil {
		return false
	}
	return true
}

func (s *Session) Resume(message string) bool {
	if s.State() != model.SessionStatePaused {
		return false
	}
	if err := s.setState(model.SessionStateRunning, message); err != nil {
		return false
	}
	return true
}

func (s *Session) Wait() error {
	return s.runtime.Wait()
}

func (s *Session) Close() error {
	return s.runtime.Close()
}

func (s *Session) State() model.SessionState {
	return s.state.State()
}

func (s *Session) Events() <-chan runtime.Event {
	return s.events
}

func (s *Session) BindConversation(nativeID string, strategy model.ResumeStrategy) bool {
	trimmed := nativeID
	if trimmed == "" {
		return false
	}

	s.mu.Lock()
	changed := s.model.NativeConversationID != trimmed || (strategy != "" && s.model.ResumeStrategy != strategy)
	s.model.NativeConversationID = trimmed
	if strategy != "" {
		s.model.ResumeStrategy = strategy
	}
	s.model.LastSeen = time.Now().UTC()
	s.mu.Unlock()
	return changed
}

func (s *Session) SetLastOutputPreview(preview string) bool {
	trimmed := preview
	s.mu.Lock()
	changed := s.model.LastOutputPreview != trimmed
	if changed {
		s.model.LastOutputPreview = trimmed
		s.model.LastSeen = time.Now().UTC()
	}
	s.mu.Unlock()
	return changed
}
