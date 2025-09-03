package transport

import (
	"context"
	"encoding/json"
	"net"
	"testing"
	"time"

	"github.com/kc1awv/m17-webclient/internal/m17"
	"github.com/kc1awv/m17-webclient/internal/reflector"
)

func TestRemoveSessionReleasesGoroutines(t *testing.T) {
	manager := NewSessionManager()
	session, err := manager.AddSession()
	if err != nil {
		t.Fatalf("AddSession returned error: %v", err)
	}

	done := make(chan struct{})
	go func() {
		audioCh := session.OutgoingAudio
		msgCh := session.OutgoingMessages
		for audioCh != nil || msgCh != nil {
			select {
			case _, ok := <-audioCh:
				if !ok {
					audioCh = nil
				}
			case _, ok := <-msgCh:
				if !ok {
					msgCh = nil
				}
			}
		}
		close(done)
	}()

	manager.RemoveSession(session.ID)

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatalf("goroutine did not exit after RemoveSession")
	}
}

func TestHandleReflectorPacketsDoneWithoutLeak(t *testing.T) {
	server, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 0})
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer server.Close()

	ctx, cancel := context.WithCancel(context.Background())
	client, err := reflector.NewReflectorClient(ctx, server.LocalAddr().String(), "TEST", 'A')
	if err != nil {
		t.Fatalf("NewReflectorClient: %v", err)
	}
	defer client.Close()

	conn, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 0})
	if err != nil {
		t.Fatalf("client listen: %v", err)
	}
	defer conn.Close()

	sh, err := m17.NewStreamHandler(conn, server.LocalAddr().(*net.UDPAddr), "SRC", "DST")
	if err != nil {
		t.Fatalf("NewStreamHandler: %v", err)
	}
	defer sh.Close()

	stopCh := make(chan struct{})
	s := &Session{
		Reflector:        client,
		Stream:           sh,
		OutgoingAudio:    make(chan []byte, 1),
		OutgoingMessages: make(chan ServerMessage, 1),
		streamStop:       stopCh,
	}

	done := make(chan struct{})
	go func() {
		s.handleReflectorPackets(stopCh)
		close(done)
	}()

	var meta [14]byte
	lsf, _ := m17.BuildLSF("DST", "SRC", meta)
	lsd := m17.LSFToLSD(lsf)
	var payload [16]byte
	pkt, _ := m17.BuildStreamPacket(0x1234, lsd, 0, false, payload)
	client.Packets <- pkt

	time.Sleep(100 * time.Millisecond)

	cancel()

	select {
	case <-done:
	case <-time.After(500 * time.Millisecond):
		t.Fatalf("handleReflectorPackets did not exit after reflector done")
	}
}

func TestStopStreamHandlerStopsPacketProcessing(t *testing.T) {
	server, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 0})
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer server.Close()

	conn, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 0})
	if err != nil {
		t.Fatalf("client listen: %v", err)
	}
	defer conn.Close()

	packets := make(chan []byte, 10)
	client := reflector.NewTestClient(context.Background(), conn, server.LocalAddr().(*net.UDPAddr), "TEST", 'A', "TEST", packets, nil)
	defer client.Close()

	sh, err := m17.NewStreamHandler(conn, server.LocalAddr().(*net.UDPAddr), "SRC", "DST")
	if err != nil {
		t.Fatalf("NewStreamHandler: %v", err)
	}
	defer sh.Close()

	stopCh := make(chan struct{})
	s := &Session{
		Reflector:        client,
		Stream:           sh,
		OutgoingAudio:    make(chan []byte, 1),
		OutgoingMessages: make(chan ServerMessage, 1),
		streamStop:       stopCh,
	}

	done := make(chan struct{})
	go func() {
		s.handleReflectorPackets(stopCh)
		close(done)
	}()

	s.StopStreamHandler()

	select {
	case <-done:
	case <-time.After(100 * time.Millisecond):
		t.Fatalf("handleReflectorPackets did not exit after StopStreamHandler")
	}

	var meta [14]byte
	lsf, _ := m17.BuildLSF("DST", "SRC", meta)
	lsd := m17.LSFToLSD(lsf)
	var payload [16]byte
	pkt, _ := m17.BuildStreamPacket(0x1234, lsd, 0, false, payload)
	packets <- pkt

	select {
	case <-s.OutgoingAudio:
		t.Fatalf("audio processed after StopStreamHandler")
	case <-s.OutgoingMessages:
		t.Fatalf("message sent after StopStreamHandler")
	case <-time.After(100 * time.Millisecond):
	}
}

