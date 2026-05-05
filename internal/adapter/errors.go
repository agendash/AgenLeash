package adapter

import "fmt"

type InvalidSpecError struct {
	Reason string
}

func (e InvalidSpecError) Error() string {
	return fmt.Sprintf("invalid adapter spec: %s", e.Reason)
}

func ErrInvalidSpec(reason string) error {
	return InvalidSpecError{Reason: reason}
}
