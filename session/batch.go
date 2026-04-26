package session

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	anthropic "github.com/anthropics/anthropic-sdk-go"
)

// batchResults holds the outcome of a completed batch poll.
type batchResults struct {
	// succeeded maps sessionID → the API response message.
	succeeded map[string]anthropic.Message
	// failed maps sessionID → the error that caused the failure.
	failed map[string]error
	// ctxErr is set (and succeeded/failed are empty) when the context was cancelled.
	ctxErr error
}

// RunBatch submits all sessions as a single Anthropic Message Batch request,
// polls until the batch is complete, then delivers results to each session.
// All turns — including tool-use continuations — run through the Batch API
// (50% cost). Sessions never fall back to streaming.
//
// Call in a goroutine. tuiSend must be tea.Program.Send — thread-safe.
func RunBatch(ctx context.Context, client *anthropic.Client, sessions []*Session, tuiSend func(any)) {
	if len(sessions) == 0 {
		return
	}

	// Build one batch request per session.
	requests := make([]anthropic.MessageBatchNewParamsRequest, len(sessions))
	for i, s := range sessions {
		s.setState(StateRunning)
		tuiSend(StateMsg{SessionID: s.ID, NewState: StateRunning})
		entry := LogEntry{Kind: LogSystem, Text: fmt.Sprintf("queued for batch (project: %s)", s.Config.ProjectPath)}
		s.appendLog(entry)
		tuiSend(LogMsg{SessionID: s.ID, Entry: entry})

		params := anthropic.MessageBatchNewParamsRequestParams{
			Model:     anthropic.Model(s.Config.Model),
			MaxTokens: 8192,
			System: []anthropic.TextBlockParam{
				{Text: buildSystemPrompt(s.Config.ProjectPath, s.Config.Goal, s.Config.SystemPrompt)},
			},
			Messages: []anthropic.MessageParam{
				anthropic.NewUserMessage(anthropic.NewTextBlock(s.Config.Goal)),
			},
			Tools: buildToolDefinitions(),
		}
		if s.Config.Thinking {
			budget := int64(s.Config.ThinkingBudget)
			if params.MaxTokens <= budget {
				params.MaxTokens = budget + 4096
			}
			params.Thinking = anthropic.ThinkingConfigParamOfEnabled(budget)
		}

		requests[i] = anthropic.MessageBatchNewParamsRequest{
			CustomID: s.ID,
			Params:   params,
		}
	}

	// Submit the batch.
	batch, err := client.Messages.Batches.New(ctx, anthropic.MessageBatchNewParams{
		Requests: requests,
	})
	if err != nil {
		for _, s := range sessions {
			s.setErr(err)
			s.setState(StateFailed)
			entry := LogEntry{Kind: LogSystem, Text: fmt.Sprintf("batch submit error: %v", err)}
			s.appendLog(entry)
			tuiSend(LogMsg{SessionID: s.ID, Entry: entry})
			tuiSend(DoneMsg{SessionID: s.ID, Err: err})
			close(s.DoneCh)
		}
		return
	}

	batchID := batch.ID
	for _, s := range sessions {
		entry := LogEntry{Kind: LogSystem, Text: fmt.Sprintf("batch submitted (id: %s) — polling for results…", batchID)}
		s.appendLog(entry)
		tuiSend(LogMsg{SessionID: s.ID, Entry: entry})
	}

	byID := make(map[string]*Session, len(sessions))
	for _, s := range sessions {
		byID[s.ID] = s
	}

	res := pollBatch(ctx, client, batchID, sessions, tuiSend)

	// Context was cancelled — close DoneCh for all sessions.
	if res.ctxErr != nil {
		for _, s := range byID {
			s.setErr(res.ctxErr)
			s.setState(StateFailed)
			tuiSend(DoneMsg{SessionID: s.ID, Err: res.ctxErr})
			close(s.DoneCh)
		}
		return
	}

	// Close DoneCh for sessions that failed during the batch.
	for id, err := range res.failed {
		s := byID[id]
		s.setErr(err)
		s.setState(StateFailed)
		entry := LogEntry{Kind: LogSystem, Text: err.Error()}
		s.appendLog(entry)
		tuiSend(LogMsg{SessionID: s.ID, Entry: entry})
		tuiSend(DoneMsg{SessionID: s.ID, Err: err})
		close(s.DoneCh)
	}

	// Dispatch succeeded results.
	for id, msg := range res.succeeded {
		s := byID[id]

		// Log any text from this turn.
		for _, block := range msg.Content {
			if tb, ok := block.AsAny().(anthropic.TextBlock); ok && tb.Text != "" {
				entry := LogEntry{Kind: LogText, Text: tb.Text}
				s.appendLog(entry)
				tuiSend(LogMsg{SessionID: s.ID, Entry: entry})
			}
		}

		switch msg.StopReason {
		case anthropic.StopReasonEndTurn:
			finishBatchSession(s, tuiSend)
			close(s.DoneCh)

		case anthropic.StopReasonToolUse:
			entry := LogEntry{Kind: LogSystem, Text: "batch response received — continuing with tool execution (batch mode)"}
			s.appendLog(entry)
			tuiSend(LogMsg{SessionID: s.ID, Entry: entry})
			initialMessages := []anthropic.MessageParam{
				anthropic.NewUserMessage(anthropic.NewTextBlock(s.Config.Goal)),
				msg.ToParam(),
			}
			// DoneCh ownership transfers to the goroutine.
			go s.runBatchAgentLoop(ctx, client, initialMessages, msg, 1, tuiSend)

		default:
			reason := string(msg.StopReason)
			s.setState(StateDone)
			s.deleteSnapshot()
			entry := LogEntry{Kind: LogSystem, Text: fmt.Sprintf("batch result: stopped (%s)", reason)}
			s.appendLog(entry)
			tuiSend(LogMsg{SessionID: s.ID, Entry: entry})
			tuiSend(DoneMsg{SessionID: s.ID})
			close(s.DoneCh)
		}
	}
}

