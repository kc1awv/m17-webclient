package transport

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/kc1awv/m17-webclient/internal/cors"
	"github.com/kc1awv/m17-webclient/internal/reflector"
)

func newMockReflector(callsign string, module byte) *reflector.ReflectorClient {
	conn, _ := net.ListenUDP("udp", &net.UDPAddr{IP: net.IPv4zero, Port: 0})
	packets := make(chan []byte)
	events := make(chan reflector.Event)
	return reflector.NewTestClient(context.Background(), conn, &net.UDPAddr{IP: net.IPv4zero, Port: 0}, callsign, module, "TEST", packets, events)
}

func TestHandleWebSocketFlow(t *testing.T) {
	manager := NewSessionManager()
	cfg := WebSocketConfig{
		NewReflectorClient: func(ctx context.Context, addr, callsign string, module byte) (*reflector.ReflectorClient, error) {
			return newMockReflector(callsign, module), nil
		},
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		HandleWebSocket(manager, cfg, w, r)
	}))
	defer srv.Close()

	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http")
	hdr := http.Header{"Origin": {srv.URL}}
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, hdr)
	if err != nil {
		t.Fatalf("dial failed: %v", err)
	}
	defer conn.Close()

	var msg ServerMessage
	conn.SetReadDeadline(time.Now().Add(time.Second))
	if err := conn.ReadJSON(&msg); err != nil || msg.Type != "welcome" {
		t.Fatalf("expected welcome, got %v, err %v", msg, err)
	}

	joinPayload := map[string]string{"callsign": "TEST", "reflector": "127.0.0.1:17000", "module": "A"}
	jb, _ := json.Marshal(joinPayload)
	conn.WriteJSON(ClientMessage{Type: "join", Data: jb})
	conn.SetReadDeadline(time.Now().Add(time.Second))
	if err := conn.ReadJSON(&msg); err != nil || msg.Type != "joined" {
		t.Fatalf("expected joined, got %v, err %v", msg, err)
	}

	formatPayload := map[string]string{"audio": "pcm"}
	fb, _ := json.Marshal(formatPayload)
	conn.WriteJSON(ClientMessage{Type: "format", Data: fb})
	conn.SetReadDeadline(time.Now().Add(time.Second))
	if err := conn.ReadJSON(&msg); err != nil || msg.Type != "format" {
		t.Fatalf("expected format, got %v, err %v", msg, err)
	}

	pttPayload := map[string]bool{"active": true}
	pb, _ := json.Marshal(pttPayload)
	conn.WriteJSON(ClientMessage{Type: "ptt", Data: pb})
	conn.SetReadDeadline(time.Now().Add(time.Second))
	if err := conn.ReadJSON(&msg); err != nil || msg.Type != "ptt" {
		t.Fatalf("expected ptt, got %v, err %v", msg, err)
	}

	pttPayload["active"] = false
	pb, _ = json.Marshal(pttPayload)
	conn.WriteJSON(ClientMessage{Type: "ptt", Data: pb})
	conn.SetReadDeadline(time.Now().Add(time.Second))
	if err := conn.ReadJSON(&msg); err != nil || msg.Type != "ptt" {
		t.Fatalf("expected ptt false, got %v, err %v", msg, err)
	}

	conn.WriteJSON(ClientMessage{Type: "disconnect"})
	conn.SetReadDeadline(time.Now().Add(time.Second))
	if err := conn.ReadJSON(&msg); err != nil || msg.Type != "disconnected" {
		t.Fatalf("expected disconnected, got %v, err %v", msg, err)
	}

	conn.Close()
	for i := 0; i < 20 && manager.Count() != 0; i++ {
		time.Sleep(10 * time.Millisecond)
	}

	if manager.Count() != 0 {
		t.Fatalf("session not cleaned up")
	}
}

