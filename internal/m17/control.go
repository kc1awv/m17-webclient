package m17

import (
	"bytes"
	"errors"
	"fmt"
	"strings"
)

const (
	MagicCONN = "CONN"
	MagicACKN = "ACKN"
	MagicNACK = "NACK"
	MagicPING = "PING"
	MagicPONG = "PONG"
	MagicDISC = "DISC"
)

type ControlType int

const (
	CtrlUnknown ControlType = iota
	CtrlCONN
	CtrlACKN
	CtrlNACK
	CtrlPING
	CtrlPONG
	CtrlDISC
)

func ParseControlPacket(data []byte) (ControlType, string, byte, error) {
	if len(data) < 4 {
		return CtrlUnknown, "", 0, errors.New("packet too short")
	}

	magic := string(data[:4])

	switch magic {
	case MagicCONN:
		if len(data) < 11 {
			return CtrlUnknown, "", 0, errors.New("invalid CONN length")
		}
		cs := DecodeCallsign(data[4:10])
		module := data[10]
		return CtrlCONN, cs, module, nil
	case MagicACKN:
		if len(data) < 4 {
			return CtrlUnknown, "", 0, errors.New("invalid ACKN length")
		}
		return CtrlACKN, "", 0, nil

	case MagicNACK:
		if len(data) < 4 {
			return CtrlUnknown, "", 0, errors.New("invalid NACK length")
		}
		return CtrlNACK, "", 0, nil

	case MagicPING:
		if len(data) < 10 {
			return CtrlUnknown, "", 0, errors.New("invalid PING length")
		}
		cs := DecodeCallsign(data[4:10])
		return CtrlPING, cs, 0, nil

	case MagicPONG:
		if len(data) < 10 {
			return CtrlUnknown, "", 0, errors.New("invalid PONG length")
		}
		cs := DecodeCallsign(data[4:10])
		return CtrlPONG, cs, 0, nil

	case MagicDISC:
		if len(data) == 4 {
			return CtrlDISC, "", 0, nil
		}
		if len(data) < 10 {
			return CtrlUnknown, "", 0, errors.New("invalid DISC length")
		}
		cs := DecodeCallsign(data[4:10])
		return CtrlDISC, cs, 0, nil

	default:
		return CtrlUnknown, "", 0, errors.New("unknown control magic")
	}
}

func BuildCONN(callsign string, module byte) ([]byte, error) {
	pkt, err := buildControlPacket(MagicCONN, callsign)
	if err != nil {
		return nil, err
	}
	return append(pkt, module), nil
}

func BuildPONG(callsign string) ([]byte, error) {
	return buildControlPacket(MagicPONG, callsign)
}

func BuildDISC(callsign string) ([]byte, error) {
	return buildControlPacket(MagicDISC, callsign)
}

func buildControlPacket(magic string, callsign string) ([]byte, error) {
	var buf bytes.Buffer
	buf.WriteString(magic)

	encoded, err := EncodeCallsign(callsign)
	if err != nil {
		return nil, err
	}
	buf.Write(encoded[:])

	return buf.Bytes(), nil
}

const base40Chars = " ABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789-/."

func ValidateCallsign(callsign string) (string, error) {
	callsign = strings.ToUpper(callsign)

	if len(callsign) > 9 {
		return "", fmt.Errorf("callsign too long: max 9 characters")
	}

	for i := 0; i < len(callsign); i++ {
		c := callsign[i]
		switch {
		case c == ' ':
		case 'A' <= c && c <= 'Z':
		case '0' <= c && c <= '9':
		case c == '-':
		case c == '/':
		case c == '.':
		default:
			return "", fmt.Errorf("invalid character in callsign: %c", c)
		}
	}

	return callsign, nil
}

func EncodeCallsign(callsign string) ([]byte, error) {
	callsign = strings.ToUpper(callsign)

	if len(callsign) < 9 {
		callsign = callsign + strings.Repeat(" ", 9-len(callsign))
	} else if len(callsign) > 9 {
		return nil, fmt.Errorf("callsign too long: max 9 characters")
	}

	address := uint64(0)
	for i := len(callsign) - 1; i >= 0; i-- {
		c := callsign[i]
		val := 0
		switch {
		case c == ' ':
			val = 0
		case 'A' <= c && c <= 'Z':
			val = int(c-'A') + 1
		case '0' <= c && c <= '9':
			val = int(c-'0') + 27
		case c == '-':
			val = 37
		case c == '/':
			val = 38
		case c == '.':
			val = 39
		default:
			return nil, fmt.Errorf("invalid character in callsign: %c", c)
		}
		address = address*40 + uint64(val)
	}

	result := make([]byte, 6)
	for i := 5; i >= 0; i-- {
		result[i] = byte(address & 0xFF)
		address >>= 8
	}

	return result, nil
}

func DecodeCallsign(encoded []byte) string {
	if len(encoded) != 6 {
		return ""
	}

	address := uint64(0)
	for _, b := range encoded {
		address = address*256 + uint64(b)
	}

	chars := make([]rune, 0, 9)
	for address > 0 {
		idx := address % 40
		chars = append(chars, rune(base40Chars[idx]))
		address /= 40
	}

	return strings.TrimSpace(string(chars))
}
