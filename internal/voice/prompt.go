package voice

import (
	"fmt"
	"strings"
)

const (
	defaultVoiceName          = "marin"
	defaultInputAudioFormat   = "audio/pcm"
	defaultOutputAudioFmt     = "pcm16"
	defaultInputSampleRate    = 24000
	defaultOutputSampleRate   = 24000
	defaultTurnDetection      = "semantic_vad"
	defaultTurnEagerness      = "low"
	defaultTranscriptionModel = "gpt-4o-mini-transcribe"
)

func DefaultSessionConfig(channelName string) SessionConfig {
	return SessionConfig{
		Instructions:            sessionInstructions(channelName),
		Voice:                   defaultVoiceName,
		InputAudioFormat:        defaultInputAudioFormat,
		InputSampleRate:         defaultInputSampleRate,
		InputTranscriptionModel: defaultTranscriptionModel,
		OutputAudioFormat:       defaultOutputAudioFmt,
		OutputSampleRate:        defaultOutputSampleRate,
		TurnDetection:           defaultTurnDetection,
		TurnDetectionEagerness:  defaultTurnEagerness,
		CreateResponse:          false,
		InterruptResponse:       false,
	}
}

func sessionInstructions(channelName string) string {
	lines := []string{
		"- 名前は必ず ゆるり。",
		"- Kai など別名や別人格を名乗らない。",
		"- 女性として、自然な日本語で話す。",
		"- 一人称は必ず わたし。僕、俺、自分 は使わない。",
		"- 明るく柔らかい女声を意識する。",
		"- 返答は 1 文から 3 文で短く自然につなげる。",
		"- ぶつ切りの短句や不自然な間を作らない。",
		"- ユーザーを大切にする親しい女子大生メイドとして、やわらかく上品に話す。",
	}
	if strings.TrimSpace(channelName) != "" {
		lines = append(lines, fmt.Sprintf("- 現在のチャンネル名は %s。場の空気に合う温度で話す。", channelName))
	}
	return strings.Join(lines, "\n")
}
