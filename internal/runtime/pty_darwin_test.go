//go:build darwin

package runtime

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestPTYRuntimeCapturesOutput(t *testing.T) {
	rt := NewPTYRuntime()
	if err := rt.Start(context.Background(), Spec{
		Command: "/bin/sh",
		Args:    []string{"-lc", "printf hello"},
	}); err != nil {
		t.Fatalf("start failed: %v", err)
	}

	if err := rt.Wait(); err != nil {
		t.Fatalf("wait failed: %v", err)
	}

	var sawOutput, sawExit bool
	deadline := time.After(2 * time.Second)
	for !sawOutput || !sawExit {
		select {
		case event, ok := <-rt.Events():
			if !ok {
				if !sawOutput {
					t.Fatalf("missing output event")
				}
				if !sawExit {
					t.Fatalf("missing exit event")
				}
				return
			}
			if event.Kind == EventKindOutput && strings.Contains(string(event.Data), "hello") {
				sawOutput = true
			}
			if event.Kind == EventKindExited && event.State == StateStopped {
				sawExit = true
			}
		case <-deadline:
			t.Fatalf("timed out waiting for PTY events")
		}
	}
}

func TestPTYRuntimeStop(t *testing.T) {
	rt := NewPTYRuntime()
	if err := rt.Start(context.Background(), Spec{
		Command: "/bin/sh",
		Args:    []string{"-lc", "trap 'exit 0' TERM; while :; do :; done"},
	}); err != nil {
		t.Fatalf("start failed: %v", err)
	}

	time.Sleep(100 * time.Millisecond)

	if err := rt.Stop(); err != nil {
		t.Fatalf("stop failed: %v", err)
	}
	if err := rt.Wait(); err != nil {
		t.Fatalf("wait failed: %v", err)
	}
	if got := rt.State(); got != StateStopped {
		t.Fatalf("expected stopped state, got %s", got)
	}
}
