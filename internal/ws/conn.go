package ws

import (
	"bufio"
	"crypto/sha1"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"

	"github.com/agendash/AgenLeash/internal/event"
)

const (
	opcodeContinuation = 0x0
	opcodeText         = 0x1
	opcodeBinary       = 0x2
	opcodeClose        = 0x8
	opcodePing         = 0x9
	opcodePong         = 0xA
)

var websocketGUID = "258EAFA5-E914-47DA-95CA-C5AB0DC85B11"

type Conn struct {
	conn net.Conn
	r    *bufio.Reader
	w    *bufio.Writer
}

func Upgrade(w http.ResponseWriter, r *http.Request) (*Conn, error) {
	hj, ok := w.(http.Hijacker)
	if !ok {
		return nil, errors.New("response writer does not support hijacking")
	}

	key := strings.TrimSpace(r.Header.Get("Sec-WebSocket-Key"))
	if key == "" {
		return nil, errors.New("missing Sec-WebSocket-Key")
	}
	if !headerContainsToken(r.Header, "Connection", "upgrade") || !headerContainsToken(r.Header, "Upgrade", "websocket") {
		return nil, errors.New("invalid websocket upgrade request")
	}

	accept := computeAcceptKey(key)
	conn, rw, err := hj.Hijack()
	if err != nil {
		return nil, err
	}

	if _, err := fmt.Fprintf(rw, "HTTP/1.1 101 Switching Protocols\r\n"); err != nil {
		_ = conn.Close()
		return nil, err
	}
	if _, err := fmt.Fprintf(rw, "Upgrade: websocket\r\nConnection: Upgrade\r\nSec-WebSocket-Accept: %s\r\n\r\n", accept); err != nil {
		_ = conn.Close()
		return nil, err
	}
	if err := rw.Flush(); err != nil {
		_ = conn.Close()
		return nil, err
	}

	return &Conn{conn: conn, r: rw.Reader, w: rw.Writer}, nil
}

func computeAcceptKey(key string) string {
	sum := sha1.Sum([]byte(key + websocketGUID))
	return base64.StdEncoding.EncodeToString(sum[:])
}

func headerContainsToken(header http.Header, key, want string) bool {
	for _, part := range strings.Split(header.Get(key), ",") {
		if strings.EqualFold(strings.TrimSpace(part), want) {
			return true
		}
	}
	return false
}

func (c *Conn) Close() error {
	if c == nil || c.conn == nil {
		return nil
	}
	return c.conn.Close()
}

func (c *Conn) ReadJSON(v any) error {
	_, payload, err := c.ReadMessage()
	if err != nil {
		return err
	}
	return json.Unmarshal(payload, v)
}

func (c *Conn) ReadMessage() (byte, []byte, error) {
	for {
		fin, opcode, payload, err := c.readFrame()
		if err != nil {
			return 0, nil, err
		}

		switch opcode {
		case opcodeContinuation, opcodeText, opcodeBinary:
			if fin {
				return opcode, payload, nil
			}
		case opcodePing:
			if err := c.writeFrame(opcodePong, payload); err != nil {
				return 0, nil, err
			}
		case opcodeClose:
			return opcodeClose, payload, io.EOF
		case opcodePong:
			continue
		default:
			return 0, nil, fmt.Errorf("unsupported websocket opcode %d", opcode)
		}
	}
}

func (c *Conn) WriteJSON(v any) error {
	payload, err := json.Marshal(v)
	if err != nil {
		return err
	}
	return c.writeFrame(opcodeText, payload)
}

func (c *Conn) WriteText(payload []byte) error {
	return c.writeFrame(opcodeText, payload)
}

func (c *Conn) writeFrame(opcode byte, payload []byte) error {
	if c == nil || c.conn == nil {
		return errors.New("websocket connection is closed")
	}

	header := []byte{0x80 | opcode}
	length := len(payload)
	switch {
	case length < 126:
		header = append(header, byte(length))
	case length <= 0xffff:
		header = append(header, 126, 0, 0)
		binary.BigEndian.PutUint16(header[len(header)-2:], uint16(length))
	default:
		header = append(header, 127, 0, 0, 0, 0, 0, 0, 0, 0)
		binary.BigEndian.PutUint64(header[len(header)-8:], uint64(length))
	}

	if _, err := c.w.Write(header); err != nil {
		return err
	}
	if _, err := c.w.Write(payload); err != nil {
		return err
	}
	return c.w.Flush()
}

func (c *Conn) readFrame() (bool, byte, []byte, error) {
	header := make([]byte, 2)
	if _, err := io.ReadFull(c.r, header); err != nil {
		return false, 0, nil, err
	}

	fin := header[0]&0x80 != 0
	opcode := header[0] & 0x0f
	masked := header[1]&0x80 != 0
	length := int64(header[1] & 0x7f)
	switch length {
	case 126:
		var ext [2]byte
		if _, err := io.ReadFull(c.r, ext[:]); err != nil {
			return false, 0, nil, err
		}
		length = int64(binary.BigEndian.Uint16(ext[:]))
	case 127:
		var ext [8]byte
		if _, err := io.ReadFull(c.r, ext[:]); err != nil {
			return false, 0, nil, err
		}
		length = int64(binary.BigEndian.Uint64(ext[:]))
	}

	var mask [4]byte
	if masked {
		if _, err := io.ReadFull(c.r, mask[:]); err != nil {
			return false, 0, nil, err
		}
	}

	payload := make([]byte, int(length))
	if _, err := io.ReadFull(c.r, payload); err != nil {
		return false, 0, nil, err
	}
	if masked {
		for i := range payload {
			payload[i] ^= mask[i%4]
		}
	}

	return fin, opcode, payload, nil
}

func DecodeCommand(payload []byte) (event.Command, error) {
	var cmd event.Command
	return cmd, json.Unmarshal(payload, &cmd)
}
