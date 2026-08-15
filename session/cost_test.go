package session

import (
	"testing"

	anthropic "github.com/anthropics/anthropic-sdk-go"
	"github.com/chronosarchive/chronosarchive/config"
)

// newTestSession builds a session with the given cost ceiling.
func newTestSession(limit float64) *Session {
	return New("s0", config.SessionConfig{
		Name:       "t",
		Model:      "claude-opus-4-6", // $5/MTok in, $25/MTok out
		MaxCostUSD: limit,
	})
}

func TestCheckCostLimit_DisabledByDefault(t *testing.T) {
	s := newTestSession(0)
	s.addUsage(anthropic.Usage{InputTokens: 100_000_000, OutputTokens: 100_000_000})

	if s.checkCostLimit(func(any) {}) {
		t.Fatal("a zero limit must mean no limit, but the session was stopped")
	}
}

func TestCheckCostLimit_UnderBudgetContinues(t *testing.T) {
	s := newTestSession(10.0)
	// 100k input at $5/MTok = $0.50, well under the $10 ceiling.
	s.addUsage(anthropic.Usage{InputTokens: 100_000})

	if s.checkCostLimit(func(any) {}) {
		t.Fatalf("stopped at $%.4f despite a $10 budget", s.CostUSD())
	}
	if s.State() == StateDone {
		t.Error("session should still be running")
	}
}

func TestCheckCostLimit_StopsWhenExceeded(t *testing.T) {
	s := newTestSession(1.0)
	// 1M output at $25/MTok = $25, far past the $1 ceiling.
	s.addUsage(anthropic.Usage{OutputTokens: 1_000_000})

	var msgs []any
	if !s.checkCostLimit(func(m any) { msgs = append(msgs, m) }) {
		t.Fatalf("expected a stop at $%.4f against a $1 budget", s.CostUSD())
	}
	if s.State() != StateDone {
		t.Errorf("state = %v, want done", s.State())
	}
	if s.Err() == nil {
		t.Error("expected the budget overrun to be recorded as an error")
	}
	// The TUI must be told, or the session would appear to stall.
	var sawDone bool
	for _, m := range msgs {
		if _, ok := m.(DoneMsg); ok {
			sawDone = true
		}
	}
	if !sawDone {
		t.Error("no DoneMsg sent to the TUI")
	}
}

// Cache reads bill at ~0.1x, so they must count toward the ceiling at the
// discounted rate rather than the full input rate.
func TestCheckCostLimit_UsesCacheAwareCost(t *testing.T) {
	cached := newTestSession(0)
	cached.addUsage(anthropic.Usage{CacheReadInputTokens: 1_000_000})

	fresh := newTestSession(0)
	fresh.addUsage(anthropic.Usage{InputTokens: 1_000_000})

	if cached.CostUSD() >= fresh.CostUSD() {
		t.Errorf("cached $%.4f should be cheaper than fresh $%.4f",
			cached.CostUSD(), fresh.CostUSD())
	}
}

// The Batch API discount must be reflected in the ceiling check too.
func TestCostUSD_AppliesBatchDiscount(t *testing.T) {
	plain := New("a", config.SessionConfig{Model: "claude-opus-4-6"})
	batched := New("b", config.SessionConfig{Model: "claude-opus-4-6", Batch: true})
	for _, s := range []*Session{plain, batched} {
		s.addUsage(anthropic.Usage{InputTokens: 1_000_000, OutputTokens: 1_000_000})
	}

	if got, want := batched.CostUSD(), plain.CostUSD()/2; got != want {
		t.Errorf("batch cost = $%.4f, want $%.4f", got, want)
	}
}
