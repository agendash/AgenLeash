package runtime

import "testing"

func TestEventBufferRing(t *testing.T) {
	buf := NewEventBuffer(2)

	first := buf.Append(Event{Kind: EventKindOutput, Message: "one"})
	second := buf.Append(Event{Kind: EventKindOutput, Message: "two"})
	third := buf.Append(Event{Kind: EventKindOutput, Message: "three"})

	if first.Sequence != 1 || second.Sequence != 2 || third.Sequence != 3 {
		t.Fatalf("unexpected sequence numbers: %#v %#v %#v", first.Sequence, second.Sequence, third.Sequence)
	}

	snapshot := buf.Snapshot()
	if len(snapshot) != 2 {
		t.Fatalf("expected 2 events in snapshot, got %d", len(snapshot))
	}
	if snapshot[0].Message != "two" || snapshot[1].Message != "three" {
		t.Fatalf("unexpected snapshot order: %#v", snapshot)
	}

	since := buf.Since(2)
	if len(since) != 1 || since[0].Message != "three" {
		t.Fatalf("unexpected since results: %#v", since)
	}
}
