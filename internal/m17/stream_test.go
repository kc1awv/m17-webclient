package m17

import "testing"

func TestStreamPacketLastFlag(t *testing.T) {
	var lsd [28]byte
	var payload [16]byte

	pktBytes, err := BuildStreamPacket(0x1234, lsd, 0x0042, true, payload)
	if err != nil {
		t.Fatalf("BuildStreamPacket: %v", err)
	}

	sp, err := ParseStreamPacket(pktBytes)
	if err != nil {
		t.Fatalf("ParseStreamPacket: %v", err)
	}

	if sp.FrameNum != 0x8042 {
		t.Fatalf("expected frame number 0x8042, got %#04x", sp.FrameNum)
	}
	if !sp.IsLast() {
		t.Fatalf("expected IsLast true")
	}

	pktBytes, err = BuildStreamPacket(0x1234, lsd, 0x0043, false, payload)
	if err != nil {
		t.Fatalf("BuildStreamPacket: %v", err)
	}
	sp, err = ParseStreamPacket(pktBytes)
	if err != nil {
		t.Fatalf("ParseStreamPacket: %v", err)
	}
	if sp.FrameNum != 0x0043 {
		t.Fatalf("expected frame number 0x0043, got %#04x", sp.FrameNum)
	}
	if sp.IsLast() {
		t.Fatalf("expected IsLast false")
	}
}
