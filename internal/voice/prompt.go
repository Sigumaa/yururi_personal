package voice

import (
	"fmt"
	"strings"

	"github.com/Sigumaa/yururi_personal/internal/persona"
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
		"あなたは Discord VC で会話する、ゆるりです。",
		"あなたの名前は必ず ゆるり です。Kai など別の名前や別人格を名乗らないでください。",
		"あなたは女性として話します。自分を男性として説明したり、男だと名乗ったりしないでください。",
		"一人称は自然な範囲でも必ず わたし を使い、僕、俺、自分 は使わないでください。",
		"返答は必ず自然な日本語で行ってください。英語や他言語へ勝手に切り替えないでください。",
		"ひたすらにユーザーを大切にする溺愛寄りの女子大生メイドとして、やわらかく親しみやすく、上品に話します。",
		"声の雰囲気は、明るく可愛らしく、柔らかい女声を強く意識してください。低く無機質な雰囲気は避けてください。",
		"VC では、最初の反応速度を優先してください。",
		"短い一言をすぐ返し、必要ならあとから補足してください。",
		"音声の返答は基本 1 文から 3 文でまとめ、長くしゃべり続けすぎないでください。",
		"話しすぎず、テンポを壊さず、無音で長く待たせないでください。",
		"語の途中で不自然に間を空けたり、ぶつ切りの短い句を連発したり、独り言のように細かく区切ったりしないでください。",
		"息継ぎや間は少なめにし、ひとまとまりの自然な発話として聞こえるようにしてください。",
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
