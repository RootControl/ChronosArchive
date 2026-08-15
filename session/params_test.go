package session

import (
	"strings"
	"testing"

	"github.com/chronosarchive/chronosarchive/config"
)

func TestSupportsAdaptiveThinking(t *testing.T) {
	adaptive := []string{
		"claude-opus-5", "claude-opus-4-8", "claude-opus-4-7", "claude-opus-4-6",
		"claude-sonnet-5", "claude-sonnet-4-6", "claude-fable-5", "claude-mythos-5",
	}
	legacy := []string{
		"claude-opus-4-5", "claude-opus-4-1", "claude-sonnet-4-5",
		"claude-haiku-4-5", "claude-haiku-4-5-20251001", "claude-3-opus-20240229",
	}
	for _, m := range adaptive {
		if !supportsAdaptiveThinking(m) {
			t.Errorf("%s should use adaptive thinking", m)
		}
	}
	for _, m := range legacy {
		if supportsAdaptiveThinking(m) {
			t.Errorf("%s should use the legacy budget_tokens form", m)
		}
	}
}

// budget_tokens returns a 400 on Opus 4.7+, Opus 5, Sonnet 5 and Fable 5, so a
// thinking-enabled session on those models must send the adaptive form.
func TestThinkingUsesAdaptiveOnModernModels(t *testing.T) {
	cfg := config.SessionConfig{Model: "claude-opus-5", Thinking: true, ThinkingBudget: 10000}
	sh := buildRequestShape(cfg, "sys")

	if sh.Thinking.OfAdaptive == nil {
		t.Fatal("expected adaptive thinking config")
	}
	if sh.Thinking.OfEnabled != nil {
		t.Fatal("budget_tokens form would be rejected with a 400 on this model")
	}
}

func TestThinkingUsesBudgetOnLegacyModels(t *testing.T) {
	cfg := config.SessionConfig{
		Model: "claude-haiku-4-5", Thinking: true, ThinkingBudget: 10000,
		MaxOutputTokens: 8192,
	}
	sh := buildRequestShape(cfg, "sys")

	if sh.Thinking.OfEnabled == nil {
		t.Fatal("expected the legacy budget_tokens form on a legacy model")
	}
	// max_tokens must exceed the thinking budget.
	if sh.MaxTokens <= 10000 {
		t.Fatalf("max_tokens (%d) must exceed the thinking budget (10000)", sh.MaxTokens)
	}
}

// Thinking is on by default on Opus 5, so an opted-out session has to disable
// it explicitly rather than leaving the field unset.
func TestThinkingDisabledIsExplicitOnModernModels(t *testing.T) {
	cfg := config.SessionConfig{Model: "claude-opus-5", Thinking: false}
	sh := buildRequestShape(cfg, "sys")

	if sh.Thinking.OfDisabled == nil {
		t.Fatal("expected an explicit disabled thinking config")
	}
}

// Disabling thinking is rejected at effort=max, so the field must stay unset.
func TestThinkingUnsetWhenDisabledAtMaxEffort(t *testing.T) {
	cfg := config.SessionConfig{Model: "claude-opus-5", Thinking: false, Effort: "max"}
	sh := buildRequestShape(cfg, "sys")

	if sh.Thinking.OfDisabled != nil {
		t.Fatal("disabled thinking at effort=max would be rejected with a 400")
	}
}

func TestEffortOnlySentToAdaptiveModels(t *testing.T) {
	modern := buildRequestShape(config.SessionConfig{Model: "claude-opus-5", Effort: "high"}, "sys")
	if modern.Output.Effort != "high" {
		t.Errorf("expected effort to be forwarded, got %q", modern.Output.Effort)
	}

	// effort errors on Sonnet 4.5 / Haiku 4.5, so it must be omitted there.
	legacy := buildRequestShape(config.SessionConfig{Model: "claude-haiku-4-5", Effort: "high"}, "sys")
	if legacy.Output.Effort != "" {
		t.Errorf("effort must not be sent to legacy models, got %q", legacy.Output.Effort)
	}
}

func TestPromptCacheBreakpoints(t *testing.T) {
	cfg := config.SessionConfig{Model: "claude-opus-5"}
	sh := buildRequestShape(cfg, "sys")

	// Breakpoint on the last system block caches tools + system together.
	if sh.System[len(sh.System)-1].CacheControl.Type == "" {
		t.Error("expected a cache breakpoint on the system prompt")
	}
	// Top-level auto-placed breakpoint caches the conversation prefix.
	if sh.CacheControl.Type == "" {
		t.Error("expected a top-level cache breakpoint")
	}
}

func TestPromptCacheCanBeDisabled(t *testing.T) {
	cfg := config.SessionConfig{Model: "claude-opus-5", DisablePromptCache: true}
	sh := buildRequestShape(cfg, "sys")

	if sh.System[len(sh.System)-1].CacheControl.Type != "" {
		t.Error("system cache breakpoint should be absent when caching is disabled")
	}
	if sh.CacheControl.Type != "" {
		t.Error("top-level cache breakpoint should be absent when caching is disabled")
	}
}

func TestMaxOutputTokensDefaults(t *testing.T) {
	sh := buildRequestShape(config.SessionConfig{Model: "claude-opus-5"}, "sys")
	if sh.MaxTokens != config.DefaultMaxOutputTokens {
		t.Errorf("got max_tokens %d, want default %d", sh.MaxTokens, config.DefaultMaxOutputTokens)
	}

	sh = buildRequestShape(config.SessionConfig{Model: "claude-opus-5", MaxOutputTokens: 32000}, "sys")
	if sh.MaxTokens != 32000 {
		t.Errorf("got max_tokens %d, want 32000", sh.MaxTokens)
	}
}

// The system prompt advertises the tool list to the model; it must list every
// tool actually registered, or the model never learns some of them exist.
func TestSystemPromptListsEveryTool(t *testing.T) {
	prompt := buildSystemPrompt("/tmp/x", "do a thing", "")
	for _, def := range buildToolDefinitions() {
		if def.OfTool == nil {
			continue
		}
		if !strings.Contains(prompt, def.OfTool.Name) {
			t.Errorf("system prompt does not mention tool %q", def.OfTool.Name)
		}
	}
}
