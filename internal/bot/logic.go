package bot

import "strings"

const noReplyToken = "<NO_REPLY>"

type assistantAction string

const (
	assistantActionIgnore assistantAction = "ignore"
	assistantActionReply  assistantAction = "reply"
)

type assistantReply struct {
	Action  assistantAction
	Reason  string
	Message string
}

func parseAssistantReply(raw string) assistantReply {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" || strings.EqualFold(trimmed, noReplyToken) {
		return assistantReply{
			Action: assistantActionIgnore,
			Reason: "codex selected silence",
		}
	}
	return assistantReply{
		Action:  assistantActionReply,
		Reason:  "codex text reply",
		Message: trimmed,
	}
}