func TestProcessPacket(t *testing.T) {
	server, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 0})
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer server.Close()

	conn, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 0})
	if err != nil {
		t.Fatalf("client listen: %v", err)
	}
	defer conn.Close()

	sh, err := m17.NewStreamHandler(conn, server.LocalAddr().(*net.UDPAddr), "SRC", "DST")
	if err != nil {
		t.Fatalf("NewStreamHandler: %v", err)
	}
	defer sh.Close()

	s := &Session{
		Stream:           sh,
		OutgoingAudio:    make(chan []byte, 1),
		OutgoingMessages: make(chan ServerMessage, 2),
	}

	var rxActive bool

	var meta [14]byte
	lsf, _ := m17.BuildLSF("DST", "SRC", meta)
	lsd := m17.LSFToLSD(lsf)
	var payload [16]byte

	pkt, _ := m17.BuildStreamPacket(0x1234, lsd, 0, false, payload)
	s.processPacket(pkt, &rxActive)
	if !rxActive {
		t.Fatalf("rxActive not set")
	}
	select {
	case msg := <-s.OutgoingMessages:
		if msg.Type != "rx" {
			t.Fatalf("unexpected message type: %s", msg.Type)
		}
		var data RxStatusMessage
		if err := json.Unmarshal(msg.Data, &data); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if !data.Active {
			t.Fatalf("expected active true, got %#v", data)
		}
	default:
		t.Fatalf("expected rx active message")
	}

	select {
	case <-s.OutgoingAudio:
	default:
		t.Fatalf("expected audio frame")
	}

	pktLast, _ := m17.BuildStreamPacket(0x1234, lsd, 1, true, payload)
	s.processPacket(pktLast, &rxActive)
	if rxActive {
		t.Fatalf("rxActive not cleared")
	}
	select {
	case msg := <-s.OutgoingMessages:
		if msg.Type != "rx" {
			t.Fatalf("unexpected message type: %s", msg.Type)
		}
		var data RxStatusMessage
		if err := json.Unmarshal(msg.Data, &data); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if data.Active {
			t.Fatalf("expected active false, got %#v", data)
		}
	default:
		t.Fatalf("expected rx inactive message")
	}
}

