// Package pricing converts token usage into an estimated USD cost. It lives
// outside both session and tui because each needs it: tui to display spend,
// session to enforce a spend ceiling mid-run.
package pricing

// Rate holds per-million-token list prices in USD.
type Rate struct{ In, Out float64 }

// ModelRates maps model ID to its published $/MTok input and output rates.
var ModelRates = map[string]Rate{
	"claude-fable-5":            {10.0, 50.0},
	"claude-mythos-5":           {10.0, 50.0},
	"claude-opus-5":             {5.0, 25.0},
	"claude-opus-4-8":           {5.0, 25.0},
	"claude-opus-4-7":           {5.0, 25.0},
	"claude-opus-4-6":           {5.0, 25.0},
	"claude-opus-4-5":           {5.0, 25.0},
	"claude-opus-4-1":           {15.0, 75.0},
	"claude-opus-4-0":           {15.0, 75.0},
	"claude-sonnet-5":           {3.0, 15.0},
	"claude-sonnet-4-6":         {3.0, 15.0},
	"claude-sonnet-4-5":         {3.0, 15.0},
	"claude-sonnet-4-0":         {3.0, 15.0},
	"claude-haiku-4-5":          {1.0, 5.0},
	"claude-haiku-4-5-20251001": {1.0, 5.0},
}

// FallbackModel supplies rates for models missing from ModelRates.
const FallbackModel = "claude-sonnet-4-6"

// Multipliers applied to a model's input rate.
const (
	CacheWriteMultiplier = 1.25 // 5-minute TTL write premium
	CacheReadMultiplier  = 0.10 // cache hit
	BatchDiscount        = 0.50 // Message Batches API
)

// Usage is the token breakdown for a session as reported by the API. Input
// counts only the uncached remainder; cache reads and writes bill at different
// rates and so are tracked separately.
type Usage struct {
	Input      int64
	Output     int64
	CacheWrite int64
	CacheRead  int64
}

// Total returns the sum of all token counts, for display purposes.
func (u Usage) Total() int64 { return u.Input + u.Output + u.CacheWrite + u.CacheRead }

// Estimate returns the approximate USD cost for the given model and usage,
// accounting for prompt-cache rates and the Batch API discount. Unknown models
// fall back to Sonnet rates.
func Estimate(model string, u Usage, batch bool) float64 {
	r, ok := ModelRates[model]
	if !ok {
		r = ModelRates[FallbackModel]
	}
	cost := float64(u.Input)/1e6*r.In +
		float64(u.Output)/1e6*r.Out +
		float64(u.CacheWrite)/1e6*r.In*CacheWriteMultiplier +
		float64(u.CacheRead)/1e6*r.In*CacheReadMultiplier
	if batch {
		cost *= BatchDiscount
	}
	return cost
}
