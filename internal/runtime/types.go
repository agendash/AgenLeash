package runtime

import (
	"context"
	"time"
)

type State string

const (
	StateUnknown State = "unknown"
	StatePending State = "pending"
	StateRunning State = "running"
	StateStopped State = "stopped"
	StateErrored State = "errored"
)

type EventKind string

const (
	EventKindStarted      EventKind = "started"
	EventKindOutput       EventKind = "output"
	EventKindStateChanged EventKind = "state_changed"
	EventKindInterrupted  EventKind = "interrupted"
	EventKindStopped      EventKind = "stopped"
	EventKindResized      EventKind = "resized"
	EventKindExited       EventKind = "exited"
	EventKindBuffered     EventKind = "buffered"
	EventKindSnapshot     EventKind = "snapshot"
)

type Spec struct {
	Mode    string
	Command string
	Args    []string
	Dir     string
	Env     []string
	Rows    uint16
	Cols    uint16
}

type Event struct {
	Sequence uint64    `json:"sequence"`
	At       time.Time `json:"at"`
	Kind     EventKind `json:"kind"`
	State    State     `json:"state,omitempty"`
	PID      int       `json:"pid,omitempty"`
	ExitCode int       `json:"exit_code,omitempty"`
	Signal   string    `json:"signal,omitempty"`
	Rows     uint16    `json:"rows,omitempty"`
	Cols     uint16    `json:"cols,omitempty"`
	Message  string    `json:"message,omitempty"`
	Data     []byte    `json:"data,omitempty"`
}

type Snapshot struct {
	PID       int       `json:"pid"`
	State     State     `json:"state"`
	Command   string    `json:"command,omitempty"`
	Args      []string  `json:"args,omitempty"`
	Dir       string    `json:"dir,omitempty"`
	StartedAt time.Time `json:"started_at,omitempty"`
	UpdatedAt time.Time `json:"updated_at,omitempty"`
	ExitedAt  time.Time `json:"exited_at,omitempty"`
	ExitCode  int       `json:"exit_code,omitempty"`
	Signal    string    `json:"signal,omitempty"`
}

type Runtime interface {
	Start(context.Context, Spec) error
	Write([]byte) (int, error)
	Interrupt() error
	Stop() error
	Resize(rows, cols uint16) error
	Wait() error
	Close() error
	PID() int
	State() State
	Events() <-chan Event
	Snapshot() Snapshot
}
