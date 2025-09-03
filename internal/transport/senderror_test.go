package transport

import (
	"encoding/json"
	"sync"
	"testing"
	"time"
)

type testJSONConn struct {
	deadline time.Time
	msg      interface{}
}

func (c *testJSONConn) SetWriteDeadline(t time.Time) error {
	c.deadline = t
	return nil
}

func (c *testJSONConn) WriteJSON(v interface{}) error {
	c.msg = v
	return nil
}

func TestSendErrorUsesWriteJSON(t *testing.T) {
	conn := &testJSONConn{}
	mu := &sync.Mutex{}
	sendError(conn, mu, "fail")
	if conn.msg == nil {
		t.Fatalf("expected message to be written")
	}
	if conn.deadline.IsZero() {
		t.Fatalf("expected deadline to be set")
	}
	sm, ok := conn.msg.(ServerMessage)
	if !ok {
		t.Fatalf("expected ServerMessage, got %T", conn.msg)
	}
	if sm.Type != "error" {
		t.Fatalf("expected type 'error', got %s", sm.Type)
	}
	var em ErrorMessage
	if err := json.Unmarshal(sm.Data, &em); err != nil {
		t.Fatalf("unmarshal error message: %v", err)
	}
	if em.Message != "fail" {
		t.Fatalf("expected message 'fail', got %s", em.Message)
	}
}
