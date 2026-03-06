package codex

type InputItem struct {
	Type         string        `json:"type"`
	Text         string        `json:"text,omitempty"`
	TextElements []TextElement `json:"text_elements,omitempty"`
	URL          string        `json:"url,omitempty"`
	Path         string        `json:"path,omitempty"`
	Name         string        `json:"name,omitempty"`
}

type TextElement struct {
	Type string `json:"type"`
	Text string `json:"text,omitempty"`
}

func TextInput(text string) InputItem {
	return InputItem{
		Type: "text",
		Text: text,
	}
}

func ImageInput(url string) InputItem {
	return InputItem{
		Type: "image",
		URL:  url,
	}
}
