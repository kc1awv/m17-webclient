package transport

import (
	"encoding/binary"
	"fmt"
	"sync"

	"github.com/gorilla/websocket"
	log "github.com/kc1awv/m17-webclient/internal/logger"
)

func (s *Session) handleAudio(conn *websocket.Conn, mu *sync.Mutex, msg []byte) {
	if s.Stream != nil {
		if s.UsePCM {
			s.handlePCM(conn, mu, msg)
		} else {
			s.handleG711(conn, mu, msg)
		}
	} else {
		errStr := fmt.Sprintf("Received audio but no active stream handler (session %s)", s.ID)
		log.Warn("Received audio but no active stream handler", "session", s.ID)
		sendError(conn, mu, errStr)
	}
}

func (s *Session) processAudioFrame(conn jsonWriter, mu *sync.Mutex, msg []byte, maxSize int, name string, decode func([]byte) error) {
	if len(msg) > maxSize {
		errStr := fmt.Sprintf("%s frame too large: %d", name, len(msg))
		log.Warn(name+" frame too large", "session", s.ID, "length", len(msg))
		sendError(conn, mu, errStr)
		return
	}
	if err := decode(msg); err != nil {
		errStr := fmt.Sprintf("Error handling %s frame: %v", name, err)
		log.Warn("Error handling "+name+" frame", "session", s.ID, "err", err)
		sendError(conn, mu, errStr)
	}
}

func (s *Session) handlePCM(conn jsonWriter, mu *sync.Mutex, msg []byte) {
	s.processAudioFrame(conn, mu, msg, maxPCMFrameSize, "PCM", func(b []byte) error {
		if len(b)%2 != 0 {
			return fmt.Errorf("Invalid PCM frame length: %d", len(b))
		}
		pcm := make([]int16, len(b)/2)
		for i := range pcm {
			pcm[i] = int16(binary.LittleEndian.Uint16(b[i*2:]))
		}
		return s.HandlePCMFrame(pcm, false)
	})
}

func (s *Session) handleG711(conn jsonWriter, mu *sync.Mutex, msg []byte) {
	s.processAudioFrame(conn, mu, msg, maxG711FrameSize, "G711", func(b []byte) error {
		return s.HandleG711Frame(b, false)
	})
}
