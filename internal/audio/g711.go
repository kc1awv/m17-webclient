package audio

func MuLawEncode(dst []byte, pcm []int16) []byte {
	if cap(dst) < len(pcm) {
		dst = make([]byte, len(pcm))
	} else {
		dst = dst[:len(pcm)]
	}
	for i, s := range pcm {
		dst[i] = linearToMuLaw(s)
	}
	return dst
}

func MuLawDecode(dst []int16, mu []byte) []int16 {
	if cap(dst) < len(mu) {
		dst = make([]int16, len(mu))
	} else {
		dst = dst[:len(mu)]
	}
	for i, b := range mu {
		dst[i] = muLawToLinear(b)
	}
	return dst
}

const (
	muLawBias = 0x84
	muLawClip = 32635
)

func linearToMuLaw(sample int16) byte {
	s := int(sample)
	sign := byte((s >> 8) & 0x80)
	if sign != 0 {
		s = -s
	}
	if s > muLawClip {
		s = muLawClip
	}
	s += muLawBias

	exponent := byte(7)
	mask := 0x4000
	for exponent > 0 && (s&mask) == 0 {
		mask >>= 1
		exponent--
	}
	mantissa := byte((s >> (exponent + 3)) & 0x0F)
	return ^(sign | (exponent << 4) | mantissa)
}

func muLawToLinear(mu byte) int16 {
	mu = ^mu
	sign := mu & 0x80
	exponent := (mu >> 4) & 0x07
	mantissa := mu & 0x0F
	t := (int(mantissa) << 3) + muLawBias
	t <<= exponent
	if sign != 0 {
		t = muLawBias - t
	} else {
		t = t - muLawBias
	}
	return int16(t)
}
