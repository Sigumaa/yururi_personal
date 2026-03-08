package voice

import (
	"fmt"
	"unsafe"
)

/*
#cgo darwin,arm64 CFLAGS: -I/opt/homebrew/include -I/opt/homebrew/opt/opus/include
#cgo darwin,arm64 LDFLAGS: -L/opt/homebrew/lib -L/opt/homebrew/opt/opus/lib -lopus
#cgo darwin,amd64 CFLAGS: -I/usr/local/include -I/usr/local/opt/opus/include
#cgo darwin,amd64 LDFLAGS: -L/usr/local/lib -L/usr/local/opt/opus/lib -lopus
#cgo linux LDFLAGS: -lopus
#include <stdlib.h>
#include <opus/opus.h>
*/
import "C"

type opusDecoder struct {
	ptr      *C.OpusDecoder
	channels int
}

type opusEncoder struct {
	ptr      *C.OpusEncoder
	channels int
}

func newOpusDecoder(sampleRate int, channels int) (*opusDecoder, error) {
	var code C.int
	ptr := C.opus_decoder_create(C.opus_int32(sampleRate), C.int(channels), &code)
	if code != C.OPUS_OK {
		return nil, fmt.Errorf("opus decoder create failed: %d", int(code))
	}
	return &opusDecoder{ptr: ptr, channels: channels}, nil
}

func (d *opusDecoder) Decode(data []byte, pcm []int16) (int, error) {
	if d == nil || d.ptr == nil {
		return 0, fmt.Errorf("opus decoder is not initialized")
	}
	if len(data) == 0 || len(pcm) == 0 {
		return 0, nil
	}
	frameSize := len(pcm) / d.channels
	n := int(C.opus_decode(
		d.ptr,
		(*C.uchar)(unsafe.Pointer(&data[0])),
		C.opus_int32(len(data)),
		(*C.opus_int16)(unsafe.Pointer(&pcm[0])),
		C.int(frameSize),
		0,
	))
	if n < 0 {
		return 0, fmt.Errorf("opus decode failed: %d", n)
	}
	return n, nil
}

func (d *opusDecoder) Close() {
	if d == nil || d.ptr == nil {
		return
	}
	C.opus_decoder_destroy(d.ptr)
	d.ptr = nil
}

func newOpusEncoder(sampleRate int, channels int) (*opusEncoder, error) {
	var code C.int
	ptr := C.opus_encoder_create(C.opus_int32(sampleRate), C.int(channels), C.OPUS_APPLICATION_AUDIO, &code)
	if code != C.OPUS_OK {
		return nil, fmt.Errorf("opus encoder create failed: %d", int(code))
	}
	return &opusEncoder{ptr: ptr, channels: channels}, nil
}

func (e *opusEncoder) Encode(pcm []int16, data []byte) (int, error) {
	if e == nil || e.ptr == nil {
		return 0, fmt.Errorf("opus encoder is not initialized")
	}
	if len(pcm) == 0 || len(data) == 0 {
		return 0, nil
	}
	frameSize := len(pcm) / e.channels
	n := int(C.opus_encode(
		e.ptr,
		(*C.opus_int16)(unsafe.Pointer(&pcm[0])),
		C.int(frameSize),
		(*C.uchar)(unsafe.Pointer(&data[0])),
		C.opus_int32(len(data)),
	))
	if n < 0 {
		return 0, fmt.Errorf("opus encode failed: %d", n)
	}
	return n, nil
}

func (e *opusEncoder) Close() {
	if e == nil || e.ptr == nil {
		return
	}
	C.opus_encoder_destroy(e.ptr)
	e.ptr = nil
}
