package billingexpr

import (
	"crypto/sha256"
	"fmt"
	"time"

	"github.com/QuantumNous/new-api/common"
)

type RequestInput struct {
	RecordCalculation bool `json:"-"`
	Headers           map[string]string
	Body              []byte
	Usage             map[string]any
	// PricingTime freezes the wall-clock instant used by the hour/minute/
	// weekday/month/day time functions. When nil the functions keep reading
	// the current time (native call semantics). Durable async products (for
	// example Azure Batch) persist one UTC instant at creation and pass the
	// same value to every later evaluation, so settlement can never re-read
	// a drifted clock. The value is per-run state and is never captured by
	// the shared compile cache.
	PricingTime *time.Time
	// ExchangeRate freezes the CNY/USD rate fact used by usd_exchange_rate()
	// for this run. Nil keeps fail-closed semantics: the function errors
	// instead of guessing. Like PricingTime it is per-run state, never
	// captured by the shared compile cache; settlement overrides it from the
	// BillingSnapshot.
	ExchangeRate *ExchangeRateContext
}

// TokenParams holds all token dimensions passed into an Expr evaluation.
// Fields beyond P and C are optional — when absent they default to 0,
// which means cache-unaware expressions keep working unchanged.
type TokenParams struct {
	P    float64 // prompt tokens (text) — auto-excludes sub-categories priced separately
	C    float64 // completion tokens (text) — auto-excludes sub-categories priced separately
	Len  float64 // total input context length for tier conditions (non-Claude: raw prompt_tokens; Claude: text + cache read + cache creation)
	CR   float64 // cache read (hit) tokens
	CC   float64 // cache creation tokens (5-min TTL for Claude, generic for others)
	CC1h float64 // cache creation tokens — 1-hour TTL (Claude only)
	Img  float64 // image input tokens
	ImgO float64 // image output tokens
	AI   float64 // audio input tokens
	AO   float64 // audio output tokens
}

// RequestRuleTrace describes one request-dependent multiplier detected at compile time.
type RequestRuleTrace struct {
	Cond       string  `json:"cond"`
	Multiplier float64 `json:"multiplier"`
	Matched    bool    `json:"matched"`
}

// TraceResult holds side-channel info captured while an expression runs.
type TraceResult struct {
	Calculation  *Calculation       `json:"calculation,omitempty"`
	MatchedTier  string             `json:"matched_tier"`
	RequestRules []RequestRuleTrace `json:"request_rules,omitempty"`
	Cost         float64            `json:"cost"`
}

// BillingSnapshot captures billing state at pre-consume time. Expression and
// request fields stay frozen; group-dependent fields are refreshed before an
// auto-group retry and settlement. It is fully serializable and contains no
// compiled program pointers.
type BillingSnapshot struct {
	Calculation               *Calculation   `json:"calculation,omitempty"`
	BillingMode               string         `json:"billing_mode"`
	ModelName                 string         `json:"model_name"`
	ExprString                string         `json:"expr_string"`
	ExprHash                  string         `json:"expr_hash"`
	GroupRatio                float64        `json:"group_ratio"`
	EstimatedPromptTokens     int            `json:"estimated_prompt_tokens"`
	EstimatedCompletionTokens int            `json:"estimated_completion_tokens"`
	EstimatedQuotaBeforeGroup float64        `json:"estimated_quota_before_group"`
	EstimatedQuotaAfterGroup  int            `json:"estimated_quota_after_group"`
	EstimatedTier             string         `json:"estimated_tier"`
	QuotaPerUnit              float64        `json:"quota_per_unit"`
	ExprVersion               int            `json:"expr_version"`
	TaskUsageBilling          bool           `json:"task_usage_billing,omitempty"`
	UsageFacts                map[string]any `json:"usage_facts,omitempty"`
	// UsageUnits freezes declared meter units for historical statement display.
	// Enum/boolean fields retain their type; this never affects evaluation.
	UsageUnits map[string]string `json:"usage_units,omitempty"`
	// UsdExchangeRate freezes the CNY/USD rate fact for expressions that call
	// usd_exchange_rate(). Nil when the expression does not depend on it.
	// Settlement always re-injects this frozen fact; it never re-reads the
	// current setting, so a later global rate change cannot reprice an
	// already-accepted request.
	UsdExchangeRate *ExchangeRateContext `json:"usd_exchange_rate,omitempty"`
}

// TieredResult holds everything needed after running tiered settlement.
type TieredResult struct {
	Calculation            *Calculation       `json:"calculation,omitempty"`
	ActualQuotaBeforeGroup float64            `json:"actual_quota_before_group"`
	ActualQuotaAfterGroup  int                `json:"actual_quota_after_group"`
	MatchedTier            string             `json:"matched_tier"`
	RequestRules           []RequestRuleTrace `json:"request_rules,omitempty"`
	CrossedTier            bool               `json:"crossed_tier"`
	// Clamp records a single-request saturation event during quota conversion so the
	// caller can surface it on the consume log for admin auditing. Nil when no
	// clamping occurred. Not serialized: the marker is attached separately via
	// the shared quota-saturation audit path.
	Clamp *common.QuotaClamp `json:"-"`
}

// ExprHashString returns the SHA-256 hex digest of an expression string.
func ExprHashString(expr string) string {
	h := sha256.Sum256([]byte(expr))
	return fmt.Sprintf("%x", h)
}
