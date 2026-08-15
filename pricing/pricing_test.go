package pricing

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
	got := Estimate("claude-opus-4-6", Usage{Input: 1_000_000, Output: 1_000_000}, false)
	approx(t, got, 30.0)

	// Sonnet: $3 / $15.
	got = Estimate("claude-sonnet-4-6", Usage{Input: 1_000_000, Output: 1_000_000}, false)
	approx(t, got, 18.0)

	// Haiku 4.5: $1 / $5.
	got = Estimate("claude-haiku-4-5", Usage{Input: 1_000_000, Output: 1_000_000}, false)
	approx(t, got, 6.0)
}

func TestEstimateCostUnknownModelFallsBack(t *testing.T) {
	got := Estimate("some-future-model", Usage{Input: 1_000_000}, false)
	want := Estimate("claude-sonnet-4-6", Usage{Input: 1_000_000}, false)
	approx(t, got, want)
}

func TestEstimateCostCacheRates(t *testing.T) {
	// Cache reads bill at ~0.1x the input rate.
	read := Estimate("claude-opus-4-6", Usage{CacheRead: 1_000_000}, false)
	approx(t, read, 5.0*CacheReadMultiplier)

	// Cache writes bill at ~1.25x the input rate.
	write := Estimate("claude-opus-4-6", Usage{CacheWrite: 1_000_000}, false)
	approx(t, write, 5.0*CacheWriteMultiplier)

	// A cached read must be far cheaper than processing the same tokens fresh.
	fresh := Estimate("claude-opus-4-6", Usage{Input: 1_000_000}, false)
	if read >= fresh {
		t.Errorf("cache read ($%.4f) should cost less than fresh input ($%.4f)", read, fresh)
	}
}

func TestEstimateCostBatchDiscount(t *testing.T) {
	u := Usage{Input: 1_000_000, Output: 500_000}
	full := Estimate("claude-opus-4-6", u, false)
	batched := Estimate("claude-opus-4-6", u, true)
	approx(t, batched, full*0.5)
}

func TestUsageTotal(t *testing.T) {
	u := Usage{Input: 10, Output: 20, CacheWrite: 30, CacheRead: 40}
	if u.Total() != 100 {
		t.Errorf("got total %d, want 100", u.Total())
	}
}

// Every model the config layer can produce should have an explicit rate, so
// costs are never silently estimated with the fallback.
func TestKnownModelsHaveRates(t *testing.T) {
	for _, m := range []string{
		"claude-opus-5", "claude-opus-4-8", "claude-opus-4-7", "claude-opus-4-6",
		"claude-sonnet-5", "claude-sonnet-4-6", "claude-haiku-4-5", "claude-fable-5",
	} {
		if _, ok := ModelRates[m]; !ok {
			t.Errorf("no pricing entry for %s", m)
		}
	}
}