// runBatchAgentLoop continues a session's agent loop using only the Batch API.
// Each tool-use turn is submitted as a new single-item batch request and polled
// to completion. It closes s.DoneCh when done.
func (s *Session) runBatchAgentLoop(ctx context.Context, client *anthropic.Client, messages []anthropic.MessageParam, lastResponse anthropic.Message, startTurn int, tuiSend func(any)) {
	defer close(s.DoneCh)

	systemPrompt := buildSystemPrompt(s.Config.ProjectPath, s.Config.Goal, s.Config.SystemPrompt)
	toolDefs := buildToolDefinitions()
	currentResponse := lastResponse

	for turn := startTurn; s.Config.MaxTurns == 0 || turn < s.Config.MaxTurns; turn++ {
		s.setTurn(turn + 1)

		// Check for pause.
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

		select {
		case <-ctx.Done():
			s.setErr(ctx.Err())
			s.setState(StateFailed)
			tuiSend(DoneMsg{SessionID: s.ID, Err: ctx.Err()})
			return
		default:
		}

		// Execute tools from the last assistant response and append results.
		toolResults := s.executeToolsFromResponse(currentResponse, tuiSend)
		messages = append(messages, anthropic.NewUserMessage(toolResults...))
		messages = compressContext(messages, s.Config.ContextWindow)

		if err := s.saveSnapshot(messages); err != nil {
			entry := LogEntry{Kind: LogSystem, Text: fmt.Sprintf("snapshot save error: %v", err)}
			s.appendLog(entry)
			tuiSend(LogMsg{SessionID: s.ID, Entry: entry})
		}

		// Submit a new single-item batch for this turn.
		params := anthropic.MessageBatchNewParamsRequestParams{
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
			if params.MaxTokens <= budget {
				params.MaxTokens = budget + 4096
			}
			params.Thinking = anthropic.ThinkingConfigParamOfEnabled(budget)
		}

		batch, err := client.Messages.Batches.New(ctx, anthropic.MessageBatchNewParams{
			Requests: []anthropic.MessageBatchNewParamsRequest{
				{CustomID: s.ID, Params: params},
			},
		})
		if err != nil {
			s.setErr(err)
			s.setState(StateFailed)
			entry := LogEntry{Kind: LogSystem, Text: fmt.Sprintf("batch submit error: %v", err)}
			s.appendLog(entry)
			tuiSend(LogMsg{SessionID: s.ID, Entry: entry})
			tuiSend(DoneMsg{SessionID: s.ID, Err: err})
			return
		}

		entry := LogEntry{Kind: LogSystem, Text: fmt.Sprintf("batch turn %d submitted (id: %s) — polling…", turn+1, batch.ID)}
		s.appendLog(entry)
		tuiSend(LogMsg{SessionID: s.ID, Entry: entry})

		// Poll this single-item batch to completion.
		// pollBatch never closes DoneCh — that's our defer's job.
		res := pollBatch(ctx, client, batch.ID, []*Session{s}, tuiSend)

		if res.ctxErr != nil {
			s.setErr(res.ctxErr)
			s.setState(StateFailed)
			tuiSend(DoneMsg{SessionID: s.ID, Err: res.ctxErr})
			return
		}
		if err, bad := res.failed[s.ID]; bad {
			s.setErr(err)
			s.setState(StateFailed)
			entry := LogEntry{Kind: LogSystem, Text: err.Error()}
			s.appendLog(entry)
			tuiSend(LogMsg{SessionID: s.ID, Entry: entry})
			tuiSend(DoneMsg{SessionID: s.ID, Err: err})
			return
		}

		msg := res.succeeded[s.ID]
		s.addTokens(int64(msg.Usage.InputTokens), int64(msg.Usage.OutputTokens))
		messages = append(messages, msg.ToParam())

		// Log any text content.
		for _, block := range msg.Content {
			if tb, ok := block.AsAny().(anthropic.TextBlock); ok && tb.Text != "" {
				logEntry := LogEntry{Kind: LogText, Text: tb.Text}
				s.appendLog(logEntry)
				tuiSend(LogMsg{SessionID: s.ID, Entry: logEntry})
			}
		}

		switch msg.StopReason {
		case anthropic.StopReasonEndTurn:
			finishBatchSession(s, tuiSend) // DoneCh closed by defer
			return

		case anthropic.StopReasonToolUse:
			currentResponse = msg
			if err := s.saveSnapshot(messages); err != nil {
				logEntry := LogEntry{Kind: LogSystem, Text: fmt.Sprintf("snapshot save error: %v", err)}
				s.appendLog(logEntry)
				tuiSend(LogMsg{SessionID: s.ID, Entry: logEntry})
			}

		default:
			reason := string(msg.StopReason)
			s.setState(StateDone)
			s.deleteSnapshot()
			logEntry := LogEntry{Kind: LogSystem, Text: fmt.Sprintf("stopped: %s", reason)}
			s.appendLog(logEntry)
			tuiSend(LogMsg{SessionID: s.ID, Entry: logEntry})
			tuiSend(DoneMsg{SessionID: s.ID})
			return
		}
	}

	s.setState(StateDone)
	s.deleteSnapshot()
	entry := LogEntry{Kind: LogSystem, Text: fmt.Sprintf("max turns (%d) reached", s.Config.MaxTurns)}
	s.appendLog(entry)
	tuiSend(LogMsg{SessionID: s.ID, Entry: entry})
	tuiSend(DoneMsg{SessionID: s.ID, Err: fmt.Errorf("max turns (%d) reached", s.Config.MaxTurns)})
}

