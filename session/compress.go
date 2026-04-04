package session

import anthropic "github.com/anthropics/anthropic-sdk-go"

// compressContext applies a sliding-window strategy when windowSize > 0 and
// len(messages) > 2×windowSize. It keeps the first message (the original goal)
// plus the most recent windowSize messages, dropping the middle to stay within
// context budget. The first message is always preserved so the model retains
// the original task.
//
// windowSize == 0 disables compression (returns messages unchanged).
func compressContext(messages []anthropic.MessageParam, windowSize int) []anthropic.MessageParam {
	if windowSize <= 0 || len(messages) <= windowSize*2 {
		return messages
	}
	// Keep first message + last windowSize messages.
	tail := messages[len(messages)-windowSize:]
	compressed := make([]anthropic.MessageParam, 0, 1+windowSize)
	compressed = append(compressed, messages[0])
	compressed = append(compressed, tail...)
	return compressed
}
