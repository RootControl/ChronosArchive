package session

import anthropic "github.com/anthropics/anthropic-sdk-go"

// compressContext applies a sliding-window strategy when windowSize > 0 and
// len(messages) > 2×windowSize. It keeps the first message (the original goal)
// plus the most recent messages, dropping the middle to stay within context
// budget. The first message is always preserved so the model retains the
// original task.
//
// The kept tail always begins on an assistant message. That is what keeps the
// compressed history valid for the API: tool_result blocks live in user
// messages and must be preceded by the assistant message carrying the matching
// tool_use, so a tail starting mid-pair would orphan them and the request would
// be rejected. Starting on an assistant message also preserves user/assistant
// alternation with the retained goal message.
//
// windowSize == 0 disables compression (returns messages unchanged).
func compressContext(messages []anthropic.MessageParam, windowSize int) []anthropic.MessageParam {
	if windowSize <= 0 || len(messages) <= windowSize*2 {
		return messages
	}

	// Advance to the first assistant message at or after the desired boundary.
	start := len(messages) - windowSize
	for start < len(messages) && messages[start].Role != anthropic.MessageParamRoleAssistant {
		start++
	}
	// No assistant message in the tail — nothing safe to cut.
	if start >= len(messages) {
		return messages
	}
	// Cutting would keep everything anyway; skip the copy.
	if start <= 1 {
		return messages
	}

	compressed := make([]anthropic.MessageParam, 0, 1+len(messages)-start)
	compressed = append(compressed, messages[0])
	compressed = append(compressed, messages[start:]...)
	return compressed
}