func TestHandleReflectorPacketsTimeoutExpiration(t *testing.T) {
	old := reflectorTimeout
	reflectorTimeout = 100 * time.Millisecond
	defer func() { reflectorTimeout = old }()

	server, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 0})
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer server.Close()

	ctx, cancel := context.WithCancel(context.Background())
	client, err := reflector.NewReflectorClient(ctx, server.LocalAddr().String(), "TEST", 'A')
	if err != nil {
		t.Fatalf("NewReflectorClient: %v", err)
	}
	defer client.Close()

	conn, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 0})
	if err != nil {
		t.Fatalf("client listen: %v", err)
	}
	defer conn.Close()

	sh, err := m17.NewStreamHandler(conn, server.LocalAddr().(*net.UDPAddr), "SRC", "DST")
	if err != nil {
		t.Fatalf("NewStreamHandler: %v", err)
	}
	defer sh.Close()

	stopCh := make(chan struct{})
	s := &Session{
		Reflector:        client,
		Stream:           sh,
		OutgoingAudio:    make(chan []byte, 1),
		OutgoingMessages: make(chan ServerMessage, 2),
		streamStop:       stopCh,
	}

	done := make(chan struct{})
	go func() {
		s.handleReflectorPackets(stopCh)
		close(done)
	}()

	var meta [14]byte
	lsf, _ := m17.BuildLSF("DST", "SRC", meta)
	lsd := m17.LSFToLSD(lsf)
	var payload [16]byte
	pkt, _ := m17.BuildStreamPacket(0x1234, lsd, 0, false, payload)
	client.Packets <- pkt

	select {
	case msg := <-s.OutgoingMessages:
		if msg.Type != "rx" {
			t.Fatalf("unexpected message type: %s", msg.Type)
		}
		var data RxStatusMessage
		if err := json.Unmarshal(msg.Data, &data); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if !data.Active {
			t.Fatalf("expected active true, got %#v", data)
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatalf("expected rx active message")
	}

	select {
	case msg := <-s.OutgoingMessages:
		if msg.Type != "rx" {
			t.Fatalf("unexpected message type: %s", msg.Type)
		}
		var data RxStatusMessage
		if err := json.Unmarshal(msg.Data, &data); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if data.Active {
			t.Fatalf("expected inactive message, got %#v", data)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatalf("expected rx inactive message after timeout")
	}

	cancel()
	close(stopCh)
	<-done
}

func TestHandleReflectorPacketsNormalFlow(t *testing.T) {
	old := reflectorTimeout
	reflectorTimeout = 100 * time.Millisecond
	defer func() { reflectorTimeout = old }()

	server, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 0})
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer server.Close()

	ctx, cancel := context.WithCancel(context.Background())
	client, err := reflector.NewReflectorClient(ctx, server.LocalAddr().String(), "TEST", 'A')
	if err != nil {
		t.Fatalf("NewReflectorClient: %v", err)
	}
	defer client.Close()

	conn, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 0})
	if err != nil {
		t.Fatalf("client listen: %v", err)
	}
	defer conn.Close()

	sh, err := m17.NewStreamHandler(conn, server.LocalAddr().(*net.UDPAddr), "SRC", "DST")
	if err != nil {
		t.Fatalf("NewStreamHandler: %v", err)
	}
	defer sh.Close()

	stopCh := make(chan struct{})
	s := &Session{
		Reflector:        client,
		Stream:           sh,
		OutgoingAudio:    make(chan []byte, 1),
		OutgoingMessages: make(chan ServerMessage, 2),
		streamStop:       stopCh,
	}

	done := make(chan struct{})
	go func() {
		s.handleReflectorPackets(stopCh)
		close(done)
	}()

	var meta [14]byte
	lsf, _ := m17.BuildLSF("DST", "SRC", meta)
	lsd := m17.LSFToLSD(lsf)
	var payload [16]byte
	pkt, _ := m17.BuildStreamPacket(0x1234, lsd, 0, false, payload)

	client.Packets <- pkt
	<-s.OutgoingMessages

	time.Sleep(50 * time.Millisecond)
	client.Packets <- pkt

	select {
	case msg := <-s.OutgoingMessages:
		t.Fatalf("unexpected message before timeout: %#v", msg)
	case <-time.After(75 * time.Millisecond):
	}

	select {
	case msg := <-s.OutgoingMessages:
		if msg.Type != "rx" {
			t.Fatalf("unexpected message type: %s", msg.Type)
		}
		var data RxStatusMessage
		if err := json.Unmarshal(msg.Data, &data); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if data.Active {
			t.Fatalf("expected inactive message, got %#v", data)
		}
	case <-time.After(200 * time.Millisecond):
		t.Fatalf("expected rx inactive message after reset timeout")
	}

	cancel()
	close(stopCh)
	<-done
}

func TestStartStopStreamHandlerNoPanic(t *testing.T) {
	server, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 0})
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer server.Close()

	conn, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 0})
	if err != nil {
		t.Fatalf("client listen: %v", err)
	}
	defer conn.Close()

	client := reflector.NewTestClient(context.Background(), conn, server.LocalAddr().(*net.UDPAddr), "TEST", 'A', "TEST", nil, nil)
	defer client.Close()

	s := &Session{
		Reflector:        client,
		Callsign:         "SRC",
		OutgoingAudio:    make(chan []byte, 1),
		OutgoingMessages: make(chan ServerMessage, 1),
	}

	var meta [14]byte
	lsf, _ := m17.BuildLSF("DST", "SRC", meta)
	lsd := m17.LSFToLSD(lsf)
	var payload [16]byte
	pkt, _ := m17.BuildStreamPacket(0x1234, lsd, 0, false, payload)

	stopPackets := make(chan struct{})
	go func() {
		for {
			select {
			case <-stopPackets:
				return
			default:
			}
			select {
			case client.Packets <- pkt:
			default:
			}
			time.Sleep(1 * time.Millisecond)
		}
	}()

	for i := 0; i < 10; i++ {
		if err := s.StartStreamHandler(); err != nil {
			t.Fatalf("StartStreamHandler: %v", err)
		}
		time.Sleep(2 * time.Millisecond)
		s.StopStreamHandler()
	}

	close(stopPackets)
}
