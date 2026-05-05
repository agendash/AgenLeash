package runtime

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sync"
	"syscall"
	"time"
)

type STDIORuntime struct {
	mu          sync.RWMutex
	cmd         *exec.Cmd
	stdin       io.WriteCloser
	events      chan Event
	state       State
	snapshot    Snapshot
	done        chan struct{}
	wg          sync.WaitGroup
	waitErr     error
	closed      bool
	interrupted bool
	stopped     bool
}

func NewSTDIORuntime() *STDIORuntime {
	return &STDIORuntime{
		state:  StatePending,
		events: make(chan Event, 128),
		done:   make(chan struct{}),
	}
}

func (r *STDIORuntime) Start(ctx context.Context, spec Spec) error {
	if spec.Command == "" {
		return errors.New("command is required")
	}

	r.mu.Lock()
	if r.cmd != nil {
		r.mu.Unlock()
		return errors.New("runtime already started")
	}
	r.mu.Unlock()

	cmd := exec.CommandContext(ctx, spec.Command, spec.Args...)
	cmd.Dir = spec.Dir
	cmd.Env = spec.Env

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		_ = stdin.Close()
		return err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		_ = stdin.Close()
		return err
	}

	if err := cmd.Start(); err != nil {
		_ = stdin.Close()
		return err
	}

	now := time.Now().UTC()
	r.mu.Lock()
	r.cmd = cmd
	r.stdin = stdin
	r.state = StateRunning
	r.snapshot = Snapshot{
		PID:       cmd.Process.Pid,
		State:     StateRunning,
		Command:   spec.Command,
		Args:      append([]string(nil), spec.Args...),
		Dir:       spec.Dir,
		StartedAt: now,
		UpdatedAt: now,
	}
	r.mu.Unlock()

	r.emit(Event{Kind: EventKindStarted, State: StateRunning, PID: cmd.Process.Pid, Message: "runtime started"})

	r.wg.Add(3)
	go r.readLoop(stdout)
	go r.readLoop(stderr)
	go r.waitLoop()
	go func() {
		r.wg.Wait()
		r.mu.Lock()
		if r.events != nil {
			close(r.events)
		}
		if r.done != nil {
			close(r.done)
		}
		r.mu.Unlock()
	}()

	return nil
}

func (r *STDIORuntime) Write(p []byte) (int, error) {
	r.mu.RLock()
	stdin := r.stdin
	state := r.state
	r.mu.RUnlock()

	if stdin == nil {
		return 0, errors.New("runtime not started")
	}
	if state != StateRunning {
		return 0, fmt.Errorf("runtime not running: %s", state)
	}
	return stdin.Write(p)
}

func (r *STDIORuntime) CloseInput() error {
	r.mu.Lock()
	stdin := r.stdin
	if stdin == nil {
		r.mu.Unlock()
		return errors.New("runtime input already closed")
	}
	r.stdin = nil
	r.mu.Unlock()
	return stdin.Close()
}

func (r *STDIORuntime) Interrupt() error {
	r.mu.RLock()
	cmd := r.cmd
	r.mu.RUnlock()
	if cmd == nil || cmd.Process == nil {
		return errors.New("runtime not started")
	}
	if err := cmd.Process.Signal(os.Interrupt); err != nil {
		return err
	}
	r.mu.Lock()
	r.interrupted = true
	r.mu.Unlock()
	r.emit(Event{Kind: EventKindInterrupted, State: r.State(), PID: cmd.Process.Pid, Message: "interrupt requested"})
	return nil
}

func (r *STDIORuntime) Stop() error {
	r.mu.RLock()
	cmd := r.cmd
	r.mu.RUnlock()
	if cmd == nil || cmd.Process == nil {
		return errors.New("runtime not started")
	}
	if err := cmd.Process.Signal(syscall.SIGTERM); err != nil {
		return err
	}
	r.mu.Lock()
	r.stopped = true
	r.mu.Unlock()
	r.emit(Event{Kind: EventKindStopped, State: r.State(), PID: cmd.Process.Pid, Message: "stop requested"})
	return nil
}

