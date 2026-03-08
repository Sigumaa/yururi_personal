package voice

import (
	"encoding/base64"
	"encoding/binary"
	"fmt"
	"sync"
)

const (
	discordSampleRate   = 48000
	discordChannels     = 2
	discordFrameSamples = 960
	realtimeSampleRate  = 24000
	realtimeChannels    = 1
	maxOpusPacketSize   = 4096
)

type audioRuntime struct {
	decoder *opusDecoder
	encoder *opusEncoder

	mu     sync.Mutex
	output []int16
}

func newAudioRuntime() (*audioRuntime, error) {
	decoder, err := newOpusDecoder(discordSampleRate, discordChannels)
	if err != nil {
		return nil, fmt.Errorf("create opus decoder: %w", err)
	}
	encoder, err := newOpusEncoder(discordSampleRate, discordChannels)
	if err != nil {
		decoder.Close()
		return nil, fmt.Errorf("create opus encoder: %w", err)
	}
	return &audioRuntime{
		decoder: decoder,
		encoder: encoder,
	}, nil
}

func (a *audioRuntime) decodeDiscordOpus(packet []byte) ([]int16, error) {
	if len(packet) == 0 {
		return nil, nil
	}
	pcm := make([]int16, discordFrameSamples*discordChannels*6)
	samples, err := a.decoder.Decode(packet, pcm)
	if err != nil {
		return nil, fmt.Errorf("decode opus packet: %w", err)
	}
	return pcm[:samples*discordChannels], nil
}

func pcm16BytesToSamples(data []byte) []int16 {
	if len(data) < 2 {
		return nil
	}
	out := make([]int16, len(data)/2)
	for i := range out {
		out[i] = int16(binary.LittleEndian.Uint16(data[i*2:]))
	}
	return out
}

func samplesToPCM16Bytes(samples []int16) []byte {
	if len(samples) == 0 {
		return nil
	}
	out := make([]byte, len(samples)*2)
	for i, sample := range samples {
		binary.LittleEndian.PutUint16(out[i*2:], uint16(sample))
	}
	return out
}

func decodeAudioDelta(encoded string) ([]int16, error) {
	raw, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return nil, fmt.Errorf("decode audio delta: %w", err)
	}
	return pcm16BytesToSamples(raw), nil
}

func downsampleDiscordToRealtime(samples []int16) []int16 {
	if len(samples) < discordChannels*2 {
		return nil
	}
	frames := len(samples) / discordChannels
	out := make([]int16, 0, frames/2)
	for frame := 0; frame+1 < frames; frame += 2 {
		left := int(samples[frame*discordChannels])
		right := int(samples[frame*discordChannels+1])
		out = append(out, int16((left+right)/2))
	}
	return out
}

func upsampleRealtimeToDiscord(samples []int16) []int16 {
	if len(samples) == 0 {
		return nil
	}
	out := make([]int16, 0, len(samples)*4)
	for _, sample := range samples {
		out = append(out, sample, sample, sample, sample)
	}
	return out
}

func (a *audioRuntime) appendRealtimeOutput(samples []int16) {
	if len(samples) == 0 {
		return
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	a.output = append(a.output, upsampleRealtimeToDiscord(samples)...)
}

func (a *audioRuntime) drainOpusFrames() ([][]byte, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if len(a.output) == 0 {
		return nil, nil
	}
	frameSize := discordFrameSamples * discordChannels
	frames := make([][]byte, 0, len(a.output)/frameSize+1)
	for len(a.output) >= frameSize {
		frame := a.output[:frameSize]
		encoded := make([]byte, maxOpusPacketSize)
		n, err := a.encoder.Encode(frame, encoded)
		if err != nil {
			return nil, fmt.Errorf("encode opus frame: %w", err)
		}
		frames = append(frames, append([]byte(nil), encoded[:n]...))
		a.output = a.output[frameSize:]
	}
	return frames, nil
}

func (a *audioRuntime) resetOutput() {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.output = nil
}

func (a *audioRuntime) Close() {
	if a == nil {
		return
	}
	if a.decoder != nil {
		a.decoder.Close()
	}
	if a.encoder != nil {
		a.encoder.Close()
	}
}
