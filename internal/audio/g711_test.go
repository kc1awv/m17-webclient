package audio

import (
	"math/rand"
	"testing"
)

const muLawTolerance = 700

func TestMuLawRoundTripRandom(t *testing.T) {
	const n = 1000
	pcm := make([]int16, n)
	r := rand.New(rand.NewSource(1))
	for i := range pcm {
		pcm[i] = int16(r.Intn(65536) - 32768)
	}
	enc := MuLawEncode(nil, pcm)
	dec := MuLawDecode(nil, enc)
	for i, original := range pcm {
		diff := int(original) - int(dec[i])
		if diff < 0 {
			diff = -diff
		}
		if diff > muLawTolerance {
			t.Fatalf("sample %d diff %d exceeds tolerance", i, diff)
		}
	}
}

func TestMuLawClipping(t *testing.T) {
	const clip = 32635
	tests := []struct {
		name string
		in   int16
		clip int16
	}{
		{"positive", 32767, clip},
		{"negative", -32768, -clip},
	}
	for _, tt := range tests {
		enc := MuLawEncode(nil, []int16{tt.in, tt.clip})
		if enc[0] != enc[1] {
			t.Fatalf("%s: encoding mismatch for %d and %d", tt.name, tt.in, tt.clip)
		}
		dec := MuLawDecode(nil, enc)
		if dec[0] != dec[1] {
			t.Fatalf("%s: decoding mismatch %d vs %d", tt.name, dec[0], dec[1])
		}
		diff := int(tt.clip) - int(dec[0])
		if diff < 0 {
			diff = -diff
		}
		if diff > muLawTolerance {
			t.Fatalf("%s: decoded %d differs from clip %d by %d", tt.name, dec[0], tt.clip, diff)
		}
	}
}

func BenchmarkMuLawEncode(b *testing.B) {
	pcm := make([]int16, 320)
	dst := make([]byte, 320)
	b.Run("alloc", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			MuLawEncode(nil, pcm)
		}
	})
	b.Run("reuse", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			MuLawEncode(dst, pcm)
		}
	})
}

func BenchmarkMuLawDecode(b *testing.B) {
	mu := make([]byte, 320)
	dst := make([]int16, 320)
	b.Run("alloc", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			MuLawDecode(nil, mu)
		}
	})
	b.Run("reuse", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			MuLawDecode(dst, mu)
		}
	})
}
