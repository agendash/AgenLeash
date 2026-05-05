package session

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/agendash/AgenLeash/internal/model"
	"github.com/agendash/AgenLeash/internal/runtime"
)

type fakeRuntime struct {
	mu          sync.Mutex
	events      chan runtime.Event
	started     bool
	stopped     bool
	interrupted bool
	resized     []struct {
		rows uint16
		cols uint16
	}
}

func newFakeRuntime() *fakeRuntime {
	return &fakeRuntime{events: make(chan runtime.Event, 8)}
}

func (f *fakeRuntime) Start(context.Context, runtime.Spec) error {
	f.mu.Lock()
	f.started = true
	f.mu.Unlock()
	go func() {
		f.events <- runtime.Event{Kind: runtime.EventKindOutput, Data: []byte("fake output")}
		close(f.events)
	}()
	return nil
}

func (f *fakeRuntime) Write([]byte) (int, error) { return 0, nil }

func (f *fakeRuntime) Interrupt() error {
	f.mu.Lock()
	f.interrupted = true
	f.mu.Unlock()
	return nil
}

func (f *fakeRuntime) Stop() error {
	f.mu.Lock()
	f.stopped = true
	f.mu.Unlock()
	return nil
}

func (f *fakeRuntime) Resize(rows, cols uint16) error {
	f.mu.Lock()
	f.resized = append(f.resized, struct {
		rows uint16
		cols uint16
	}{rows: rows, cols: cols})
	f.mu.Unlock()
	return nil
}

func (f *fakeRuntime) Wait() error { return nil }

func (f *fakeRuntime) Close() error { return nil }

func (f *fakeRuntime) PID() int { return 1234 }

func (f *fakeRuntime) State() runtime.State { return runtime.StateRunning }

func (f *fakeRuntime) Events() <-chan runtime.Event { return f.events }

func (f *fakeRuntime) Snapshot() runtime.Snapshot {
	return runtime.Snapshot{PID: 1234, State: runtime.StateRunning}
}

func TestStateMachineTransitions(t *testing.T) {
	sm := NewStateMachine(model.SessionStatePending)
	if err := sm.Transition(model.SessionStateRunning); err != nil {
		t.Fatalf("transition to running failed: %v", err)
	}
	if err := sm.Transition(model.SessionStateStopped); err != nil {
		t.Fatalf("transition to stopped failed: %v", err)
	}
	if err := sm.Transition(model.SessionStateRunning); err == nil {
		t.Fatalf("expected terminal transition to fail")
	}
}

func TestManagerStartTracksSession(t *testing.T) {
	fake := newFakeRuntime()
	mgr := NewManagerWithFactory(8, func(runtime.Spec) (runtime.Runtime, error) { return fake, nil })

	sess, err := mgr.Start(context.Background(), StartRequest{
		ID:          "sess-1",
		Adapter:     "claudecode",
		RuntimeMode: "stdio",
		Runtime: runtime.Spec{
			Mode:    "stdio",
			Command: "demo",
		},
		Capabilities: model.Capabilities{SupportsInterrupt: true},
		Features:     model.FeatureSet{"streaming_text": true},
	})
	if err != nil {
		t.Fatalf("start failed: %v", err)
	}

	deadline := time.After(2 * time.Second)
	for {
		if seq := sess.LatestSequence(); seq >= 2 {
			break
		}
		select {
		case <-deadline:
			t.Fatalf("timed out waiting for session events")
		default:
			time.Sleep(10 * time.Millisecond)
		}
	}

	snapshot := sess.Snapshot()
	if snapshot.State != model.SessionStateRunning {
		t.Fatalf("expected running state, got %s", snapshot.State)
	}
	if snapshot.RuntimeMode != "stdio" {
		t.Fatalf("expected runtime mode stdio, got %q", snapshot.RuntimeMode)
	}
	if snapshot.ID != "sess-1" {
		t.Fatalf("unexpected session id: %s", snapshot.ID)
	}

	events, err := mgr.EventsSince("sess-1", 0)
	if err != nil {
		t.Fatalf("events since failed: %v", err)
	}
	if len(events) < 2 {
		t.Fatalf("expected buffered events, got %#v", events)
	}

	if err := mgr.Interrupt("sess-1"); err != nil {
		t.Fatalf("interrupt failed: %v", err)
	}
	if err := mgr.Stop("sess-1"); err != nil {
		t.Fatalf("stop failed: %v", err)
	}

	fake.mu.Lock()
	if !fake.interrupted {
		t.Fatalf("interrupt was not forwarded to runtime")
	}
	if !fake.stopped {
		t.Fatalf("stop was not forwarded to runtime")
	}
	fake.mu.Unlock()
}
