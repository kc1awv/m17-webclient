package transport

import (
	"encoding/json"
	"strings"
	"sync"
	"testing"
	"time"
)

type mockConn struct {
	msgs []ServerMessage
}

func (m *mockConn) WriteJSON(v interface{}) error {
	if msg, ok := v.(ServerMessage); ok {
		m.msgs = append(m.msgs, msg)
	}
	return nil
}

func (m *mockConn) SetWriteDeadline(time.Time) error { return nil }

func TestHandlePCMErrors(t *testing.T) {
	s := &Session{ID: "test"}
	mu := &sync.Mutex{}
	conn := &mockConn{}

	big := make([]byte, maxPCMFrameSize+2)
	s.handlePCM(conn, mu, big)
	if len(conn.msgs) == 0 || conn.msgs[0].Type != "error" {
		t.Fatalf("expected PCM frame too large error, got %#v", conn.msgs)
	}
	var errMsg ErrorMessage
	json.Unmarshal(conn.msgs[0].Data, &errMsg)
	if !strings.Contains(errMsg.Message, "PCM frame too large") {
		t.Fatalf("expected PCM frame too large error, got %#v", errMsg)
	}

	conn.msgs = nil
	odd := make([]byte, 3)
	s.handlePCM(conn, mu, odd)
	if len(conn.msgs) == 0 || conn.msgs[0].Type != "error" {
		t.Fatalf("expected invalid PCM frame length error, got %#v", conn.msgs)
	}
	json.Unmarshal(conn.msgs[0].Data, &errMsg)
	if !strings.Contains(errMsg.Message, "Invalid PCM frame length") {
		t.Fatalf("expected invalid PCM frame length error, got %#v", errMsg)
	}
}

func TestHandleG711Oversize(t *testing.T) {
	s := &Session{ID: "test"}
	mu := &sync.Mutex{}
	conn := &mockConn{}

	big := make([]byte, maxG711FrameSize+1)
	s.handleG711(conn, mu, big)
	if len(conn.msgs) == 0 || conn.msgs[0].Type != "error" {
		t.Fatalf("expected G711 frame too large error, got %#v", conn.msgs)
	}
	var errMsg ErrorMessage
	json.Unmarshal(conn.msgs[0].Data, &errMsg)
	if !strings.Contains(errMsg.Message, "G711 frame too large") {
		t.Fatalf("expected G711 frame too large error, got %#v", errMsg)
	}
}

func TestHandlePCMStreamError(t *testing.T) {
	s := &Session{ID: "test"}
	mu := &sync.Mutex{}
	conn := &mockConn{}

	msg := make([]byte, 4)
	s.handlePCM(conn, mu, msg)
	if len(conn.msgs) == 0 || conn.msgs[0].Type != "error" {
		t.Fatalf("expected no active stream handler error, got %#v", conn.msgs)
	}
	var errMsg ErrorMessage
	json.Unmarshal(conn.msgs[0].Data, &errMsg)
	if !strings.Contains(errMsg.Message, "no active stream handler") {
		t.Fatalf("expected no active stream handler error, got %#v", errMsg)
	}
}

func TestHandleG711StreamError(t *testing.T) {
	s := &Session{ID: "test"}
	mu := &sync.Mutex{}
	conn := &mockConn{}

	msg := make([]byte, 10)
	s.handleG711(conn, mu, msg)
	if len(conn.msgs) == 0 || conn.msgs[0].Type != "error" {
		t.Fatalf("expected no active stream handler error, got %#v", conn.msgs)
	}
	var errMsg ErrorMessage
	json.Unmarshal(conn.msgs[0].Data, &errMsg)
	if !strings.Contains(errMsg.Message, "no active stream handler") {
		t.Fatalf("expected no active stream handler error, got %#v", errMsg)
	}
}
