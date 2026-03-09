package voice

import "encoding/json"

type SessionConfig struct {
	Instructions            string
	Voice                   string
	InputAudioFormat        string
	InputSampleRate         int
	InputTranscriptionModel string
	OutputAudioFormat       string
	OutputSampleRate        int
	TurnDetection           string
	TurnDetectionEagerness  string
	CreateResponse          bool
	InterruptResponse       bool
}

type ServerEvent struct {
	Type string          `json:"type"`
	Raw  json.RawMessage `json:"-"`
}

func (e *ServerEvent) UnmarshalJSON(data []byte) error {
	type payload struct {
		Type string `json:"type"`
	}
	var aux payload
	if err := json.Unmarshal(data, &aux); err != nil {
		return err
	}
	e.Type = aux.Type
	e.Raw = append(e.Raw[:0], data...)
	return nil
}

func (e ServerEvent) decode(target any) bool {
	if len(e.Raw) == 0 {
		return false
	}
	return json.Unmarshal(e.Raw, target) == nil
}

func (e ServerEvent) transcript() string {
	var payload struct {
		Transcript string `json:"transcript"`
		Text       string `json:"text"`
		Delta      string `json:"delta"`
	}
	if !e.decode(&payload) {
		return ""
	}
	switch {
	case payload.Transcript != "":
		return payload.Transcript
	case payload.Text != "":
		return payload.Text
	default:
		return payload.Delta
	}
}

func (e ServerEvent) conversationItemID() string {
	var payload struct {
		ItemID             string `json:"item_id"`
		ConversationItemID string `json:"conversation_item_id"`
	}
	if !e.decode(&payload) {
		return ""
	}
	if payload.ConversationItemID != "" {
		return payload.ConversationItemID
	}
	return payload.ItemID
}

func (e ServerEvent) responseID() string {
	var payload struct {
		ResponseID string `json:"response_id"`
	}
	if !e.decode(&payload) {
		return ""
	}
	return payload.ResponseID
}

func (e ServerEvent) audioDelta() string {
	var payload struct {
		Delta string `json:"delta"`
	}
	if !e.decode(&payload) {
		return ""
	}
	return payload.Delta
}

func (e ServerEvent) errorInfo() (string, string, string) {
	var payload struct {
		Error struct {
			Code    string `json:"code"`
			Param   string `json:"param"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if !e.decode(&payload) {
		return "", "", ""
	}
	return payload.Error.Code, payload.Error.Param, payload.Error.Message
}

type SessionSettings struct {
	Voice                   string
	Instructions            string
	TurnDetection           string
	TurnDetectionEagerness  string
	InputTranscriptionModel string
	CreateResponse          bool
	InterruptResponse       bool
}

func (e ServerEvent) sessionSettings() SessionSettings {
	var nested struct {
		Session struct {
			Instructions string `json:"instructions"`
			Audio        struct {
				Input struct {
					TurnDetection struct {
						Type              string `json:"type"`
						Eagerness         string `json:"eagerness"`
						CreateResponse    bool   `json:"create_response"`
						InterruptResponse bool   `json:"interrupt_response"`
					} `json:"turn_detection"`
					Transcription struct {
						Model string `json:"model"`
					} `json:"transcription"`
				} `json:"input"`
				Output struct {
					Voice string `json:"voice"`
				} `json:"output"`
			} `json:"audio"`
			Voice string `json:"voice"`
		} `json:"session"`
	}
	if e.decode(&nested) {
		settings := SessionSettings{
			Voice:                   nested.Session.Audio.Output.Voice,
			Instructions:            nested.Session.Instructions,
			TurnDetection:           nested.Session.Audio.Input.TurnDetection.Type,
			TurnDetectionEagerness:  nested.Session.Audio.Input.TurnDetection.Eagerness,
			InputTranscriptionModel: nested.Session.Audio.Input.Transcription.Model,
			CreateResponse:          nested.Session.Audio.Input.TurnDetection.CreateResponse,
			InterruptResponse:       nested.Session.Audio.Input.TurnDetection.InterruptResponse,
		}
		if settings.Voice == "" {
			settings.Voice = nested.Session.Voice
		}
		if settings.Voice != "" || settings.Instructions != "" || settings.TurnDetection != "" {
			return settings
		}
	}

	var legacy struct {
		Session struct {
			Instructions  string `json:"instructions"`
			Voice         string `json:"voice"`
			TurnDetection struct {
				Type              string `json:"type"`
				Eagerness         string `json:"eagerness"`
				CreateResponse    bool   `json:"create_response"`
				InterruptResponse bool   `json:"interrupt_response"`
			} `json:"turn_detection"`
			InputAudioTranscription struct {
				Model string `json:"model"`
			} `json:"input_audio_transcription"`
		} `json:"session"`
	}
	if e.decode(&legacy) {
		return SessionSettings{
			Voice:                   legacy.Session.Voice,
			Instructions:            legacy.Session.Instructions,
			TurnDetection:           legacy.Session.TurnDetection.Type,
			TurnDetectionEagerness:  legacy.Session.TurnDetection.Eagerness,
			InputTranscriptionModel: legacy.Session.InputAudioTranscription.Model,
			CreateResponse:          legacy.Session.TurnDetection.CreateResponse,
			InterruptResponse:       legacy.Session.TurnDetection.InterruptResponse,
		}
	}

	return SessionSettings{}
}

func (e ServerEvent) outputItem() (string, string) {
	var payload struct {
		Item struct {
			ID   string `json:"id"`
			Role string `json:"role"`
		} `json:"item"`
		ItemID string `json:"item_id"`
	}
	if !e.decode(&payload) {
		return "", ""
	}
	if payload.Item.ID != "" {
		return payload.Item.ID, payload.Item.Role
	}
	return payload.ItemID, ""
}
