package session

import (
	"context"
	"fmt"
	"time"

	anthropic "github.com/anthropics/anthropic-sdk-go"
)

// RunBatch submits all sessions as a single Anthropic Message Batch request,
// polls until the batch is complete, then delivers results to each session.
// Each session gets a single-turn response (no tool loop). This costs 50% less
// than the standard Messages API.
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
				{Text: buildSystemPrompt(s.Config.ProjectPath, s.Config.Goal)},
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

	// Build a map for quick session lookup by custom_id (= session ID).
	byID := make(map[string]*Session, len(sessions))
	for _, s := range sessions {
		byID[s.ID] = s
	}

	// Poll until the batch ends (status == "ended") or context is cancelled.
	pollInterval := 10 * time.Second
	for {
		select {
		case <-ctx.Done():
			for _, s := range byID {
				s.setErr(ctx.Err())
				s.setState(StateFailed)
				tuiSend(DoneMsg{SessionID: s.ID, Err: ctx.Err()})
				close(s.DoneCh)
			}
			return
		case <-time.After(pollInterval):
		}

		status, err := client.Messages.Batches.Get(ctx, batchID)
		if err != nil {
			// Transient poll error — log and retry.
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
			// Increase polling interval up to 60s to avoid hammering the API.
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

	// Stream results and dispatch to each session.
	stream := client.Messages.Batches.ResultsStreaming(ctx, batchID)
	for stream.Next() {
		resp := stream.Current()
		s, ok := byID[resp.CustomID]
		if !ok {
			continue
		}

		switch variant := resp.Result.AsAny().(type) {
		case anthropic.MessageBatchSucceededResult:
			for _, block := range variant.Message.Content {
				if tb, ok := block.AsAny().(anthropic.TextBlock); ok && tb.Text != "" {
					entry := LogEntry{Kind: LogText, Text: tb.Text}
					s.appendLog(entry)
					tuiSend(LogMsg{SessionID: s.ID, Entry: entry})
				}
			}
			s.setState(StateDone)
			doneEntry := LogEntry{Kind: LogSystem, Text: "batch result received — goal complete"}
			s.appendLog(doneEntry)
			tuiSend(LogMsg{SessionID: s.ID, Entry: doneEntry})
			tuiSend(DoneMsg{SessionID: s.ID})

		case anthropic.MessageBatchErroredResult:
			apiErr := fmt.Errorf("batch request errored: %s", variant.Error.Error.Message)
			s.setErr(apiErr)
			s.setState(StateFailed)
			entry := LogEntry{Kind: LogSystem, Text: apiErr.Error()}
			s.appendLog(entry)
			tuiSend(LogMsg{SessionID: s.ID, Entry: entry})
			tuiSend(DoneMsg{SessionID: s.ID, Err: apiErr})

		case anthropic.MessageBatchCanceledResult:
			cancelErr := fmt.Errorf("batch request was canceled")
			s.setErr(cancelErr)
			s.setState(StateFailed)
			entry := LogEntry{Kind: LogSystem, Text: cancelErr.Error()}
			s.appendLog(entry)
			tuiSend(LogMsg{SessionID: s.ID, Entry: entry})
			tuiSend(DoneMsg{SessionID: s.ID, Err: cancelErr})

		case anthropic.MessageBatchExpiredResult:
			expErr := fmt.Errorf("batch request expired (>24h)")
			s.setErr(expErr)
			s.setState(StateFailed)
			entry := LogEntry{Kind: LogSystem, Text: expErr.Error()}
			s.appendLog(entry)
			tuiSend(LogMsg{SessionID: s.ID, Entry: entry})
			tuiSend(DoneMsg{SessionID: s.ID, Err: expErr})

		default:
			unknownErr := fmt.Errorf("unknown batch result type")
			s.setErr(unknownErr)
			s.setState(StateFailed)
			tuiSend(DoneMsg{SessionID: s.ID, Err: unknownErr})
		}

		close(s.DoneCh)
		delete(byID, resp.CustomID)
	}

	if err := stream.Err(); err != nil && ctx.Err() == nil {
		for _, s := range byID {
			s.setErr(err)
			s.setState(StateFailed)
			entry := LogEntry{Kind: LogSystem, Text: fmt.Sprintf("results stream error: %v", err)}
			s.appendLog(entry)
			tuiSend(LogMsg{SessionID: s.ID, Entry: entry})
			tuiSend(DoneMsg{SessionID: s.ID, Err: err})
			close(s.DoneCh)
		}
	}
}
