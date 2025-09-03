package m17

import (
	"testing"
)

func TestCallsignEncodeDecode(t *testing.T) {
	tests := []string{
		"KC1ABC",
		"N0CALL",
		"W1AW",
		"TEST123",
		"ABCD",
	}

	for _, cs := range tests {
		enc, err := EncodeCallsign(cs)
		if err != nil {
			t.Errorf("Encode error for %s: %v", cs, err)
			continue
		}

		dec := DecodeCallsign(enc)
		if dec != cs {
			t.Errorf("Roundtrip mismatch: got %s, want %s", dec, cs)
		}
	}
}

func TestValidateCallsign(t *testing.T) {
	valid := map[string]string{
		"kc1abc":  "KC1ABC",
		"N0CALL":  "N0CALL",
		"AB-CD/.": "AB-CD/.",
		"TEST123": "TEST123",
	}
	for in, expect := range valid {
		out, err := ValidateCallsign(in)
		if err != nil {
			t.Fatalf("unexpected error for %s: %v", in, err)
		}
		if out != expect {
			t.Errorf("ValidateCallsign(%s) = %s, want %s", in, out, expect)
		}
	}

	invalid := []string{"TOO-LONGCS", "BAD$"}
	for _, in := range invalid {
		if _, err := ValidateCallsign(in); err == nil {
			t.Errorf("expected error for %s", in)
		}
	}
}

func TestControlPacketRoundTrip(t *testing.T) {
	packet, err := BuildCONN("KC1ABC", 'A')
	if err != nil {
		t.Fatalf("BuildCONN failed: %v", err)
	}

	ctrlType, callsign, module, err := ParseControlPacket(packet)
	if err != nil {
		t.Fatalf("ParseControlPacket failed: %v", err)
	}

	if ctrlType != CtrlCONN {
		t.Errorf("Expected CtrlCONN, got %v", ctrlType)
	}
	if callsign != "KC1ABC" {
		t.Errorf("Expected callsign KC1ABC, got %s", callsign)
	}
	if module != 'A' {
		t.Errorf("Expected module 'A', got %c", module)
	}
}
