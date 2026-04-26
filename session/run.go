package session

import (
	"context"
	"errors"
	"fmt"
	"math/rand"
	"net"
	"time"

	anthropic "github.com/anthropics/anthropic-sdk-go"
)

// Run executes the agent loop for this session. It should be called in a
// goroutine. tuiSend must be tea.Program.Send — thread-safe.
func (s *Session) Run(ctx context.Context, client *anthropic.Client, tuiSend func(any)) {
	defer close(s.DoneCh)

	s.setState(StateRunning)
	tuiSend(StateMsg{SessionID: s.ID, NewState: StateRunning})

	systemPrompt := buildSystemPrompt(s.Config.ProjectPath, s.Config.Goal, s.Config.SystemPrompt)
	toolDefs := buildToolDefinitions()

	// Attempt to resume from a saved snapshot.
	startTurn := 0
	messages := []anthropic.MessageParam{
		anthropic.NewUserMessage(anthropic.NewTextBlock(s.Config.Goal)),
	}

	snap, snapErr := loadSnapshot(s.Config)
	if snapErr != nil {
		entry := LogEntry{Kind: LogSystem, Text: fmt.Sprintf("snapshot load error (starting fresh): %v", snapErr)}
		s.appendLog(entry)
		tuiSend(LogMsg{SessionID: s.ID, Entry: entry})
	} else if snap != nil {
		messages = snap.Messages
		startTurn = snap.Turn
		s.mu.Lock()
		s.startedAt = snap.StartedAt
		s.mu.Unlock()
		// Replay saved logs into the session buffer and TUI.
		for _, e := range snap.Logs {
			s.appendLog(e)
			tuiSend(LogMsg{SessionID: s.ID, Entry: e})
		}
		entry := LogEntry{Kind: LogSystem, Text: fmt.Sprintf("resumed from snapshot (turn %d)", startTurn)}
		s.appendLog(entry)
		tuiSend(LogMsg{SessionID: s.ID, Entry: entry})
	} else {
		entry := LogEntry{Kind: LogSystem, Text: fmt.Sprintf("starting session for project: %s", s.Config.ProjectPath)}
		s.appendLog(entry)
		tuiSend(LogMsg{SessionID: s.ID, Entry: entry})
	}

	for turn := startTurn; s.Config.MaxTurns == 0 || turn < s.Config.MaxTurns; turn++ {
		s.setTurn(turn + 1)

		// Check for pause before each API call. Blocks until resumed or ctx cancelled.
		s.mu.RLock()
		ch := s.pauseCh
		s.mu.RUnlock()
		if ch != nil {
			s.setState(StatePaused)
			tuiSend(StateMsg{SessionID: s.ID, NewState: StatePaused})
			entry := LogEntry{Kind: LogSystem, Text: "paused"}
			s.appendLog(entry)
			tuiSend(LogMsg{SessionID: s.ID, Entry: entry})
			select {
			case <-ch:
				s.setState(StateRunning)
				tuiSend(StateMsg{SessionID: s.ID, NewState: StateRunning})
				entry := LogEntry{Kind: LogSystem, Text: "resumed"}
				s.appendLog(entry)
				tuiSend(LogMsg{SessionID: s.ID, Entry: entry})
			case <-ctx.Done():
				s.setErr(ctx.Err())
				s.setState(StateFailed)
				tuiSend(DoneMsg{SessionID: s.ID, Err: ctx.Err()})
				return
			}
		}

		// Check for cancellation before each API call.
		select {
		case <-ctx.Done():
			s.setErr(ctx.Err())
			s.setState(StateFailed)
			tuiSend(DoneMsg{SessionID: s.ID, Err: ctx.Err()})
			return
		default:
		}

		// Apply sliding-window context compression if configured.
		messages = compressContext(messages, s.Config.ContextWindow)

		// Build API params.
		params := anthropic.MessageNewParams{
			Model:     anthropic.Model(s.Config.Model),
			MaxTokens: 8192,
			System: []anthropic.TextBlockParam{
				{Text: systemPrompt},
			},
			Messages: messages,
			Tools:    toolDefs,
		}
		if s.Config.Thinking {
			budget := int64(s.Config.ThinkingBudget)
			// MaxTokens must exceed ThinkingBudget; bump if needed.
			if params.MaxTokens <= budget {
				params.MaxTokens = budget + 4096
			}
			params.Thinking = anthropic.ThinkingConfigParamOfEnabled(budget)
		}

		// Call the API with streaming, retrying on transient errors.
		var (
			accumulated anthropic.Message
			streamErr   error
		)
		maxRetries := s.Config.MaxRetries
		if maxRetries < 0 {
			maxRetries = 0
		}
		for attempt := 0; attempt <= maxRetries; attempt++ {
			if attempt > 0 {
				wait := retryBackoff(attempt-1, s.Config.RetryBaseMs)
				entry := LogEntry{Kind: LogSystem, Text: fmt.Sprintf(
					"API error (attempt %d/%d), retrying in %s…", attempt, maxRetries, wait.Round(time.Millisecond),
				)}
				s.appendLog(entry)
				tuiSend(LogMsg{SessionID: s.ID, Entry: entry})
				select {
				case <-time.After(wait):
				case <-ctx.Done():
					s.setErr(ctx.Err())
					s.setState(StateFailed)
					tuiSend(DoneMsg{SessionID: s.ID, Err: ctx.Err()})
					return
				}
			}

			stream := client.Messages.NewStreaming(ctx, params)
			accumulated = anthropic.Message{}
			streamErr = nil

			for stream.Next() {
				event := stream.Current()
				if err := accumulated.Accumulate(event); err != nil {
					// Log but continue — partial accumulation is recoverable.
					s.appendLog(LogEntry{Kind: LogSystem, Text: fmt.Sprintf("accumulate warning: %v", err)})
				}
				// Forward text deltas in real time.
				if event.Type == "content_block_delta" {
					cbDelta := event.AsContentBlockDelta()
					if cbDelta.Delta.Type == "text_delta" {
						if text := cbDelta.Delta.Text; text != "" {
							entry := LogEntry{Kind: LogText, Text: text}
							s.appendLog(entry)
							tuiSend(LogMsg{SessionID: s.ID, Entry: entry})
						}
					}
				}
			}
			streamErr = stream.Err()

			if streamErr == nil {
				break
			}
			if ctx.Err() != nil {
				s.setErr(ctx.Err())
				s.setState(StateFailed)
				tuiSend(DoneMsg{SessionID: s.ID, Err: ctx.Err()})
				return
			}
			if !isRetryable(streamErr) {
				break
			}
		}
		if streamErr != nil {
			s.setErr(streamErr)
			s.setState(StateFailed)
			entry := LogEntry{Kind: LogSystem, Text: fmt.Sprintf("API error: %v", streamErr)}
			s.appendLog(entry)
			tuiSend(LogMsg{SessionID: s.ID, Entry: entry})
			tuiSend(DoneMsg{SessionID: s.ID, Err: streamErr})
			return
		}

		// Track cumulative token usage.
		s.addTokens(int64(accumulated.Usage.InputTokens), int64(accumulated.Usage.OutputTokens))

		// Append the assistant message to history.
		messages = append(messages, accumulated.ToParam())

		switch accumulated.StopReason {
		case anthropic.StopReasonEndTurn:
			s.setState(StateDone)
			s.deleteSnapshot()
			entry := LogEntry{Kind: LogSystem, Text: "goal complete"}
			s.appendLog(entry)
			tuiSend(LogMsg{SessionID: s.ID, Entry: entry})
			if s.Config.GitHub.CreatePR {
				if prURL, prErr := createGitHubPR(s.Config); prErr != nil {
					e := LogEntry{Kind: LogSystem, Text: fmt.Sprintf("gh pr create failed: %v", prErr)}
					s.appendLog(e)
					tuiSend(LogMsg{SessionID: s.ID, Entry: e})
				} else {
					e := LogEntry{Kind: LogSystem, Text: "PR created: " + prURL}
					s.appendLog(e)
					tuiSend(LogMsg{SessionID: s.ID, Entry: e})
				}
			}
			tuiSend(DoneMsg{SessionID: s.ID})
			return

		case anthropic.StopReasonToolUse:
			// Process all tool_use blocks.
			var toolResults []anthropic.ContentBlockParamUnion

			for _, block := range accumulated.Content {
				toolUse, ok := block.AsAny().(anthropic.ToolUseBlock)
				if !ok {
					continue
				}

				// Log the tool call.
				permDesc := formatPermDesc(toolUse.Name, toolUse.Input)
				entry := LogEntry{Kind: LogToolCall, ToolName: toolUse.Name, Text: permDesc}
				s.appendLog(entry)
				tuiSend(LogMsg{SessionID: s.ID, Entry: entry})

				// Spin detection: auto-pause when the same tool+input repeats consecutively.
				if s.Config.SpinThreshold > 0 {
					inputStr := string(toolUse.Input)
					if toolUse.Name == s.lastToolName && inputStr == s.lastToolInput {
						s.consecutiveCount++
					} else {
						s.consecutiveCount = 1
						s.lastToolName = toolUse.Name
						s.lastToolInput = inputStr
					}
					if s.consecutiveCount >= s.Config.SpinThreshold {
						s.consecutiveCount = 0
						s.lastToolName = ""
						s.lastToolInput = ""
						s.Pause()
						spinEntry := LogEntry{Kind: LogSystem, Text: fmt.Sprintf(
							"spin detected: %s called with identical input %d consecutive times — session paused",
							toolUse.Name, s.Config.SpinThreshold,
						)}
						s.appendLog(spinEntry)
						tuiSend(LogMsg{SessionID: s.ID, Entry: spinEntry})
						toolResults = append(toolResults,
							anthropic.NewToolResultBlock(toolUse.ID, "Session paused: spin detected.", true),
						)
						continue
					}
				}

				// Permission gate.
				approved, err := s.checkPermission(toolUse.Name, toolUse.Input, tuiSend)
				if err != nil || !approved {
					reason := "denied by user"
					if err != nil {
						reason = err.Error()
					}
					toolResults = append(toolResults,
						anthropic.NewToolResultBlock(toolUse.ID, "Tool call "+reason+".", true),
					)
					continue
				}

				// Execute.
				output, execErr := s.executeTool(toolUse.Name, toolUse.Input)
				isError := execErr != nil
				if execErr != nil {
					output = fmt.Sprintf("error: %v", execErr)
				}

				// Trim very long outputs.
				const maxOutput = 20000
				if len(output) > maxOutput {
					output = output[:maxOutput] + "\n[output truncated]"
				}

				resultEntry := LogEntry{Kind: LogToolResult, ToolName: toolUse.Name, Text: output}
				s.appendLog(resultEntry)
				tuiSend(LogMsg{SessionID: s.ID, Entry: resultEntry})

				toolResults = append(toolResults,
					anthropic.NewToolResultBlock(toolUse.ID, output, isError),
				)
			}

			messages = append(messages, anthropic.NewUserMessage(toolResults...))

			// Persist state after each complete tool turn so we can resume on restart.
			if err := s.saveSnapshot(messages); err != nil {
				entry := LogEntry{Kind: LogSystem, Text: fmt.Sprintf("snapshot save error: %v", err)}
				s.appendLog(entry)
				tuiSend(LogMsg{SessionID: s.ID, Entry: entry})
			}

		default:
			// max_tokens, stop_sequence, refusal, pause_turn, etc.
			reason := string(accumulated.StopReason)
			s.setState(StateDone)
			s.deleteSnapshot()
			entry := LogEntry{Kind: LogSystem, Text: fmt.Sprintf("stopped: %s", reason)}
			s.appendLog(entry)
			tuiSend(LogMsg{SessionID: s.ID, Entry: entry})
			tuiSend(DoneMsg{SessionID: s.ID})
			return
		}
	}

	// Max turns reached.
	s.setState(StateDone)
	s.deleteSnapshot()
	entry := LogEntry{Kind: LogSystem, Text: fmt.Sprintf("max turns (%d) reached", s.Config.MaxTurns)}
	s.appendLog(entry)
	tuiSend(LogMsg{SessionID: s.ID, Entry: entry})
	tuiSend(DoneMsg{SessionID: s.ID, Err: fmt.Errorf("max turns (%d) reached", s.Config.MaxTurns)})
}

