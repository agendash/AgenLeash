//go:build linux

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

	"github.com/creack/pty"
)

type PTYRuntime struct {
	mu          sync.RWMutex
	cmd         *exec.Cmd
	master      *os.File
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

func NewPTYRuntime() *PTYRuntime {
	return &PTYRuntime{
		state:  StatePending,
		events: make(chan Event, 128),
		done:   make(chan struct{}),
	}
}

func (r *PTYRuntime) Start(ctx context.Context, spec Spec) error {
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
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Setsid:  true,
		Setctty: true,
	}

	ws := &pty.Winsize{
		Rows: pickDimension(spec.Rows, 24),
		Cols: pickDimension(spec.Cols, 120),
	}
	master, err := pty.StartWithSize(cmd, ws)
	if err != nil {
		return err
	}

	now := time.Now().UTC()
	r.mu.Lock()
	r.cmd = cmd
	r.master = master
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

	r.wg.Add(2)
	go r.readLoop()
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

func (r *PTYRuntime) Write(p []byte) (int, error) {
	r.mu.RLock()
	master := r.master
	state := r.state
	r.mu.RUnlock()

	if master == nil {
		return 0, errors.New("runtime not started")
	}
	if state != StateRunning {
		return 0, fmt.Errorf("runtime not running: %s", state)
	}
	return master.Write(p)
}

func (r *PTYRuntime) Interrupt() error {
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

func (r *PTYRuntime) Stop() error {
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

func (r *PTYRuntime) Resize(rows, cols uint16) error {
	r.mu.RLock()
	master := r.master
	r.mu.RUnlock()
	if master == nil {
		return errors.New("runtime not started")
	}
	if err := pty.Setsize(master, &pty.Winsize{Rows: rows, Cols: cols}); err != nil {
		return err
	}
	r.emit(Event{Kind: EventKindResized, State: r.State(), Rows: rows, Cols: cols, Message: "runtime resized"})
	return nil
}

func (r *PTYRuntime) Wait() error {
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

func (r *PTYRuntime) Close() error {
	r.mu.Lock()
	if r.closed {
		r.mu.Unlock()
		return nil
	}
	r.closed = true
	cmd := r.cmd
	master := r.master
	r.mu.Unlock()

	if cmd != nil && cmd.Process != nil {
		_ = cmd.Process.Kill()
	}
	if master != nil {
		_ = master.Close()
	}
	return nil
}

func (r *PTYRuntime) PID() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if r.cmd == nil || r.cmd.Process == nil {
		return 0
	}
	return r.cmd.Process.Pid
}

func (r *PTYRuntime) State() State {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.state
}

func (r *PTYRuntime) Events() <-chan Event {
	return r.events
}

func (r *PTYRuntime) Snapshot() Snapshot {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.snapshot
}

func (r *PTYRuntime) emit(event Event) {
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

func (r *PTYRuntime) readLoop() {
	defer r.wg.Done()
	defer func() {
		_ = r.closeMaster()
	}()

	buf := make([]byte, 4096)
	for {
		r.mu.RLock()
		master := r.master
		r.mu.RUnlock()
		if master == nil {
			return
		}

		n, err := master.Read(buf)
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

func (r *PTYRuntime) waitLoop() {
	defer r.wg.Done()
	r.mu.RLock()
	cmd := r.cmd
	r.mu.RUnlock()
	if cmd == nil {
		return
	}

	waitErr := cmd.Wait()
	now := time.Now().UTC()
	r.mu.Lock()
	r.snapshot.UpdatedAt = now
	r.snapshot.ExitedAt = now
	r.snapshot.PID = cmd.Process.Pid
	stopped := r.stopped || r.interrupted
	r.mu.Unlock()

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

func (r *PTYRuntime) closeMaster() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.master != nil {
		_ = r.master.Close()
		r.master = nil
	}
	return nil
}

func pickDimension(value, fallback uint16) uint16 {
	if value == 0 {
		return fallback
	}
	return value
}
