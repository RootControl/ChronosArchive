package session

import (
	"testing"

	anthropic "github.com/anthropics/anthropic-sdk-go"
)

// buildHistory produces a realistic agent-loop history:
// user(goal), then alternating assistant(tool_use) / user(tool_result) turns.
func buildHistory(turns int) []anthropic.MessageParam {
	msgs := []anthropic.MessageParam{
		anthropic.NewUserMessage(anthropic.NewTextBlock("goal")),
	}
	for i := range turns {
		id := string(rune('a' + i))
		msgs = append(msgs,
			anthropic.NewAssistantMessage(anthropic.NewTextBlock("thinking "+id)),
			anthropic.NewUserMessage(anthropic.NewToolResultBlock(id, "result "+id, false)),
		)
	}
	return msgs
}

// hasToolResult reports whether a message carries any tool_result block.
func hasToolResult(m anthropic.MessageParam) bool {
	for _, b := range m.Content {
		if b.OfToolResult != nil {
			return true
		}
	}
	return false
}

func TestCompressContextDisabled(t *testing.T) {
	msgs := buildHistory(10)
	got := compressContext(msgs, 0)
	if len(got) != len(msgs) {
		t.Fatalf("windowSize 0 should be a no-op, got %d want %d", len(got), len(msgs))
	}
}

func TestCompressContextBelowThreshold(t *testing.T) {
	msgs := buildHistory(2) // 5 messages
	got := compressContext(msgs, 5)
	if len(got) != len(msgs) {
		t.Fatalf("history at/below 2x window should be untouched, got %d want %d", len(got), len(msgs))
	}
}

// The core invariant: a compressed history must never begin its tail on a user
// message full of tool_result blocks whose matching tool_use was just dropped.
// The API rejects that with a 400, which is what the old sliding window
// produced for every even window size.
func TestCompressContextNeverOrphansToolResults(t *testing.T) {
	for _, turns := range []int{6, 7, 8, 12, 20} {
		for window := 1; window <= 10; window++ {
			msgs := buildHistory(turns)
			got := compressContext(msgs, window)

			if len(got) == len(msgs) {
				continue // no compression applied
			}
			if got[0].Role != anthropic.MessageParamRoleUser {
				t.Fatalf("turns=%d window=%d: first message must remain the goal", turns, window)
			}
			if hasToolResult(got[1]) {
				t.Fatalf("turns=%d window=%d: tail starts with orphaned tool_result", turns, window)
			}
			if got[1].Role != anthropic.MessageParamRoleAssistant {
				t.Fatalf("turns=%d window=%d: tail must start on an assistant message, got %q",
					turns, window, got[1].Role)
			}
			// Roles must alternate for the whole compressed history.
			for i := 1; i < len(got); i++ {
				if got[i].Role == got[i-1].Role {
					t.Fatalf("turns=%d window=%d: consecutive %q messages at index %d",
						turns, window, got[i].Role, i)
				}
			}
		}
	}
}

func TestCompressContextKeepsRecentTail(t *testing.T) {
	msgs := buildHistory(20) // 41 messages
	got := compressContext(msgs, 4)

	if len(got) >= len(msgs) {
		t.Fatalf("expected compression, got %d of %d messages", len(got), len(msgs))
	}
	// The most recent message must always survive.
	last, gotLast := msgs[len(msgs)-1], got[len(got)-1]
	if last.Role != gotLast.Role || len(last.Content) != len(gotLast.Content) {
		t.Fatal("most recent message was dropped")
	}
}
