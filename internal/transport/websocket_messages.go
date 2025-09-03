package transport

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"

	"github.com/gorilla/websocket"
	log "github.com/kc1awv/m17-webclient/internal/logger"
	"github.com/kc1awv/m17-webclient/internal/m17"
	"github.com/kc1awv/m17-webclient/internal/reflector"
	"github.com/kc1awv/m17-webclient/internal/status"
)

func (s *Session) handlePing(conn *websocket.Conn, mu *sync.Mutex) {
	if err := writeJSON(mu, conn, ServerMessage{Type: "pong"}); err != nil {
		log.Warn("Error sending pong", "session", s.ID, "err", err)
	}
}

func (s *Session) handleJoin(ctx context.Context, conn *websocket.Conn, mu *sync.Mutex, data json.RawMessage, sendDisconnected func(), newReflectorClient func(context.Context, string, string, byte) (*reflector.ReflectorClient, error)) {
	var payload struct {
		Callsign  string `json:"callsign"`
		Reflector string `json:"reflector"`
		Module    string `json:"module"`
	}
	if err := json.Unmarshal(data, &payload); err != nil {
		errStr := fmt.Sprintf("Invalid join payload: %v", err)
		log.Warn("Invalid join payload", "session", s.ID, "err", err)
		sendError(conn, mu, errStr)
		return
	}
	callsign, err := m17.ValidateCallsign(payload.Callsign)
	if err != nil {
		errStr := fmt.Sprintf("Invalid callsign: %v", err)
		log.Warn("Invalid callsign", "session", s.ID, "callsign", payload.Callsign, "err", err)
		sendError(conn, mu, errStr)
		return
	}
	s.Callsign = callsign
	moduleByte := byte('A')
	if len(payload.Module) > 0 {
		moduleByte = payload.Module[0]
	}
	if len(payload.Module) > 1 || moduleByte < 'A' || moduleByte > 'Z' {
		errStr := fmt.Sprintf("Invalid module: %s", payload.Module)
		log.Warn("Invalid module", "session", s.ID, "module", payload.Module)
		sendError(conn, mu, errStr)
		return
	}

	rc, err := newReflectorClient(ctx, payload.Reflector, s.Callsign, moduleByte)
	if err != nil {
		errStr := fmt.Sprintf("Failed to connect to reflector: %v", err)
		log.Warn("Failed to connect to reflector", "session", s.ID, "err", err)
		sendError(conn, mu, errStr)
		return
	}
	s.Reflector = rc
	go func() {
		<-rc.Done()
		sendDisconnected()
	}()
	go func() {
		for evt := range rc.Events {
			if evt == reflector.EventNACK {
				log.Warn("Session received NACK from reflector", "session", s.ID)
				select {
				case s.OutgoingMessages <- ServerMessage{Type: "nack"}:
				default:
				}
				sendDisconnected()
			}
		}
	}()
	if err := s.StartStreamHandler(); err != nil {
		errStr := fmt.Sprintf("Failed to start stream handler: %v", err)
		log.Warn("Failed to start stream handler", "session", s.ID, "err", err)
		sendError(conn, mu, errStr)
		return
	}
	log.Info("Session joined reflector",
		"session", s.ID,
		"reflector", payload.Reflector,
		"module", string(moduleByte),
		"callsign", s.Callsign,
	)
	status.RecordSessionStarted()

	joined := ServerMessage{
		Type: "joined",
		Data: marshalData(JoinedMessage{Reflector: payload.Reflector, Module: string(moduleByte), Callsign: s.Callsign}),
	}
	if err := writeJSON(mu, conn, joined); err != nil {
		log.Warn("Error sending joined message", "session", s.ID, "err", err)
	}
}

func (s *Session) handlePTT(conn *websocket.Conn, mu *sync.Mutex, data json.RawMessage) {
	var payload struct {
		Active bool `json:"active"`
	}
	if err := json.Unmarshal(data, &payload); err != nil {
		errStr := fmt.Sprintf("Invalid PTT payload: %v", err)
		log.Warn("Invalid PTT payload", "session", s.ID, "err", err)
		sendError(conn, mu, errStr)
		return
	}
	log.Info("Session PTT", "session", s.ID, "active", payload.Active)
	status.RecordPTT()
	if payload.Active && s.Stream != nil {
		if err := s.Stream.StartNewStream(); err != nil {
			log.Warn("failed to start new stream", "session", s.ID, "err", err)
		}
	}
	if !payload.Active && s.Stream != nil {
		if err := s.Stream.Finalize(); err != nil {
			log.Warn("failed to finalize stream", "session", s.ID, "err", err)
		}
	}
	resp := ServerMessage{
		Type: "ptt",
		Data: marshalData(PTTMessage{Active: payload.Active}),
	}
	if err := writeJSON(mu, conn, resp); err != nil {
		log.Warn("Error sending ptt message", "session", s.ID, "err", err)
	}
}

func (s *Session) handleDisconnect(_ *websocket.Conn, sendDisconnected func()) {
	log.Info("Session requested disconnect", "session", s.ID)
	if s.Reflector != nil {
		s.Reflector.Disconnect()
		s.Reflector = nil
	}
	sendDisconnected()
}

func (s *Session) handleFormat(conn *websocket.Conn, mu *sync.Mutex, data json.RawMessage) {
	var payload struct {
		Audio string `json:"audio"`
	}
	if err := json.Unmarshal(data, &payload); err != nil {
		errStr := fmt.Sprintf("Invalid format payload: %v", err)
		log.Warn("Invalid format payload", "session", s.ID, "err", err)
		sendError(conn, mu, errStr)
		return
	}
	format := strings.ToLower(payload.Audio)
	switch format {
	case "pcm":
		s.UsePCM = true
	case "g711":
		s.UsePCM = false
	default:
		errStr := fmt.Sprintf("Unknown audio format: %s", payload.Audio)
		log.Warn("Unknown audio format", "session", s.ID, "format", payload.Audio)
		sendError(conn, mu, errStr)
		return
	}
	resp := ServerMessage{
		Type: "format",
		Data: marshalData(FormatMessage{Audio: format}),
	}
	if err := writeJSON(mu, conn, resp); err != nil {
		log.Warn("Error sending format message", "session", s.ID, "err", err)
	}
}

func (s *Session) handleUnknown(conn *websocket.Conn, mu *sync.Mutex, msgType string) {
	errStr := fmt.Sprintf("Unknown message type: %s", msgType)
	log.Warn("Unknown message type", "session", s.ID, "type", msgType)
	sendError(conn, mu, errStr)
}
