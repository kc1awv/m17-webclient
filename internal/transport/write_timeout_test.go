package transport

import (
	"errors"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

type slowConn struct {
	deadline time.Time
}

func (c *slowConn) SetWriteDeadline(t time.Time) error {
	c.deadline = t
	return nil
}

func (c *slowConn) WriteJSON(v interface{}) error {
	time.Sleep(2 * writeTimeout)
	if time.Now().After(c.deadline) {
		return os.ErrDeadlineExceeded
	}
	return nil
}

func (c *slowConn) WriteMessage(messageType int, data []byte) error {
	time.Sleep(2 * writeTimeout)
	if time.Now().After(c.deadline) {
		return os.ErrDeadlineExceeded
	}
	return nil
}

func TestWriteJSONTimeout(t *testing.T) {
	oldTimeout := writeTimeout
	writeTimeout = 50 * time.Millisecond
	defer func() { writeTimeout = oldTimeout }()

	conn := &slowConn{}
	mu := &sync.Mutex{}
	start := time.Now()
	err := writeJSON(mu, conn, ServerMessage{Type: "test"})
	if !errors.Is(err, os.ErrDeadlineExceeded) {
		t.Fatalf("expected deadline exceeded, got %v", err)
	}
	if elapsed := time.Since(start); elapsed < writeTimeout || elapsed > 3*writeTimeout {
		t.Fatalf("writeJSON returned after %v, want around %v", elapsed, writeTimeout)
	}
}

func TestWriteMessageTimeout(t *testing.T) {
	oldTimeout := writeTimeout
	writeTimeout = 50 * time.Millisecond
	defer func() { writeTimeout = oldTimeout }()

	conn := &slowConn{}
	mu := &sync.Mutex{}
	start := time.Now()
	err := writeMessage(mu, conn, websocket.BinaryMessage, []byte{1})
	if !errors.Is(err, os.ErrDeadlineExceeded) {
		t.Fatalf("expected deadline exceeded, got %v", err)
	}
	if elapsed := time.Since(start); elapsed < writeTimeout || elapsed > 3*writeTimeout {
		t.Fatalf("writeMessage returned after %v, want around %v", elapsed, writeTimeout)
	}
}
