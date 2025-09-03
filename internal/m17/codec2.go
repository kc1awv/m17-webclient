package m17

/*
#cgo LDFLAGS: -lcodec2
#include <codec2/codec2.h>
#include <stdlib.h>
*/
import "C"
import (
	"errors"
	"runtime"
	"unsafe"
)

type Codec2 struct {
	handle *C.struct_CODEC2
	mode   int
}

const (
	MODE_3200 = C.CODEC2_MODE_3200
)

func New(mode int) (*Codec2, error) {
	handle := C.codec2_create(C.int(mode))
	if handle == nil {
		return nil, errors.New("failed to create codec2")
	}
	c := &Codec2{handle: handle, mode: mode}
	runtime.SetFinalizer(c, func(c *Codec2) { c.Close() })
	return c, nil
}

func (c *Codec2) Close() {
	if c == nil || c.handle == nil {
		return
	}
	C.codec2_destroy(c.handle)
	c.handle = nil
	runtime.SetFinalizer(c, nil)
}

func (c *Codec2) Encode(pcm []int16) ([]byte, error) {
	nsam := C.codec2_samples_per_frame(c.handle)
	nbit := C.codec2_bits_per_frame(c.handle)

	if len(pcm) != int(nsam) {
		return nil, errors.New("invalid PCM length")
	}

	bits := make([]byte, nbit/8)
	C.codec2_encode(c.handle,
		(*C.uchar)(unsafe.Pointer(&bits[0])),
		(*C.short)(unsafe.Pointer(&pcm[0])),
	)

	return bits, nil
}

func (c *Codec2) Decode(bits []byte) ([]int16, error) {
	nsam := C.codec2_samples_per_frame(c.handle)
	nbit := C.codec2_bits_per_frame(c.handle)

	if len(bits) != int(nbit/8) {
		return nil, errors.New("invalid bit length")
	}

	audio := make([]int16, nsam)
	C.codec2_decode(c.handle, (*C.short)(unsafe.Pointer(&audio[0])), (*C.uchar)(unsafe.Pointer(&bits[0])))

	return audio, nil
}
