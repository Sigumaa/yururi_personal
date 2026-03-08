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
