package transport

import (
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/kc1awv/m17-webclient/internal/audio"
	log "github.com/kc1awv/m17-webclient/internal/logger"
	"github.com/kc1awv/m17-webclient/internal/m17"
	"github.com/kc1awv/m17-webclient/internal/reflector"
	"github.com/kc1awv/m17-webclient/internal/status"
)

const (
	OutgoingAudioBufSize    = 100
	OutgoingMessagesBufSize = 20
)

var reflectorTimeout = 2 * time.Second

type Session struct {
	ID        string
	Callsign  string
	Reflector *reflector.ReflectorClient
	Stream    *m17.StreamHandler

	OutgoingAudio    chan []byte
	OutgoingMessages chan ServerMessage
	UsePCM           bool
	pcmBuf           []int16

	streamStop chan struct{}
	streamWG   sync.WaitGroup
}

type SessionManager struct {
	sessions    map[string]*Session
	mu          sync.RWMutex
	MaxSessions int
}

func NewSessionManager() *SessionManager {
	return &SessionManager{
		sessions: make(map[string]*Session),
	}
}

func (sm *SessionManager) AddSession() (*Session, error) {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	if sm.MaxSessions > 0 && len(sm.sessions) >= sm.MaxSessions {
		return nil, fmt.Errorf("maximum sessions reached")
	}

	id := uuid.New().String()
	s := &Session{
		ID:               id,
		OutgoingAudio:    make(chan []byte, OutgoingAudioBufSize),
		OutgoingMessages: make(chan ServerMessage, OutgoingMessagesBufSize),
	}
	sm.sessions[id] = s
	return s, nil
}

func (sm *SessionManager) RemoveSession(id string) {
	sm.mu.Lock()
	s, ok := sm.sessions[id]
	if ok {
		delete(sm.sessions, id)
	}
	sm.mu.Unlock()

	if ok {
		if err := cleanupSession(s); err != nil {
			log.Warn("session cleanup failed", "session", id, "err", err)
		}
	}
}

func cleanupSession(s *Session) error {
	try := func(name string, fn func()) (err error) {
		defer func() {
			if r := recover(); r != nil {
				err = fmt.Errorf("%s: %v", name, r)
			}
		}()
		fn()
		return nil
	}

	var errs []error

	if s.Stream != nil {
		if err := try("StopStreamHandler", s.StopStreamHandler); err != nil {
			errs = append(errs, err)
		}
	}
	if s.Reflector != nil {
		if err := try("Reflector.Disconnect", s.Reflector.Disconnect); err != nil {
			errs = append(errs, err)
		}
	}
	if err := try("close OutgoingAudio", func() { close(s.OutgoingAudio) }); err != nil {
		errs = append(errs, err)
	}
	if err := try("close OutgoingMessages", func() { close(s.OutgoingMessages) }); err != nil {
		errs = append(errs, err)
	}

	if len(errs) > 0 {
		return errors.Join(errs...)
	}
	return nil
}

func (sm *SessionManager) GetSession(id string) *Session {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	return sm.sessions[id]
}

func (sm *SessionManager) Count() int {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	return len(sm.sessions)
}

func (s *Session) StartStreamHandler() error {
	if s.Stream != nil {
		s.StopStreamHandler()
	}
	if s.Reflector == nil {
		return fmt.Errorf("no reflector connected")
	}

	udpConn := s.Reflector.Conn()
	reflectorAddr := s.Reflector.Addr()

	dstID := fmt.Sprintf("%s %c", s.Reflector.Designator, s.Reflector.Module)
	handler, err := m17.NewStreamHandler(udpConn, reflectorAddr, s.Callsign, dstID)
	if err != nil {
		return err
	}

	s.Stream = handler

	s.streamStop = make(chan struct{})
	s.streamWG.Add(1)
	go func() {
		defer s.streamWG.Done()
		s.handleReflectorPackets(s.streamStop)
	}()

	return nil
}

func (s *Session) StopStreamHandler() {
	if s.streamStop != nil {
		close(s.streamStop)
		s.streamStop = nil
	}
	s.streamWG.Wait()
	if s.Stream != nil {
		s.Stream.Close()
		s.Stream = nil
	}
}

func (s *Session) HandleG711Frame(frame []byte, isLast bool) error {
	if s.Stream == nil {
		return fmt.Errorf("no active stream handler")
	}
	s.pcmBuf = audio.MuLawDecode(s.pcmBuf, frame)
	return s.Stream.SendPCMFrame(s.pcmBuf, isLast)
}

func (s *Session) HandlePCMFrame(frame []int16, isLast bool) error {
	if s.Stream == nil {
		return fmt.Errorf("no active stream handler")
	}
	return s.Stream.SendPCMFrame(frame, isLast)
}

func (s *Session) handleReflectorPackets(stop <-chan struct{}) {
	if s.Reflector == nil {
		return
	}

	timer := time.NewTimer(reflectorTimeout)
	defer timer.Stop()

	rxActive := false

	for {
		select {
		case <-stop:
			if rxActive {
				rxActive = false
				s.notifyRxInactive()
			}
			return
		case pkt, ok := <-s.Reflector.Packets:
			if !ok {
				if rxActive {
					rxActive = false
					s.notifyRxInactive()
				}
				return
			}

			s.processPacket(pkt, &rxActive)
			if rxActive {
				if !timer.Stop() {
					<-timer.C
				}
				timer.Reset(reflectorTimeout)
			}

		case <-s.Reflector.Done():
			if rxActive {
				rxActive = false
				s.notifyRxInactive()
			}
			return

		case <-timer.C:
			if rxActive {
				rxActive = false
				s.notifyRxInactive()
			}
			timer.Reset(reflectorTimeout)
		}
	}
}

func (s *Session) notifyRxActive(src string) {
	select {
	case s.OutgoingMessages <- ServerMessage{Type: "rx", Data: marshalData(RxStatusMessage{Active: true, Src: src})}:
	default:
	}
}

func (s *Session) notifyRxInactive() {
	select {
	case s.OutgoingMessages <- ServerMessage{Type: "rx", Data: marshalData(RxStatusMessage{Active: false})}:
	default:
	}
}

func (s *Session) processPacket(pkt []byte, rxActive *bool) {
	if len(pkt) < 4 || string(pkt[0:4]) != "M17 " {
		return
	}

	spkt, lsf, err := m17.ParseStreamPacketWithLSF(pkt)
	if err != nil {
		log.Warn("failed to parse incoming stream", "session", s.ID, "err", err)
		return
	}

	log.Debug("Incoming stream", "stream_id", spkt.StreamID, "src", lsf.Source, "dst", lsf.Destination, "session", s.ID)

	if !*rxActive {
		*rxActive = true
		s.notifyRxActive(lsf.Source)
	}

	audioFrame, err := s.Stream.HandleIncomingPacket(pkt, s.UsePCM)
	if err != nil {
		log.Warn("failed to parse incoming stream", "session", s.ID, "err", err)
		return
	}
	if len(audioFrame) != 0 {
		select {
		case s.OutgoingAudio <- audioFrame:
		default:
			log.Warn("dropping audio frame; outgoing channel full", "session", s.ID)
			status.RecordAudioFrameDropped()
		}
	}

	if spkt.IsLast() && *rxActive {
		*rxActive = false
		s.notifyRxInactive()
	}
}
