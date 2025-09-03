package m17

import (
	"bytes"
	"net"
	"testing"
	"time"
)

func newTestStreamHandler(t *testing.T) (*StreamHandler, *net.UDPConn) {
	t.Helper()
	reflector, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: 0})
	if err != nil {
		t.Fatalf("listen reflector: %v", err)
	}
	sender, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: 0})
	if err != nil {
		t.Fatalf("listen sender: %v", err)
	}
	sh, err := NewStreamHandler(sender, reflector.LocalAddr().(*net.UDPAddr), "SRC", "DST")
	if err != nil {
		t.Fatalf("NewStreamHandler: %v", err)
	}
	return sh, reflector
}

func TestSendPCMFrameFull(t *testing.T) {
	sh, reflector := newTestStreamHandler(t)
	defer sh.Close()
	defer sh.udpConn.Close()
	defer reflector.Close()

	pcm := make([]int16, 320)
	for i := range pcm {
		pcm[i] = int16(i)
	}

	c2, err := New(MODE_3200)
	if err != nil {
		t.Fatalf("codec2 init: %v", err)
	}
	defer c2.Close()
	part1, err := c2.Encode(pcm[:160])
	if err != nil {
		t.Fatalf("Encode part1: %v", err)
	}
	part2, err := c2.Encode(pcm[160:])
	if err != nil {
		t.Fatalf("Encode part2: %v", err)
	}
	var expected [16]byte
	copy(expected[0:8], part1)
	copy(expected[8:16], part2)

	if err := sh.SendPCMFrame(pcm, true); err != nil {
		t.Fatalf("SendPCMFrame: %v", err)
	}

	reflector.SetReadDeadline(time.Now().Add(time.Second))
	buf := make([]byte, 128)
	n, _, err := reflector.ReadFromUDP(buf)
	if err != nil {
		t.Fatalf("ReadFromUDP: %v", err)
	}

	pkt, err := ParseStreamPacket(buf[:n])
	if err != nil {
		t.Fatalf("ParseStreamPacket: %v", err)
	}

	if !bytes.Equal(pkt.Payload[:], expected[:]) {
		t.Fatalf("payload mismatch")
	}
	if !pkt.IsLast() {
		t.Fatalf("expected last frame")
	}
}

func TestSendPCMFramePadded(t *testing.T) {
	sh, reflector := newTestStreamHandler(t)
	defer sh.Close()
	defer sh.udpConn.Close()
	defer reflector.Close()

	pcm := make([]int16, 160)
	for i := range pcm {
		pcm[i] = int16(i)
	}
	padded := make([]int16, 320)
	copy(padded, pcm)

	c2, err := New(MODE_3200)
	if err != nil {
		t.Fatalf("codec2 init: %v", err)
	}
	defer c2.Close()
	part1, err := c2.Encode(padded[:160])
	if err != nil {
		t.Fatalf("Encode part1: %v", err)
	}
	part2, err := c2.Encode(padded[160:])
	if err != nil {
		t.Fatalf("Encode part2: %v", err)
	}
	var expected [16]byte
	copy(expected[0:8], part1)
	copy(expected[8:16], part2)

	if err := sh.SendPCMFrame(pcm, true); err != nil {
		t.Fatalf("SendPCMFrame: %v", err)
	}

	reflector.SetReadDeadline(time.Now().Add(time.Second))
	buf := make([]byte, 128)
	n, _, err := reflector.ReadFromUDP(buf)
	if err != nil {
		t.Fatalf("ReadFromUDP: %v", err)
	}

	pkt, err := ParseStreamPacket(buf[:n])
	if err != nil {
		t.Fatalf("ParseStreamPacket: %v", err)
	}

	if !bytes.Equal(pkt.Payload[:], expected[:]) {
		t.Fatalf("payload mismatch")
	}
	if !pkt.IsLast() {
		t.Fatalf("expected last frame")
	}
}
