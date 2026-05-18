package runtime

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"sync"
	"time"
)

const PromptArgMode = "stdio_prompt_arg"

type PromptArgRuntime struct {
	mu       sync.RWMutex
	spec     Spec
	ctx      context.Context
	child    *STDIORuntime
	events   chan Event
	done     chan struct{}
	buffer   bytes.Buffer
	state    State
	snapshot Snapshot
	closed   bool
	launched bool
}

func NewPromptArgRuntime() *PromptArgRuntime {
	return &PromptArgRuntime{
		events: make(chan Event, 128),
		done:   make(chan struct{}),
		state:  StatePending,
	}
}

func (r *PromptArgRuntime) Start(ctx context.Context, spec Spec) error {
	if spec.Command == "" {
		return errors.New("command is required")
	}
	if ctx == nil {
		ctx = context.Background()
	}

	now := time.Now().UTC()
	r.mu.Lock()
	if r.state != StatePending {
		r.mu.Unlock()
		return errors.New("runtime already started")
	}
	r.ctx = ctx
	r.spec = spec
	r.state = StateRunning
	r.snapshot = Snapshot{
		State:     StateRunning,
		Command:   spec.Command,
		Args:      append([]string(nil), spec.Args...),
		Dir:       spec.Dir,
		StartedAt: now,
		UpdatedAt: now,
	}
	r.mu.Unlock()

	r.emit(Event{Kind: EventKindStarted, State: StateRunning, Message: "runtime waiting for prompt"})
	return nil
}

func (r *PromptArgRuntime) Write(p []byte) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.closed {
		return 0, errors.New("runtime input already closed")
	}
	if r.launched {
		return 0, errors.New("runtime prompt already submitted")
	}
	return r.buffer.Write(p)
}

func (r *PromptArgRuntime) CloseInput() error {
	r.mu.Lock()
	if r.closed {
		r.mu.Unlock()
		return errors.New("runtime input already closed")
	}
	r.closed = true
	if r.launched {
		r.mu.Unlock()
		return nil
	}
	prompt := strings.TrimRight(r.buffer.String(), "\r\n")
	spec := r.spec
	ctx := r.ctx
	r.launched = true
	r.mu.Unlock()

	spec.Args = substitutePromptArgs(spec.Args, prompt)
	child := NewSTDIORuntime()
	if err := child.Start(ctx, spec); err != nil {
		r.mu.Lock()
		r.state = StateErrored
		r.snapshot.State = StateErrored
		r.snapshot.UpdatedAt = time.Now().UTC()
		r.mu.Unlock()
		r.emit(Event{Kind: EventKindExited, State: StateErrored, Message: err.Error()})
		r.closeEvents()
		return err
	}

	r.mu.Lock()
	r.child = child
	r.snapshot = child.Snapshot()
	r.state = StateRunning
	r.mu.Unlock()

	go r.forwardChildEvents(child)
	return nil
}

func substitutePromptArgs(args []string, prompt string) []string {
	out := make([]string, 0, len(args)+1)
	replaced := false
	for _, arg := range args {
		next := strings.ReplaceAll(arg, "{prompt}", prompt)
		next = strings.ReplaceAll(next, "{AGENLEASH_PROMPT}", prompt)
		if next != arg {
			replaced = true
		}
		out = append(out, next)
	}
	if !replaced {
		out = append(out, prompt)
	}
	return out
}

func (r *PromptArgRuntime) forwardChildEvents(child *STDIORuntime) {
	for evt := range child.Events() {
		r.mu.Lock()
		r.snapshot = child.Snapshot()
		r.state = child.State()
		r.mu.Unlock()
		r.emit(evt)
	}
	r.closeEvents()
}

func (r *PromptArgRuntime) Interrupt() error {
	r.mu.RLock()
	child := r.child
	r.mu.RUnlock()
	if child == nil {
		return errors.New("runtime has not launched")
	}
	return child.Interrupt()
}

func (r *PromptArgRuntime) Stop() error {
	r.mu.RLock()
	child := r.child
	r.mu.RUnlock()
	if child != nil {
		return child.Stop()
	}
	r.mu.Lock()
	if r.state != StateStopped {
		r.state = StateStopped
		r.snapshot.State = StateStopped
		r.snapshot.ExitedAt = time.Now().UTC()
		r.snapshot.UpdatedAt = r.snapshot.ExitedAt
	}
	r.mu.Unlock()
	r.emit(Event{Kind: EventKindStopped, State: StateStopped, Message: "runtime stopped before prompt"})
	r.emit(Event{Kind: EventKindExited, State: StateStopped, Message: "runtime stopped before prompt"})
	r.closeEvents()
	return nil
}

func (r *PromptArgRuntime) Resize(uint16, uint16) error {
	return errors.New("stdio prompt-arg runtime does not support resize")
}

func (r *PromptArgRuntime) Wait() error {
	r.mu.RLock()
	done := r.done
	r.mu.RUnlock()
	if done == nil {
		return nil
	}
	<-done
	return nil
}

func (r *PromptArgRuntime) Close() error {
	r.mu.RLock()
	child := r.child
	r.mu.RUnlock()
	if child != nil {
		return child.Close()
	}
	return r.Stop()
}

func (r *PromptArgRuntime) PID() int {
	r.mu.RLock()
	child := r.child
	defer r.mu.RUnlock()
	if child != nil {
		return child.PID()
	}
	return r.snapshot.PID
}

func (r *PromptArgRuntime) State() State {
	r.mu.RLock()
	child := r.child
	defer r.mu.RUnlock()
	if child != nil {
		return child.State()
	}
	return r.state
}

func (r *PromptArgRuntime) Events() <-chan Event {
	return r.events
}

func (r *PromptArgRuntime) Snapshot() Snapshot {
	r.mu.RLock()
	child := r.child
	defer r.mu.RUnlock()
	if child != nil {
		return child.Snapshot()
	}
	return r.snapshot
}

func (r *PromptArgRuntime) emit(event Event) {
	event.At = time.Now().UTC()
	select {
	case r.events <- event:
	default:
	}
}

func (r *PromptArgRuntime) closeEvents() {
	r.mu.Lock()
	if r.closed && r.events == nil {
		r.mu.Unlock()
		return
	}
	events := r.events
	done := r.done
	r.events = nil
	r.done = nil
	r.mu.Unlock()

	if events != nil {
		close(events)
	}
	if done != nil {
		close(done)
	}
}
