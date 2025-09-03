package transport

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/gorilla/websocket"
)

func closedConn(t *testing.T) *websocket.Conn {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, _ := (&websocket.Upgrader{}).Upgrade(w, r, nil)
		c.Close()
	}))
	t.Cleanup(srv.Close)

	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http")
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial failed: %v", err)
	}
	return conn
}

func TestWriteJSONError(t *testing.T) {
	conn := closedConn(t)
	conn.Close()
	mu := &sync.Mutex{}
	err := writeJSON(mu, conn, ServerMessage{Type: "test"})
	if err == nil {
		t.Fatalf("expected error writing to closed connection")
	}
}

func TestHandlePingWriteError(t *testing.T) {
	conn := closedConn(t)
	defer conn.Close()
	s := &Session{ID: "test"}
	mu := &sync.Mutex{}
	s.handlePing(conn, mu)
}

func TestHandlePTTWriteError(t *testing.T) {
	conn := closedConn(t)
	defer conn.Close()
	s := &Session{ID: "test"}
	mu := &sync.Mutex{}
	payload := []byte(`{"active": true}`)
	s.handlePTT(conn, mu, payload)
}
