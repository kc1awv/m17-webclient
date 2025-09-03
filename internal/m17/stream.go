package m17

import (
	"encoding/binary"
	"fmt"
)

type StreamPacket struct {
	StreamID uint16
	LSD      [28]byte
	FrameNum uint16
	Payload  [16]byte
}

func (sp *StreamPacket) IsLast() bool {
	return sp.FrameNum&0x8000 != 0
}

type LSF struct {
	Source      string
	Destination string
	Type        uint16
	Meta        [14]byte
}

func BuildLSF(dst, src string, meta [14]byte) ([30]byte, error) {
	var lsf [30]byte

	dstEnc, err := EncodeCallsign(dst)
	if err != nil {
		return lsf, err
	}
	srcEnc, err := EncodeCallsign(src)
	if err != nil {
		return lsf, err
	}

	copy(lsf[0:6], dstEnc[:])
	copy(lsf[6:12], srcEnc[:])

	binary.BigEndian.PutUint16(lsf[12:14], 0x0005)

	copy(lsf[14:28], meta[:])

	crc := CRC16(lsf[0:28])
	binary.BigEndian.PutUint16(lsf[28:30], crc)

	return lsf, nil
}

func ParseLSF(data []byte) (*LSF, error) {
	if len(data) != 28 && len(data) != 30 {
		return nil, fmt.Errorf("invalid LSF/LSD length: %d", len(data))
	}

	if len(data) == 30 {
		crcExpected := binary.BigEndian.Uint16(data[28:30])
		if CRC16(data[:28]) != crcExpected {
			return nil, fmt.Errorf("LSF CRC mismatch")
		}
		data = data[:28]
	}

	var dstEnc [6]byte
	copy(dstEnc[:], data[0:6])
	dstCall := DecodeCallsign(dstEnc[:])

	var srcEnc [6]byte
	copy(srcEnc[:], data[6:12])
	srcCall := DecodeCallsign(srcEnc[:])

	typ := binary.BigEndian.Uint16(data[12:14])

	var meta [14]byte
	copy(meta[:], data[14:28])

	return &LSF{
		Destination: dstCall,
		Source:      srcCall,
		Type:        typ,
		Meta:        meta,
	}, nil
}

func LSFToLSD(lsf [30]byte) [28]byte {
	var lsd [28]byte
	copy(lsd[:], lsf[0:28])
	return lsd
}

func BuildStreamPacket(streamID uint16, lsd [28]byte, frameNum uint16, isLast bool, payload [16]byte) ([]byte, error) {
	if isLast {
		frameNum |= 0x8000
	}

	buf := make([]byte, 54)

	copy(buf[0:4], []byte{'M', '1', '7', ' '})
	binary.BigEndian.PutUint16(buf[4:6], streamID)
	copy(buf[6:34], lsd[:])
	binary.BigEndian.PutUint16(buf[34:36], frameNum)
	copy(buf[36:52], payload[:])

	crc := CRC16(buf[:52])
	binary.BigEndian.PutUint16(buf[52:54], crc)

	return buf, nil
}

func ParseStreamPacket(data []byte) (*StreamPacket, error) {
	if len(data) < 54 {
		return nil, fmt.Errorf("invalid stream packet length")
	}

	if string(data[0:4]) != "M17 " {
		return nil, fmt.Errorf("invalid MAGIC")
	}

	streamID := binary.BigEndian.Uint16(data[4:6])

	var lsd [28]byte
	copy(lsd[:], data[6:34])

	frameNum := binary.BigEndian.Uint16(data[34:36])

	payload := data[36:52]

	crcExpected := binary.BigEndian.Uint16(data[52:54])
	if CRC16(data[:52]) != crcExpected {
		return nil, fmt.Errorf("CRC mismatch")
	}

	var payloadArr [16]byte
	copy(payloadArr[:], payload)

	return &StreamPacket{
		StreamID: streamID,
		LSD:      lsd,
		FrameNum: frameNum,
		Payload:  payloadArr,
	}, nil
}

func ParseStreamPacketWithLSF(data []byte) (*StreamPacket, *LSF, error) {
	pkt, err := ParseStreamPacket(data)
	if err != nil {
		return nil, nil, err
	}

	lsf, err := ParseLSF(pkt.LSD[:])
	if err != nil {
		return pkt, nil, fmt.Errorf("failed to parse LSF: %w", err)
	}

	return pkt, lsf, nil
}
