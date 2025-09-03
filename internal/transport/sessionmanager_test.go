package transport

import "testing"

func TestSessionManagerAddRemove(t *testing.T) {
	sm := NewSessionManager()
	if sm.Count() != 0 {
		t.Fatalf("expected zero sessions")
	}
	s, err := sm.AddSession()
	if err != nil {
		t.Fatalf("AddSession returned error: %v", err)
	}
	if sm.Count() != 1 {
		t.Fatalf("expected one session")
	}
	if got := sm.GetSession(s.ID); got != s {
		t.Fatalf("GetSession returned unexpected session")
	}
	sm.RemoveSession(s.ID)
	if sm.Count() != 0 {
		t.Fatalf("expected zero sessions after removal")
	}
	if _, ok := <-s.OutgoingAudio; ok {
		t.Fatalf("OutgoingAudio channel not closed")
	}
	if _, ok := <-s.OutgoingMessages; ok {
		t.Fatalf("OutgoingMessages channel not closed")
	}
}

func TestSessionManagerRemoveSessionIdempotent(t *testing.T) {
	sm := NewSessionManager()
	s, err := sm.AddSession()
	if err != nil {
		t.Fatalf("AddSession returned error: %v", err)
	}
	sm.RemoveSession(s.ID)
	sm.RemoveSession(s.ID)
	if sm.Count() != 0 {
		t.Fatalf("expected zero sessions after second removal")
	}
	if _, ok := <-s.OutgoingAudio; ok {
		t.Fatalf("OutgoingAudio channel not closed")
	}
	if _, ok := <-s.OutgoingMessages; ok {
		t.Fatalf("OutgoingMessages channel not closed")
	}
}

func TestSessionManagerMaxSessions(t *testing.T) {
	sm := NewSessionManager()
	sm.MaxSessions = 1
	if _, err := sm.AddSession(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, err := sm.AddSession(); err == nil {
		t.Fatalf("expected error when max sessions reached")
	}
}