func TestHandleJoinModuleValidation(t *testing.T) {
	manager := NewSessionManager()
	cfg := WebSocketConfig{
		NewReflectorClient: func(ctx context.Context, addr, callsign string, module byte) (*reflector.ReflectorClient, error) {
			return newMockReflector(callsign, module), nil
		},
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		HandleWebSocket(manager, cfg, w, r)
	}))
	defer srv.Close()

	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http")
	hdr := http.Header{"Origin": {srv.URL}}

	tests := []struct {
		name   string
		module string
		expect string
	}{
		{"valid", "B", "joined"},
		{"invalid_lowercase", "b", "error"},
		{"invalid_length", "AB", "error"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			conn, _, err := websocket.DefaultDialer.Dial(wsURL, hdr)
			if err != nil {
				t.Fatalf("dial failed: %v", err)
			}
			defer conn.Close()

			var msg ServerMessage
			conn.SetReadDeadline(time.Now().Add(time.Second))
			if err := conn.ReadJSON(&msg); err != nil || msg.Type != "welcome" {
				t.Fatalf("expected welcome, got %v, err %v", msg, err)
			}

			joinPayload := map[string]string{"callsign": "TEST", "reflector": "127.0.0.1:17000", "module": tt.module}
			jb, _ := json.Marshal(joinPayload)
			conn.WriteJSON(ClientMessage{Type: "join", Data: jb})

			conn.SetReadDeadline(time.Now().Add(time.Second))
			if err := conn.ReadJSON(&msg); err != nil || msg.Type != tt.expect {
				t.Fatalf("expected %s, got %v, err %v", tt.expect, msg, err)
			}
		})
	}
}

func TestHandleWebSocketUnknown(t *testing.T) {
	manager := NewSessionManager()
	cfg := WebSocketConfig{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		HandleWebSocket(manager, cfg, w, r)
	}))
	defer srv.Close()

	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http")
	hdr := http.Header{"Origin": {srv.URL}}
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, hdr)
	if err != nil {
		t.Fatalf("dial failed: %v", err)
	}
	defer conn.Close()

	var msg ServerMessage
	conn.SetReadDeadline(time.Now().Add(time.Second))
	if err := conn.ReadJSON(&msg); err != nil {
		t.Fatalf("read welcome failed: %v", err)
	}
	if msg.Type != "welcome" {
		t.Fatalf("expected welcome, got %s", msg.Type)
	}

	conn.WriteJSON(ClientMessage{Type: "bogus"})
	conn.SetReadDeadline(time.Now().Add(time.Second))
	if err := conn.ReadJSON(&msg); err != nil || msg.Type != "error" {
		t.Fatalf("expected error, got %v, err %v", msg, err)
	}
}

func TestHandleWebSocketMaxSessions(t *testing.T) {
	manager := NewSessionManager()
	manager.MaxSessions = 1

	cfg := WebSocketConfig{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		HandleWebSocket(manager, cfg, w, r)
	}))
	defer srv.Close()

	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http")
	hdr := http.Header{"Origin": {srv.URL}}

	conn1, _, err := websocket.DefaultDialer.Dial(wsURL, hdr)
	if err != nil {
		t.Fatalf("dial1 failed: %v", err)
	}
	defer conn1.Close()

	var msg ServerMessage
	conn1.SetReadDeadline(time.Now().Add(time.Second))
	if err := conn1.ReadJSON(&msg); err != nil || msg.Type != "welcome" {
		t.Fatalf("expected welcome, got %v, err %v", msg, err)
	}

	conn2, _, err := websocket.DefaultDialer.Dial(wsURL, hdr)
	if err != nil {
		t.Fatalf("dial2 failed: %v", err)
	}
	defer conn2.Close()

	conn2.SetReadDeadline(time.Now().Add(time.Second))
	if err := conn2.ReadJSON(&msg); err != nil || msg.Type != "error" {
		t.Fatalf("expected error, got %v, err %v", msg, err)
	}
}

func TestHandleWebSocketContextCancel(t *testing.T) {
	manager := NewSessionManager()
	cfg := WebSocketConfig{}

	var cancel context.CancelFunc
	done := make(chan struct{})

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var ctx context.Context
		ctx, cancel = context.WithCancel(r.Context())
		r = r.WithContext(ctx)
		HandleWebSocket(manager, cfg, w, r)
		close(done)
	}))
	defer srv.Close()

	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http")
	hdr := http.Header{"Origin": {srv.URL}}
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, hdr)
	if err != nil {
		t.Fatalf("dial failed: %v", err)
	}
	defer conn.Close()

	var msg ServerMessage
	conn.SetReadDeadline(time.Now().Add(time.Second))
	if err := conn.ReadJSON(&msg); err != nil || msg.Type != "welcome" {
		t.Fatalf("expected welcome, got %v, err %v", msg, err)
	}

	cancel()

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatalf("handler did not exit after context cancel")
	}
}

