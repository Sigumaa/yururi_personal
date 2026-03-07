package presence

import (
	"fmt"
	"strings"
	"time"
)

type Activity struct {
	Name          string     `json:"name"`
	Type          string     `json:"type"`
	URL           string     `json:"url,omitempty"`
	ApplicationID string     `json:"application_id,omitempty"`
	State         string     `json:"state,omitempty"`
	Details       string     `json:"details,omitempty"`
	LargeText     string     `json:"large_text,omitempty"`
	SmallText     string     `json:"small_text,omitempty"`
	StartAt       *time.Time `json:"start_at,omitempty"`
	EndAt         *time.Time `json:"end_at,omitempty"`
}

func SummaryList(items []Activity) []string {
	if len(items) == 0 {
		return nil
	}
	out := make([]string, 0, len(items))
	for _, item := range items {
		out = append(out, Summary(item))
	}
	return out
}

func Summary(item Activity) string {
	parts := make([]string, 0, 4)
	if strings.TrimSpace(item.Type) != "" {
		parts = append(parts, item.Type)
	}
	if strings.TrimSpace(item.Name) != "" {
		parts = append(parts, item.Name)
	}

	main := strings.Join(parts, " / ")
	switch {
	case strings.TrimSpace(item.Details) != "" && strings.TrimSpace(item.State) != "":
		main = fmt.Sprintf("%s (%s - %s)", main, item.Details, item.State)
	case strings.TrimSpace(item.Details) != "":
		main = fmt.Sprintf("%s (%s)", main, item.Details)
	case strings.TrimSpace(item.State) != "":
		main = fmt.Sprintf("%s (%s)", main, item.State)
	}
	return strings.TrimSpace(main)
}

func Describe(item Activity) string {
	lines := []string{
		fmt.Sprintf("- type=%s name=%s", emptyToNone(item.Type), emptyToNone(item.Name)),
	}
	if strings.TrimSpace(item.Details) != "" {
		lines = append(lines, "  details="+item.Details)
	}
	if strings.TrimSpace(item.State) != "" {
		lines = append(lines, "  state="+item.State)
	}
	if strings.TrimSpace(item.LargeText) != "" {
		lines = append(lines, "  large_text="+item.LargeText)
	}
	if strings.TrimSpace(item.SmallText) != "" {
		lines = append(lines, "  small_text="+item.SmallText)
	}
	if strings.TrimSpace(item.URL) != "" {
		lines = append(lines, "  url="+item.URL)
	}
	if strings.TrimSpace(item.ApplicationID) != "" {
		lines = append(lines, "  application_id="+item.ApplicationID)
	}
	if item.StartAt != nil {
		lines = append(lines, "  start_at="+item.StartAt.UTC().Format(time.RFC3339))
	}
	if item.EndAt != nil {
		lines = append(lines, "  end_at="+item.EndAt.UTC().Format(time.RFC3339))
	}
	return strings.Join(lines, "\n")
}

func DescribeList(items []Activity) string {
	if len(items) == 0 {
		return "- none"
	}
	lines := make([]string, 0, len(items))
	for _, item := range items {
		lines = append(lines, Describe(item))
	}
	return strings.Join(lines, "\n")
}

func emptyToNone(value string) string {
	if strings.TrimSpace(value) == "" {
		return "none"
	}
	return value
}
