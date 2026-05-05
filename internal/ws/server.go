package ws

import (
	"context"
	"errors"
	"io"
	"net/http"

	"github.com/agendash/AgenLeash/internal/event"
)

type SnapshotFunc func(string) (event.Event, bool)
type CommandHandler func(context.Context, string, event.Command) error

func ServeSessionEvents(ctx context.Context, w http.ResponseWriter, r *http.Request, sessionID string, hub *event.Hub, snapshot SnapshotFunc, handler CommandHandler) error {
	if hub == nil {
		return errors.New("event hub is required")
	}

	conn, err := Upgrade(w, r)
	if err != nil {
		return err
	}
	defer conn.Close()

	sub := hub.Subscribe(sessionID)
	defer sub.Close()

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	writeErr := make(chan error, 1)
	send := func(evt event.Event) {
		if err := conn.WriteJSON(evt); err != nil {
			select {
			case writeErr <- err:
			default:
			}
		}
	}

	if snapshot != nil {
		if evt, ok := snapshot(sessionID); ok {
			send(evt)
		}
	}
	for _, evt := range sub.Recent {
		send(evt)
	}
	send(event.SyncEnd(sessionID))

	go func() {
		defer cancel()
		for {
			_, payload, err := conn.ReadMessage()
			if err != nil {
				if errors.Is(err, io.EOF) {
					return
				}
				select {
				case writeErr <- err:
				default:
				}
				return
			}

			cmd, err := DecodeCommand(payload)
			if err != nil {
				continue
			}
			if handler != nil {
				if err := handler(ctx, sessionID, cmd); err != nil {
					select {
					case writeErr <- err:
					default:
					}
					return
				}
			}
		}
	}()

	for {
		select {
		case evt, ok := <-sub.C:
			if !ok {
				return nil
			}
			send(evt)
		case err := <-writeErr:
			if err != nil {
				return err
			}
		case <-ctx.Done():
			return nil
		}
	}
}
