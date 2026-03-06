package bot

import "testing"

func TestConversationContextHasNoDeadline(t *testing.T) {
	ctx, cancel := conversationContext()
	defer cancel()

	if _, ok := ctx.Deadline(); ok {
		t.Fatal("expected conversation context without deadline")
	}
}
