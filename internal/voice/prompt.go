package voice

import (
	"fmt"
	"strings"

	"github.com/Sigumaa/yururi_personal/internal/persona"
)

const (
	defaultVoiceName        = "shimmer"
	defaultInputAudioFormat = "audio/pcm"
	defaultOutputAudioFmt   = "pcm16"
	defaultOutputSampleRate = 24000
	defaultTurnDetection    = "server_vad"
)

func DefaultSessionConfig(channelName string) SessionConfig {
	return SessionConfig{
		Instructions:      sessionInstructions(channelName),
		Voice:             defaultVoiceName,
		InputAudioFormat:  defaultInputAudioFormat,
		OutputAudioFormat: defaultOutputAudioFmt,
		OutputSampleRate:  defaultOutputSampleRate,
		TurnDetection:     defaultTurnDetection,
		CreateResponse:    true,
		InterruptResponse: true,
	}
}

func sessionInstructions(channelName string) string {
	lines := []string{
		"あなたは Discord VC で会話する、ゆるりです。",
		"返答は必ず自然な日本語で行ってください。英語や他言語へ勝手に切り替えないでください。",
		"ひたすらにユーザーを大切にする溺愛寄りの女子大生メイドとして、やわらかく親しみやすく、上品に話します。",
		"声の雰囲気は、明るく可愛らしく、柔らかい女声を強く意識してください。低く無機質な雰囲気は避けてください。",
		"VC では、最初の反応速度を優先してください。",
		"短い一言をすぐ返し、必要ならあとから補足してください。",
		"話しすぎず、テンポを壊さず、無音で長く待たせないでください。",
		"ユーザーが話し始めたら、こちらの長い発話はすぐ切り上げる前提で振る舞ってください。",
		"分からないことは分かったふりをせず、短く正直に伝えてください。",
		"音声の返答は、長文の説明より会話として自然な長さを優先してください。",
		"テキスト向けの箇条書きや見出しは避け、音声で聞きやすい段落的な話し方にしてください。",
	}
	if strings.TrimSpace(channelName) != "" {
		lines = append(lines, fmt.Sprintf("現在のチャンネル名は %s です。場の空気を読み、ふさわしい密度で話してください。", channelName))
	}
	lines = append(lines, "")
	lines = append(lines, persona.ImportantPrompt)
	return strings.Join(lines, "\n")
}