func TestCheckOriginPatterns(t *testing.T) {
	tests := []struct {
		env    string
		origin string
		host   string
		allow  bool
	}{
		{"*", "https://evil.com", "example.com", true},
		{"https://*.example.com", "https://foo.example.com", "example.com", true},
		{"https://*.example.com", "https://bar.test.com", "example.com", false},
	}

	for _, tt := range tests {
		cfg := WebSocketConfig{OriginValidator: cors.NewOriginValidator(cors.ParseOriginRules(tt.env))}
		upg := newUpgrader(cfg)
		r := &http.Request{Header: http.Header{"Origin": []string{tt.origin}}, Host: tt.host}
		if got := upg.CheckOrigin(r); got != tt.allow {
			t.Errorf("CheckOrigin(%q) with env %q = %v; want %v", tt.origin, tt.env, got, tt.allow)
		}
	}
}

func TestHandleWebSocketRejectsOversizedPCMFrame(t *testing.T) {
	manager := NewSessionManager()

	cfg := WebSocketConfig{
		NewReflectorClient: func(ctx context.Context, addr, callsign string, module byte) (*reflector.ReflectorClient, error) {
			return newMockReflector(callsign, module), nil
		},
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		HandleWebSocket(manager, cfg, w, r)
	}))
	defer srv.Close()

	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http")
	hdr := http.Header{"Origin": {srv.URL}}
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, hdr)
	if err != nil {
		t.Fatalf("dial failed: %v", err)
	}
	defer conn.Close()

	var msg ServerMessage
	conn.SetReadDeadline(time.Now().Add(time.Second))
	if err := conn.ReadJSON(&msg); err != nil || msg.Type != "welcome" {
		t.Fatalf("expected welcome, got %v, err %v", msg, err)
	}

	joinPayload := map[string]string{"callsign": "TEST", "reflector": "127.0.0.1:17000", "module": "A"}
	jb, _ := json.Marshal(joinPayload)
	conn.WriteJSON(ClientMessage{Type: "join", Data: jb})
	conn.SetReadDeadline(time.Now().Add(time.Second))
	if err := conn.ReadJSON(&msg); err != nil || msg.Type != "joined" {
		t.Fatalf("expected joined, got %v, err %v", msg, err)
	}

	formatPayload := map[string]string{"audio": "pcm"}
	fb, _ := json.Marshal(formatPayload)
	conn.WriteJSON(ClientMessage{Type: "format", Data: fb})
	conn.SetReadDeadline(time.Now().Add(time.Second))
	if err := conn.ReadJSON(&msg); err != nil || msg.Type != "format" {
		t.Fatalf("expected format, got %v, err %v", msg, err)
	}

	big := make([]byte, maxPCMFrameSize+2)
	if err := conn.WriteMessage(websocket.BinaryMessage, big); err != nil {
		t.Fatalf("write failed: %v", err)
	}
	conn.SetReadDeadline(time.Now().Add(time.Second))
	if err := conn.ReadJSON(&msg); err != nil || msg.Type != "error" {
		t.Fatalf("expected error, got %v, err %v", msg, err)
	}
}