// isRetryable returns true for transient API and network errors worth retrying.
func isRetryable(err error) bool {
	var apiErr *anthropic.Error
	if errors.As(err, &apiErr) {
		c := apiErr.StatusCode
		return c == 429 || c == 529 || c == 500 || c == 503
	}
	var netErr net.Error
	return errors.As(err, &netErr)
}

// retryBackoff returns the wait duration for the given retry attempt number
// using exponential backoff capped at 30s, plus uniform jitter up to base.
func retryBackoff(attempt, baseMs int) time.Duration {
	base := time.Duration(baseMs) * time.Millisecond
	backoff := base * (1 << attempt)
	if backoff > 30*time.Second {
		backoff = 30 * time.Second
	}
	if base > 0 {
		backoff += time.Duration(rand.Int63n(int64(base)))
	}
	return backoff
}

func buildSystemPrompt(projectPath, goal, extra string) string {
	base := fmt.Sprintf(`You are an autonomous coding agent working on a software project.

PROJECT DIRECTORY: %s
GOAL: %s

You have tools: read_file, write_file, edit_file, list_directory, bash, grep.

Work step by step toward the goal. When complete, say "GOAL COMPLETE" and stop.
Do not ask clarifying questions — use tools to explore and act directly.
Always read files before editing them. Make focused, minimal changes.`, projectPath, goal)
	if extra != "" {
		base += "\n\n" + extra
	}
	return base
}
