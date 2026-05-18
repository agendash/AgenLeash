package runtime

import (
	"bytes"
	"context"
	"testing"
	"time"
)

func TestPromptArgRuntimeLaunchesAfterCloseInput(t *testing.T) {
	rt := NewPromptArgRuntime()
	err := rt.Start(context.Background(), Spec{
		Mode:    PromptArgMode,
		Command: "/bin/sh",
		Args: []string{
			"-c",
			"printf '%s' \"$1\"",
			"sh",
			"{prompt}",
		},
	})
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	if _, err := rt.Write([]byte("hello from prompt\n")); err != nil {
		t.Fatalf("Write() error = %v", err)
	}
	if err := rt.CloseInput(); err != nil {
		t.Fatalf("CloseInput() error = %v", err)
	}

	var out bytes.Buffer
	events := rt.Events()
	deadline := time.After(2 * time.Second)
	for {
		select {
		case evt, ok := <-events:
			if !ok {
				if got := out.String(); got != "hello from prompt" {
					t.Fatalf("output = %q, want prompt without submit newline", got)
				}
				return
			}
			if evt.Kind == EventKindOutput {
				out.Write(evt.Data)
			}
		case <-deadline:
			t.Fatal("timeout waiting for runtime events")
		}
	}
}