func TestHandleWebSocketRejectsOddPCMFrame(t *testing.T) {
	manager := NewSessionManager()

	cfg := WebSocketConfig{
		NewReflectorClient: func(ctx context.Context, addr, callsign string, module byte) (*reflector.ReflectorClient, error) {
			return newMockReflector(callsign, module), nil
		},
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		HandleWebSocket(manager, cfg, w, r)
	}))
	defer srv.Close()

	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http")
	hdr := http.Header{"Origin": {srv.URL}}
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, hdr)
	if err != nil {
		t.Fatalf("dial failed: %v", err)
	}
	defer conn.Close()

	var msg ServerMessage
	conn.SetReadDeadline(time.Now().Add(time.Second))
	if err := conn.ReadJSON(&msg); err != nil || msg.Type != "welcome" {
		t.Fatalf("expected welcome, got %v, err %v", msg, err)
	}

	joinPayload := map[string]string{"callsign": "TEST", "reflector": "127.0.0.1:17000", "module": "A"}
	jb, _ := json.Marshal(joinPayload)
	conn.WriteJSON(ClientMessage{Type: "join", Data: jb})
	conn.SetReadDeadline(time.Now().Add(time.Second))
	if err := conn.ReadJSON(&msg); err != nil || msg.Type != "joined" {
		t.Fatalf("expected joined, got %v, err %v", msg, err)
	}

	formatPayload := map[string]string{"audio": "pcm"}
	fb, _ := json.Marshal(formatPayload)
	conn.WriteJSON(ClientMessage{Type: "format", Data: fb})
	conn.SetReadDeadline(time.Now().Add(time.Second))
	if err := conn.ReadJSON(&msg); err != nil || msg.Type != "format" {
		t.Fatalf("expected format, got %v, err %v", msg, err)
	}

	odd := make([]byte, 3)
	if err := conn.WriteMessage(websocket.BinaryMessage, odd); err != nil {
		t.Fatalf("write failed: %v", err)
	}
	conn.SetReadDeadline(time.Now().Add(time.Second))
	if err := conn.ReadJSON(&msg); err != nil || msg.Type != "error" {
		t.Fatalf("expected error, got %v, err %v", msg, err)
	}

	var errMsg ErrorMessage
	if err := json.Unmarshal(msg.Data, &errMsg); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}
	if !strings.Contains(errMsg.Message, "Invalid PCM frame length") {
		t.Fatalf("expected invalid PCM frame length error, got %v", errMsg)
	}
}

func TestHandleWebSocketRejectsOversizedG711Frame(t *testing.T) {
	manager := NewSessionManager()

	cfg := WebSocketConfig{
		NewReflectorClient: func(ctx context.Context, addr, callsign string, module byte) (*reflector.ReflectorClient, error) {
			return newMockReflector(callsign, module), nil
		},
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		HandleWebSocket(manager, cfg, w, r)
	}))
	defer srv.Close()

	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http")
	hdr := http.Header{"Origin": {srv.URL}}
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, hdr)
	if err != nil {
		t.Fatalf("dial failed: %v", err)
	}
	defer conn.Close()

	var msg ServerMessage
	conn.SetReadDeadline(time.Now().Add(time.Second))
	if err := conn.ReadJSON(&msg); err != nil || msg.Type != "welcome" {
		t.Fatalf("expected welcome, got %v, err %v", msg, err)
	}

	joinPayload := map[string]string{"callsign": "TEST", "reflector": "127.0.0.1:17000", "module": "A"}
	jb, _ := json.Marshal(joinPayload)
	conn.WriteJSON(ClientMessage{Type: "join", Data: jb})
	conn.SetReadDeadline(time.Now().Add(time.Second))
	if err := conn.ReadJSON(&msg); err != nil || msg.Type != "joined" {
		t.Fatalf("expected joined, got %v, err %v", msg, err)
	}

	formatPayload := map[string]string{"audio": "g711"}
	fb, _ := json.Marshal(formatPayload)
	conn.WriteJSON(ClientMessage{Type: "format", Data: fb})
	conn.SetReadDeadline(time.Now().Add(time.Second))
	if err := conn.ReadJSON(&msg); err != nil || msg.Type != "format" {
		t.Fatalf("expected format, got %v, err %v", msg, err)
	}

	big := make([]byte, maxG711FrameSize+1)
	if err := conn.WriteMessage(websocket.BinaryMessage, big); err != nil {
		t.Fatalf("write failed: %v", err)
	}
	conn.SetReadDeadline(time.Now().Add(time.Second))
	if err := conn.ReadJSON(&msg); err != nil || msg.Type != "error" {
		t.Fatalf("expected error, got %v, err %v", msg, err)
	}
}

