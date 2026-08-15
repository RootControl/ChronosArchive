package session

import (
	"strings"

	anthropic "github.com/anthropics/anthropic-sdk-go"
	"github.com/chronosarchive/chronosarchive/config"
)

// legacyThinkingModels are model-ID prefixes that still require the deprecated
// thinking form {type:"enabled", budget_tokens:N} and that reject the `effort`
// parameter. Every other model gets adaptive thinking, which is the only form
// accepted on Opus 4.7+, Opus 5, Sonnet 5 and Fable 5 (budget_tokens returns a
// 400 there).
var legacyThinkingModels = []string{
	"claude-opus-4-5",
	"claude-opus-4-1",
	"claude-opus-4-0",
	"claude-sonnet-4-5",
	"claude-sonnet-4-0",
	"claude-haiku-4-5",
	"claude-haiku-3",
	"claude-3",
	"claude-2",
}

// supportsAdaptiveThinking reports whether the model accepts
// thinking={"type":"adaptive"} and the `effort` parameter.
func supportsAdaptiveThinking(model string) bool {
	for _, prefix := range legacyThinkingModels {
		if strings.HasPrefix(model, prefix) {
			return false
		}
	}
	return true
}

// requestShape holds the request fields shared by the streaming and batch code
// paths. MessageNewParams and MessageBatchNewParamsRequestParams are distinct
// SDK types with identical fields for everything we set, so the shape is built
// once and applied to whichever one the caller needs.
type requestShape struct {
	Model     anthropic.Model
	MaxTokens int64
	System    []anthropic.TextBlockParam
	Tools     []anthropic.ToolUnionParam
	Thinking  anthropic.ThinkingConfigParamUnion
	Output    anthropic.OutputConfigParam
	// CacheControl is the top-level auto-placed breakpoint. It marks the last
	// cacheable block in the request — the end of the conversation so far — so
	// each turn reuses the prefix written by the previous one.
	CacheControl anthropic.CacheControlEphemeralParam
}

// buildRequestShape assembles the model, token budget, system prompt, tools,
// thinking config, and cache breakpoints for one API call.
func buildRequestShape(cfg config.SessionConfig, systemPrompt string) requestShape {
	maxTokens := int64(cfg.MaxOutputTokens)
	if maxTokens <= 0 {
		maxTokens = config.DefaultMaxOutputTokens
	}

	adaptive := supportsAdaptiveThinking(cfg.Model)

	sh := requestShape{
		Model:     anthropic.Model(cfg.Model),
		MaxTokens: maxTokens,
		System:    []anthropic.TextBlockParam{{Text: systemPrompt}},
		Tools:     buildToolDefinitions(),
	}

	if adaptive && cfg.Effort != "" {
		sh.Output = anthropic.OutputConfigParam{
			Effort: anthropic.OutputConfigEffort(cfg.Effort),
		}
	}

	switch {
	case adaptive && cfg.Thinking:
		sh.Thinking = anthropic.ThinkingConfigParamUnion{
			OfAdaptive: &anthropic.ThinkingConfigAdaptiveParam{},
		}
	case adaptive && !cfg.Thinking:
		// Thinking is on by default on Opus 5, so "thinking: false" has to be
		// stated explicitly rather than left unset. Disabling is rejected at
		// effort=max, so leave it unset there and let the model decide.
		if cfg.Effort != "max" {
			sh.Thinking = anthropic.ThinkingConfigParamUnion{
				OfDisabled: &anthropic.ThinkingConfigDisabledParam{},
			}
		}
	case !adaptive && cfg.Thinking:
		budget := int64(cfg.ThinkingBudget)
		// MaxTokens must exceed the thinking budget on the legacy form.
		if sh.MaxTokens <= budget {
			sh.MaxTokens = budget + 4096
		}
		sh.Thinking = anthropic.ThinkingConfigParamOfEnabled(budget)
	}

	if !cfg.DisablePromptCache {
		// Breakpoint 1: end of the system prompt. Tools render before system,
		// so this one entry caches the tool definitions and system prompt
		// together — the large, byte-stable prefix every turn shares.
		sh.System[len(sh.System)-1].CacheControl = anthropic.NewCacheControlEphemeralParam()
		// Breakpoint 2: auto-placed at the end of the conversation.
		sh.CacheControl = anthropic.NewCacheControlEphemeralParam()
	}

	return sh
}

// applyToMessageParams fills a streaming/non-batch request with the shape.
func (sh requestShape) applyToMessageParams(messages []anthropic.MessageParam) anthropic.MessageNewParams {
	return anthropic.MessageNewParams{
		Model:        sh.Model,
		MaxTokens:    sh.MaxTokens,
		System:       sh.System,
		Messages:     messages,
		Tools:        sh.Tools,
		Thinking:     sh.Thinking,
		OutputConfig: sh.Output,
		CacheControl: sh.CacheControl,
	}
}

// applyToBatchParams fills a Batch API request with the shape.
func (sh requestShape) applyToBatchParams(messages []anthropic.MessageParam) anthropic.MessageBatchNewParamsRequestParams {
	return anthropic.MessageBatchNewParamsRequestParams{
		Model:        sh.Model,
		MaxTokens:    sh.MaxTokens,
		System:       sh.System,
		Messages:     messages,
		Tools:        sh.Tools,
		Thinking:     sh.Thinking,
		OutputConfig: sh.Output,
		CacheControl: sh.CacheControl,
	}
}
