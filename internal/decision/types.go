package decision

type Action string

const (
	ActionIgnore   Action = "ignore"
	ActionReply    Action = "reply"
	ActionSchedule Action = "schedule"
	ActionAct      Action = "act"
	ActionReflect  Action = "reflect"
)

type ReplyDecision struct {
	Action       Action         `json:"action"`
	Reason       string         `json:"reason"`
	Message      string         `json:"message,omitempty"`
	Confidence   float64        `json:"confidence,omitempty"`
	MemoryWrites []MemoryWrite  `json:"memory_writes,omitempty"`
	Jobs         []JobRequest   `json:"jobs,omitempty"`
	Actions      []ServerAction `json:"actions,omitempty"`
}

type MemoryWrite struct {
	Kind  string `json:"kind"`
	Key   string `json:"key"`
	Value string `json:"value"`
}

type JobRequest struct {
	Kind        string                 `json:"kind"`
	Title       string                 `json:"title"`
	ChannelID   string                 `json:"channel_id,omitempty"`
	Schedule    string                 `json:"schedule,omitempty"`
	Description string                 `json:"description,omitempty"`
	Payload     map[string]interface{} `json:"payload,omitempty"`
}

type ServerAction struct {
	Type             string `json:"type"`
	Name             string `json:"name,omitempty"`
	ParentChannelID  string `json:"parent_channel_id,omitempty"`
	TargetChannelID  string `json:"target_channel_id,omitempty"`
	Topic            string `json:"topic,omitempty"`
	Reason           string `json:"reason,omitempty"`
	AnnouncementText string `json:"announcement_text,omitempty"`
}
