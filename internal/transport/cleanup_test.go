package transport

import "testing"

func TestCleanupSessionReportsError(t *testing.T) {
	s := &Session{
		OutgoingAudio:    make(chan []byte),
		OutgoingMessages: make(chan ServerMessage),
	}
	close(s.OutgoingAudio)
	close(s.OutgoingMessages)
	if err := cleanupSession(s); err == nil {
		t.Fatalf("expected error from cleanupSession when channels are pre-closed")
	}
}
