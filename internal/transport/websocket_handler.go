package transport

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	log "github.com/kc1awv/m17-webclient/internal/logger"
	"github.com/kc1awv/m17-webclient/internal/status"
)

func newUpgrader(cfg WebSocketConfig) websocket.Upgrader {
	return websocket.Upgrader{
		CheckOrigin: func(r *http.Request) bool {
			origin := r.Header.Get("Origin")
			if origin == "" {
				return false
			}
			if origin == "http://"+r.Host || origin == "https://"+r.Host {
				return true
			}
			return cfg.OriginValidator(origin)
		},
	}
}

func HandleWebSocket(manager *SessionManager, cfg WebSocketConfig, w http.ResponseWriter, r *http.Request) {
	cfg.applyDefaults()
	ctx := r.Context()
	upgrader := newUpgrader(cfg)
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Error("WebSocket upgrade error", "err", err)
		return
	}
	conn.SetReadLimit(maxMessageSize)
	defer conn.Close()

	var writeMu sync.Mutex

	pingTicker := setupPingPong(conn, &writeMu, cfg.PingInterval, cfg.PongWait)
	defer pingTicker.Stop()

	var disconnectedOnce sync.Once
	sendDisconnected := func() {
		disconnectedOnce.Do(func() {
			if err := writeJSON(&writeMu, conn, ServerMessage{Type: "disconnected"}); err != nil {
				log.Warn("Error sending disconnected message", "err", err)
			}
		})
	}

	session, err := manager.AddSession()
	if err != nil {
		log.Warn("Session not accepted", "err", err)
		sendError(conn, &writeMu, err.Error())
		return
	}
	log.Info("New session connected", "session", session.ID)

	go func() {
		audioCh := session.OutgoingAudio
		msgCh := session.OutgoingMessages
		for audioCh != nil || msgCh != nil {
			select {
			case frame, ok := <-audioCh:
				if !ok {
					audioCh = nil
					continue
				}
				if err := writeMessage(&writeMu, conn, websocket.BinaryMessage, frame); err != nil {
					log.Warn("Error sending audio to browser", "session", session.ID, "err", err)
					return
				}
			case msg, ok := <-msgCh:
				if !ok {
					msgCh = nil
					continue
				}
				if err := writeJSON(&writeMu, conn, msg); err != nil {
					log.Warn("Error sending message to browser", "session", session.ID, "err", err)
					return
				}
			}
		}
	}()

	defer cleanup(session, manager)

	welcome := ServerMessage{
		Type: "welcome",
		Data: marshalData(WelcomeMessage{SessionID: session.ID, Server: cfg.ServerName}),
	}
	if err := writeJSON(&writeMu, conn, welcome); err != nil {
		log.Warn("Error sending welcome message", "session", session.ID, "err", err)
	}

	serveClient(ctx, session, conn, cfg, &writeMu, sendDisconnected)
}

func serveClient(ctx context.Context, session *Session, conn *websocket.Conn, cfg WebSocketConfig, mu *sync.Mutex, sendDisconnected func()) {
	done := make(chan struct{})
	go func() {
		select {
		case <-ctx.Done():
			conn.Close()
		case <-done:
		}
	}()
	defer close(done)

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		msgType, msg, err := conn.ReadMessage()
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			log.Warn("Read error", "session", session.ID, "err", err)
			break
		}

		if msgType == websocket.BinaryMessage {
			session.handleAudio(conn, mu, msg)
			continue
		}

		var clientMsg ClientMessage
		if err := json.Unmarshal(msg, &clientMsg); err != nil {
			errStr := fmt.Sprintf("Invalid JSON: %v", err)
			log.Warn("Invalid JSON", "session", session.ID, "err", err)
			sendError(conn, mu, errStr)
			continue
		}

		switch clientMsg.Type {
		case "ping":
			session.handlePing(conn, mu)
		case "join":
			session.handleJoin(ctx, conn, mu, clientMsg.Data, sendDisconnected, cfg.NewReflectorClient)
		case "ptt":
			session.handlePTT(conn, mu, clientMsg.Data)
		case "disconnect":
			session.handleDisconnect(conn, sendDisconnected)
		case "format":
			session.handleFormat(conn, mu, clientMsg.Data)
		default:
			session.handleUnknown(conn, mu, clientMsg.Type)
		}
	}
}

func setupPingPong(conn *websocket.Conn, mu *sync.Mutex, pingInterval, pongWait time.Duration) *time.Ticker {
	conn.SetReadDeadline(time.Now().Add(pongWait))
	conn.SetPongHandler(func(string) error {
		conn.SetReadDeadline(time.Now().Add(pongWait))
		return nil
	})

	pingTicker := time.NewTicker(pingInterval)
	go func() {
		for range pingTicker.C {
			mu.Lock()
			if err := conn.WriteControl(websocket.PingMessage, []byte{}, time.Now().Add(time.Second)); err != nil {
				mu.Unlock()
				log.Warn("Error sending ping", "err", err)
				return
			}
			mu.Unlock()
		}
	}()
	return pingTicker
}

func cleanup(session *Session, manager *SessionManager) {
	manager.RemoveSession(session.ID)
	status.RecordSessionEnded()
	log.Info("Session disconnected", "session", session.ID)
}
