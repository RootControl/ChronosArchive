package tui

import (
	"math"
	"testing"
)

func approx(t *testing.T, got, want float64) {
	t.Helper()
	if math.Abs(got-want) > 1e-9 {
		t.Errorf("got $%.8f, want $%.8f", got, want)
	}
}

func TestEstimateCostBasic(t *testing.T) {
	// Opus-tier list rates are $5/MTok in, $25/MTok out.
	got := estimateCost("claude-opus-4-6", usage{input: 1_000_000, output: 1_000_000}, false)
	approx(t, got, 30.0)

	// Sonnet: $3 / $15.
	got = estimateCost("claude-sonnet-4-6", usage{input: 1_000_000, output: 1_000_000}, false)
	approx(t, got, 18.0)

	// Haiku 4.5: $1 / $5.
	got = estimateCost("claude-haiku-4-5", usage{input: 1_000_000, output: 1_000_000}, false)
	approx(t, got, 6.0)
}

func TestEstimateCostUnknownModelFallsBack(t *testing.T) {
	got := estimateCost("some-future-model", usage{input: 1_000_000}, false)
	want := estimateCost("claude-sonnet-4-6", usage{input: 1_000_000}, false)
	approx(t, got, want)
}

func TestEstimateCostCacheRates(t *testing.T) {
	// Cache reads bill at ~0.1x the input rate.
	read := estimateCost("claude-opus-4-6", usage{cacheRead: 1_000_000}, false)
	approx(t, read, 5.0*cacheReadMultiplier)

	// Cache writes bill at ~1.25x the input rate.
	write := estimateCost("claude-opus-4-6", usage{cacheWrite: 1_000_000}, false)
	approx(t, write, 5.0*cacheWriteMultiplier)

	// A cached read must be far cheaper than processing the same tokens fresh.
	fresh := estimateCost("claude-opus-4-6", usage{input: 1_000_000}, false)
	if read >= fresh {
		t.Errorf("cache read ($%.4f) should cost less than fresh input ($%.4f)", read, fresh)
	}
}

func TestEstimateCostBatchDiscount(t *testing.T) {
	u := usage{input: 1_000_000, output: 500_000}
	full := estimateCost("claude-opus-4-6", u, false)
	batched := estimateCost("claude-opus-4-6", u, true)
	approx(t, batched, full*0.5)
}

func TestUsageTotal(t *testing.T) {
	u := usage{input: 10, output: 20, cacheWrite: 30, cacheRead: 40}
	if u.total() != 100 {
		t.Errorf("got total %d, want 100", u.total())
	}
}

// Every model the config layer can produce should have an explicit rate, so
// costs are never silently estimated with the fallback.
func TestKnownModelsHaveRates(t *testing.T) {
	for _, m := range []string{
		"claude-opus-5", "claude-opus-4-8", "claude-opus-4-7", "claude-opus-4-6",
		"claude-sonnet-5", "claude-sonnet-4-6", "claude-haiku-4-5", "claude-fable-5",
	} {
		if _, ok := modelRates[m]; !ok {
			t.Errorf("no pricing entry for %s", m)
		}
	}
}
