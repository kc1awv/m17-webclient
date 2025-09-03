package m17

import (
	"crypto/rand"
	"encoding/binary"
	"fmt"
	"net"
	"strings"

	"github.com/kc1awv/m17-webclient/internal/audio"
)

type StreamHandler struct {
	udpConn    *net.UDPConn
	reflector  *net.UDPAddr
	codec2Inst *Codec2
	streamID   uint16
	lsd        [28]byte
	frameNum   uint16
	pcmBuffer  []int16
	muBuf      []byte
}

func generateStreamID() (uint16, error) {
	var id uint16
	if err := binary.Read(rand.Reader, binary.BigEndian, &id); err != nil {
		return 0, err
	}
	return id, nil
}

func NewStreamHandler(conn *net.UDPConn, reflectorAddr *net.UDPAddr, src, dst string) (*StreamHandler, error) {
	if len(dst) > 9 || strings.Contains(dst, ":") {
		dst = src
	}

	meta := [14]byte{}
	lsf, err := BuildLSF(dst, src, meta)
	if err != nil {
		return nil, fmt.Errorf("BuildLSF failed: %w", err)
	}
	lsd := LSFToLSD(lsf)

	c2, err := New(MODE_3200)
	if err != nil {
		return nil, fmt.Errorf("Codec2 init failed: %w", err)
	}

	sid, err := generateStreamID()
	if err != nil {
		return nil, fmt.Errorf("failed to generate stream ID: %w", err)
	}

	return &StreamHandler{
		udpConn:    conn,
		reflector:  reflectorAddr,
		codec2Inst: c2,
		streamID:   sid,
		lsd:        lsd,
		frameNum:   0,
		pcmBuffer:  make([]int16, 0, 320),
		muBuf:      make([]byte, 0, 320),
	}, nil
}

func (sh *StreamHandler) StartNewStream() error {
	sid, err := generateStreamID()
	if err != nil {
		return fmt.Errorf("failed to generate stream ID: %w", err)
	}
	sh.streamID = sid
	sh.frameNum = 0
	sh.pcmBuffer = sh.pcmBuffer[:0]
	return nil
}

func (sh *StreamHandler) buildPayload(pcm []int16) ([16]byte, error) {
	part1, err := sh.codec2Inst.Encode(pcm[:160])
	if err != nil {
		return [16]byte{}, err
	}
	part2, err := sh.codec2Inst.Encode(pcm[160:])
	if err != nil {
		return [16]byte{}, err
	}
	var payload [16]byte
	copy(payload[0:8], part1)
	copy(payload[8:16], part2)
	return payload, nil
}

func (sh *StreamHandler) SendPCMFrame(pcm []int16, isLast bool) error {
	sh.pcmBuffer = append(sh.pcmBuffer, pcm...)

	for len(sh.pcmBuffer) >= 320 {
		markLast := isLast && len(sh.pcmBuffer) == 320

		payload, err := sh.buildPayload(sh.pcmBuffer[:320])
		if err != nil {
			return err
		}

		pkt, err := BuildStreamPacket(sh.streamID, sh.lsd, sh.frameNum, markLast, payload)
		if err != nil {
			return err
		}

		if _, err := sh.udpConn.WriteToUDP(pkt, sh.reflector); err != nil {
			return err
		}

		sh.frameNum++
		sh.pcmBuffer = sh.pcmBuffer[320:]
	}

	if isLast && len(sh.pcmBuffer) > 0 {
		padded := make([]int16, 320)
		copy(padded, sh.pcmBuffer)

		payload, err := sh.buildPayload(padded)
		if err != nil {
			return err
		}

		pkt, err := BuildStreamPacket(sh.streamID, sh.lsd, sh.frameNum, true, payload)
		if err != nil {
			return err
		}
		if _, err := sh.udpConn.WriteToUDP(pkt, sh.reflector); err != nil {
			return err
		}
		sh.frameNum++
		sh.pcmBuffer = sh.pcmBuffer[:0]
	}

	return nil
}

func (sh *StreamHandler) Finalize() error {
	return sh.SendPCMFrame(nil, true)
}

func (sh *StreamHandler) HandleIncomingPacket(data []byte, wantPCM bool) ([]byte, error) {
	pkt, _, err := ParseStreamPacketWithLSF(data)
	if err != nil {
		return nil, err
	}

	part1, err := sh.codec2Inst.Decode(pkt.Payload[0:8])
	if err != nil {
		return nil, err
	}
	part2, err := sh.codec2Inst.Decode(pkt.Payload[8:16])
	if err != nil {
		return nil, err
	}
	pcm8k := make([]int16, 0, len(part1)+len(part2))
	pcm8k = append(pcm8k, part1...)
	pcm8k = append(pcm8k, part2...)

	if wantPCM {
		out := make([]byte, len(pcm8k)*2)
		for i, s := range pcm8k {
			binary.LittleEndian.PutUint16(out[i*2:], uint16(s))
		}
		return out, nil
	}
	sh.muBuf = audio.MuLawEncode(sh.muBuf, pcm8k)
	return sh.muBuf, nil
}

func (sh *StreamHandler) Close() {
	if sh.codec2Inst != nil {
		sh.codec2Inst.Close()
	}
}
