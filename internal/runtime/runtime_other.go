//go:build !darwin && !linux

package runtime

import (
	"context"
	"errors"
)

type PTYRuntime struct{}

func NewPTYRuntime() *PTYRuntime {
	return &PTYRuntime{}
}

func (r *PTYRuntime) Start(context.Context, Spec) error {
	return errors.New("pty runtime is only implemented on darwin in this build")
}

func (r *PTYRuntime) Write([]byte) (int, error) {
	return 0, errors.New("pty runtime is only implemented on darwin in this build")
}

func (r *PTYRuntime) Interrupt() error {
	return errors.New("pty runtime is only implemented on darwin in this build")
}
func (r *PTYRuntime) Stop() error {
	return errors.New("pty runtime is only implemented on darwin in this build")
}
func (r *PTYRuntime) Resize(uint16, uint16) error {
	return errors.New("pty runtime is only implemented on darwin in this build")
}
func (r *PTYRuntime) Wait() error {
	return errors.New("pty runtime is only implemented on darwin in this build")
}
func (r *PTYRuntime) Close() error         { return nil }
func (r *PTYRuntime) PID() int             { return 0 }
func (r *PTYRuntime) State() State         { return StateUnknown }
func (r *PTYRuntime) Events() <-chan Event { return nil }
func (r *PTYRuntime) Snapshot() Snapshot   { return Snapshot{} }