func TestHandleFormatValidAndCaseVariant(t *testing.T) {
	tests := []struct {
		input    string
		expected string
		usePCM   bool
	}{
		{"pcm", "pcm", true},
		{"g711", "g711", false},
		{"PCM", "pcm", true},
		{"G711", "g711", false},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			manager := NewSessionManager()
			cfg := WebSocketConfig{}
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				HandleWebSocket(manager, cfg, w, r)
			}))
			defer srv.Close()

			wsURL := "ws" + strings.TrimPrefix(srv.URL, "http")
			hdr := http.Header{"Origin": {srv.URL}}
			conn, _, err := websocket.DefaultDialer.Dial(wsURL, hdr)
			if err != nil {
				t.Fatalf("dial failed: %v", err)
			}
			defer conn.Close()

			var msg ServerMessage
			conn.SetReadDeadline(time.Now().Add(time.Second))
			if err := conn.ReadJSON(&msg); err != nil || msg.Type != "welcome" {
				t.Fatalf("expected welcome, got %v, err %v", msg, err)
			}
			var welcome WelcomeMessage
			if err := json.Unmarshal(msg.Data, &welcome); err != nil {
				t.Fatalf("unmarshal welcome: %v", err)
			}
			session := manager.GetSession(welcome.SessionID)
			if session == nil {
				t.Fatalf("session not found")
			}

			payload := map[string]string{"audio": tt.input}
			b, _ := json.Marshal(payload)
			conn.WriteJSON(ClientMessage{Type: "format", Data: b})

			conn.SetReadDeadline(time.Now().Add(time.Second))
			if err := conn.ReadJSON(&msg); err != nil || msg.Type != "format" {
				t.Fatalf("expected format, got %v, err %v", msg, err)
			}
			var resp FormatMessage
			if err := json.Unmarshal(msg.Data, &resp); err != nil {
				t.Fatalf("unmarshal format: %v", err)
			}
			if resp.Audio != tt.expected {
				t.Fatalf("audio = %q; want %q", resp.Audio, tt.expected)
			}
			if session.UsePCM != tt.usePCM {
				t.Fatalf("UsePCM = %v; want %v", session.UsePCM, tt.usePCM)
			}
		})
	}
}

func TestHandleFormatInvalid(t *testing.T) {
	manager := NewSessionManager()
	cfg := WebSocketConfig{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		HandleWebSocket(manager, cfg, w, r)
	}))
	defer srv.Close()

	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http")
	hdr := http.Header{"Origin": {srv.URL}}
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, hdr)
	if err != nil {
		t.Fatalf("dial failed: %v", err)
	}
	defer conn.Close()

	var msg ServerMessage
	conn.SetReadDeadline(time.Now().Add(time.Second))
	if err := conn.ReadJSON(&msg); err != nil || msg.Type != "welcome" {
		t.Fatalf("expected welcome, got %v, err %v", msg, err)
	}
	var welcome WelcomeMessage
	if err := json.Unmarshal(msg.Data, &welcome); err != nil {
		t.Fatalf("unmarshal welcome: %v", err)
	}
	session := manager.GetSession(welcome.SessionID)
	if session == nil {
		t.Fatalf("session not found")
	}

	session.UsePCM = true

	payload := map[string]string{"audio": "bogus"}
	b, _ := json.Marshal(payload)
	conn.WriteJSON(ClientMessage{Type: "format", Data: b})

	var resp ServerMessage
	conn.SetReadDeadline(time.Now().Add(time.Second))
	if err := conn.ReadJSON(&resp); err != nil || resp.Type != "error" {
		t.Fatalf("expected error, got %v, err %v", resp, err)
	}
	if !session.UsePCM {
		t.Fatalf("UsePCM changed on invalid format")
	}
}

func TestHandleWebSocketRejectsOversizedMessage(t *testing.T) {
	manager := NewSessionManager()
	cfg := WebSocketConfig{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		HandleWebSocket(manager, cfg, w, r)
	}))
	defer srv.Close()

	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http")
	hdr := http.Header{"Origin": {srv.URL}}
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, hdr)
	if err != nil {
		t.Fatalf("dial failed: %v", err)
	}
	defer conn.Close()

	var msg ServerMessage
	conn.SetReadDeadline(time.Now().Add(time.Second))
	if err := conn.ReadJSON(&msg); err != nil || msg.Type != "welcome" {
		t.Fatalf("expected welcome, got %v, err %v", msg, err)
	}

	big := make([]byte, maxMessageSize+1)
	if err := conn.WriteMessage(websocket.TextMessage, big); err != nil {
		t.Fatalf("write failed: %v", err)
	}

	conn.SetReadDeadline(time.Now().Add(time.Second))
	if _, _, err := conn.ReadMessage(); err == nil || !websocket.IsCloseError(err, websocket.CloseMessageTooBig) {
		t.Fatalf("expected close due to message too big, got %v", err)
	}
}
