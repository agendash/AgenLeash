package session

import (
	"fmt"
	"sync"

	"github.com/agendash/AgenLeash/internal/model"
)

type StateMachine struct {
	mu    sync.RWMutex
	state model.SessionState
}

func NewStateMachine(initial model.SessionState) *StateMachine {
	if initial == "" {
		initial = model.SessionStateUnknown
	}
	return &StateMachine{state: initial}
}

func (m *StateMachine) State() model.SessionState {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.state
}

func (m *StateMachine) Transition(next model.SessionState) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	return transitionLocked(&m.state, next)
}

func transitionLocked(current *model.SessionState, next model.SessionState) error {
	if next == "" {
		return fmt.Errorf("session state is required")
	}
	if *current == next {
		return nil
	}

	switch *current {
	case model.SessionStateUnknown:
		switch next {
		case model.SessionStatePending, model.SessionStateRunning, model.SessionStateStopped, model.SessionStateErrored:
			*current = next
			return nil
		}
	case model.SessionStatePending:
		switch next {
		case model.SessionStateRunning, model.SessionStateStopped, model.SessionStateErrored:
			*current = next
			return nil
		}
	case model.SessionStateRunning:
		switch next {
		case model.SessionStatePaused, model.SessionStateStopped, model.SessionStateErrored:
			*current = next
			return nil
		}
	case model.SessionStatePaused:
		switch next {
		case model.SessionStateRunning, model.SessionStateStopped, model.SessionStateErrored:
			*current = next
			return nil
		}
	case model.SessionStateStopped, model.SessionStateErrored:
		return fmt.Errorf("session is terminal in state %s", *current)
	}

	return fmt.Errorf("invalid transition from %s to %s", *current, next)
}