func (r *STDIORuntime) Resize(uint16, uint16) error {
	return errors.New("stdio runtime does not support resize")
}

func (r *STDIORuntime) Wait() error {
	r.mu.RLock()
	started := r.cmd != nil
	done := r.done
	r.mu.RUnlock()
	if !started {
		return errors.New("runtime not started")
	}
	if done == nil {
		return nil
	}
	<-done
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.waitErr
}

func (r *STDIORuntime) Close() error {
	r.mu.Lock()
	if r.closed {
		r.mu.Unlock()
		return nil
	}
	r.closed = true
	cmd := r.cmd
	stdin := r.stdin
	r.mu.Unlock()

	if stdin != nil {
		_ = stdin.Close()
	}
	if cmd != nil && cmd.Process != nil {
		_ = cmd.Process.Kill()
	}
	return nil
}

func (r *STDIORuntime) PID() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if r.cmd == nil || r.cmd.Process == nil {
		return 0
	}
	return r.cmd.Process.Pid
}

func (r *STDIORuntime) State() State {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.state
}

func (r *STDIORuntime) Events() <-chan Event {
	return r.events
}

func (r *STDIORuntime) Snapshot() Snapshot {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.snapshot
}

func (r *STDIORuntime) emit(event Event) {
	event = Event{
		Sequence: event.Sequence,
		At:       time.Now().UTC(),
		Kind:     event.Kind,
		State:    event.State,
		PID:      event.PID,
		ExitCode: event.ExitCode,
		Signal:   event.Signal,
		Rows:     event.Rows,
		Cols:     event.Cols,
		Message:  event.Message,
		Data:     append([]byte(nil), event.Data...),
	}

	r.mu.RLock()
	events := r.events
	r.mu.RUnlock()
	if events == nil {
		return
	}
	events <- event
}

func (r *STDIORuntime) readLoop(reader io.ReadCloser) {
	defer r.wg.Done()
	defer func() {
		_ = reader.Close()
	}()

	buf := make([]byte, 4096)
	for {
		n, err := reader.Read(buf)
		if n > 0 {
			r.emit(Event{Kind: EventKindOutput, State: r.State(), PID: r.PID(), Data: append([]byte(nil), buf[:n]...)})
		}
		if err != nil {
			if errors.Is(err, io.EOF) || errors.Is(err, os.ErrClosed) {
				return
			}
			return
		}
	}
}

func (r *STDIORuntime) waitLoop() {
	defer r.wg.Done()
	var waitErr error

	r.mu.RLock()
	cmd := r.cmd
	r.mu.RUnlock()
	if cmd == nil {
		return
	}

	waitErr = cmd.Wait()
	now := time.Now().UTC()

	r.mu.Lock()
	r.snapshot.UpdatedAt = now
	r.snapshot.ExitedAt = now
	r.snapshot.PID = cmd.Process.Pid
	stdin := r.stdin
	r.stdin = nil
	stopped := r.stopped || r.interrupted
	r.mu.Unlock()

	if stdin != nil {
		_ = stdin.Close()
	}

	if waitErr == nil || stopped {
		r.mu.Lock()
		r.waitErr = nil
		r.state = StateStopped
		r.snapshot.State = StateStopped
		r.mu.Unlock()
		r.emit(Event{Kind: EventKindExited, State: StateStopped, PID: cmd.Process.Pid, Message: "process exited"})
		return
	}

	r.mu.Lock()
	r.waitErr = waitErr
	r.state = StateErrored
	r.snapshot.State = StateErrored
	r.mu.Unlock()
	r.emit(Event{Kind: EventKindExited, State: StateErrored, PID: cmd.Process.Pid, Message: waitErr.Error()})
}