// pollBatch polls a batch until it ends and returns the results.
// It never closes any session's DoneCh — callers are responsible for that.
func pollBatch(ctx context.Context, client *anthropic.Client, batchID string, sessions []*Session, tuiSend func(any)) batchResults {
	pollInterval := 10 * time.Second
	for {
		select {
		case <-ctx.Done():
			return batchResults{ctxErr: ctx.Err()}
		case <-time.After(pollInterval):
		}

		status, err := client.Messages.Batches.Get(ctx, batchID)
		if err != nil {
			for _, s := range sessions {
				entry := LogEntry{Kind: LogSystem, Text: fmt.Sprintf("poll error (will retry): %v", err)}
				s.appendLog(entry)
				tuiSend(LogMsg{SessionID: s.ID, Entry: entry})
			}
			continue
		}

		counts := status.RequestCounts
		total := int(counts.Processing + counts.Succeeded + counts.Errored + counts.Canceled + counts.Expired)
		pending := int(counts.Processing)
		succeeded := int(counts.Succeeded)
		for _, s := range sessions {
			s.SetBatchProgress(total, succeeded, pending)
			entry := LogEntry{
				Kind: LogSystem,
				Text: fmt.Sprintf("batch status: %s (processing:%d succeeded:%d errored:%d)",
					status.ProcessingStatus, counts.Processing, counts.Succeeded, counts.Errored),
			}
			s.appendLog(entry)
			tuiSend(LogMsg{SessionID: s.ID, Entry: entry})
		}

		if status.ProcessingStatus != anthropic.MessageBatchProcessingStatusEnded {
			if pollInterval < 60*time.Second {
				pollInterval *= 2
				if pollInterval > 60*time.Second {
					pollInterval = 60 * time.Second
				}
			}
			continue
		}

		break
	}

	// Build a quick lookup so we can match results to sessions.
	byID := make(map[string]*Session, len(sessions))
	for _, s := range sessions {
		byID[s.ID] = s
	}

	res := batchResults{
		succeeded: make(map[string]anthropic.Message),
		failed:    make(map[string]error),
	}

	stream := client.Messages.Batches.ResultsStreaming(ctx, batchID)
	for stream.Next() {
		resp := stream.Current()
		if _, ok := byID[resp.CustomID]; !ok {
			continue
		}

		switch variant := resp.Result.AsAny().(type) {
		case anthropic.MessageBatchSucceededResult:
			res.succeeded[resp.CustomID] = variant.Message

		case anthropic.MessageBatchErroredResult:
			res.failed[resp.CustomID] = fmt.Errorf("batch request errored: %s", variant.Error.Error.Message)

		case anthropic.MessageBatchCanceledResult:
			res.failed[resp.CustomID] = fmt.Errorf("batch request was canceled")

		case anthropic.MessageBatchExpiredResult:
			res.failed[resp.CustomID] = fmt.Errorf("batch request expired (>24h)")

		default:
			res.failed[resp.CustomID] = fmt.Errorf("unknown batch result type")
		}
	}

	if err := stream.Err(); err != nil && ctx.Err() == nil {
		// Mark any sessions whose result was not received as failed.
		for id := range byID {
			if _, ok := res.succeeded[id]; !ok {
				if _, ok := res.failed[id]; !ok {
					res.failed[id] = fmt.Errorf("results stream error: %v", err)
				}
			}
		}
	}

	return res
}

