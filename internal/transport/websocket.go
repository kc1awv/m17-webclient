package transport

import (
	"context"
	"encoding/json"
	"sync"
	"time"

	log "github.com/kc1awv/m17-webclient/internal/logger"
	"github.com/kc1awv/m17-webclient/internal/reflector"
)

type WebSocketConfig struct {
	OriginValidator    func(string) bool
	NewReflectorClient func(ctx context.Context, addr, callsign string, module byte) (*reflector.ReflectorClient, error)
	PingInterval       time.Duration
	PongWait           time.Duration
	ServerName         string
}

func (c *WebSocketConfig) applyDefaults() {
	if c.OriginValidator == nil {
		c.OriginValidator = func(string) bool { return false }
	}
	if c.NewReflectorClient == nil {
		c.NewReflectorClient = reflector.NewReflectorClient
	}
	if c.PingInterval <= 0 {
		c.PingInterval = defaultPingInterval
	}
	if c.PongWait <= 0 {
		c.PongWait = defaultPongWait
	}
}

type ClientMessage struct {
	Type string          `json:"type"`
	Data json.RawMessage `json:"data"`
}

type ServerMessage struct {
	Type string          `json:"type"`
	Data json.RawMessage `json:"data,omitempty"`
}

type WelcomeMessage struct {
	SessionID string `json:"session_id"`
	Server    string `json:"server"`
}

type JoinedMessage struct {
	Reflector string `json:"reflector"`
	Module    string `json:"module"`
	Callsign  string `json:"callsign"`
}

type PTTMessage struct {
	Active bool `json:"active"`
}

type FormatMessage struct {
	Audio string `json:"audio"`
}

type RxStatusMessage struct {
	Active bool   `json:"active"`
	Src    string `json:"src,omitempty"`
}

type ErrorMessage struct {
	Message string `json:"message"`
}

func marshalData(v interface{}) json.RawMessage {
	b, err := json.Marshal(v)
	if err != nil {
		log.Warn("Error marshaling message", "err", err)
		return nil
	}
	return b
}

type jsonWriter interface {
	SetWriteDeadline(time.Time) error
	WriteJSON(v interface{}) error
}

type messageWriter interface {
	SetWriteDeadline(time.Time) error
	WriteMessage(messageType int, data []byte) error
}

const (
	defaultPingInterval = 30 * time.Second
	defaultPongWait     = 60 * time.Second
	maxPCMFrameSize     = 640
	maxG711FrameSize    = 320
	maxMessageSize      = 64 * 1024
)

var writeTimeout = 5 * time.Second

func writeJSON(mu *sync.Mutex, conn jsonWriter, v any) error {
	mu.Lock()
	defer mu.Unlock()
	conn.SetWriteDeadline(time.Now().Add(writeTimeout))
	return conn.WriteJSON(v)
}

func writeMessage(mu *sync.Mutex, conn messageWriter, messageType int, data []byte) error {
	mu.Lock()
	defer mu.Unlock()
	conn.SetWriteDeadline(time.Now().Add(writeTimeout))
	return conn.WriteMessage(messageType, data)
}

func sendError(conn jsonWriter, mu *sync.Mutex, message string) {
	errMsg := ServerMessage{
		Type: "error",
		Data: marshalData(ErrorMessage{Message: message}),
	}
	if err := writeJSON(mu, conn, errMsg); err != nil {
		log.Warn("Error sending error message", "err", err)
	}
}