// finishBatchSession marks a session as done and handles optional PR creation.
// It does NOT close s.DoneCh — callers are responsible for that.
func finishBatchSession(s *Session, tuiSend func(any)) {
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
}

// executeToolsFromResponse executes all tool_use blocks in an assistant
// response and returns the tool result blocks for the next user message.
func (s *Session) executeToolsFromResponse(msg anthropic.Message, tuiSend func(any)) []anthropic.ContentBlockParamUnion {
	var toolResults []anthropic.ContentBlockParamUnion

	for _, block := range msg.Content {
		toolUse, ok := block.AsAny().(anthropic.ToolUseBlock)
		if !ok {
			continue
		}

		rawInput, err := json.Marshal(toolUse.Input)
		if err != nil {
			rawInput = []byte("{}")
		}

		permDesc := formatPermDesc(toolUse.Name, rawInput)
		entry := LogEntry{Kind: LogToolCall, ToolName: toolUse.Name, Text: permDesc}
		s.appendLog(entry)
		tuiSend(LogMsg{SessionID: s.ID, Entry: entry})

		approved, err := s.checkPermission(toolUse.Name, rawInput, tuiSend)
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

		output, execErr := s.executeTool(toolUse.Name, rawInput)
		isError := execErr != nil
		if execErr != nil {
			output = fmt.Sprintf("error: %v", execErr)
		}

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

	return toolResults
}
