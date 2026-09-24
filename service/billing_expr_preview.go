package service

import (
	"fmt"
	"math"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/pkg/billingexpr"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/setting/operation_setting"
)

// 管理员只读试算：复用真实引擎与统一 quota 换算路径，显式 GroupRatio=1，
// 不构造真实资金会话、不调用任何落账接口。sample 的用量语义必须由调用方
// 显式声明（OpenAI 总输入 / Anthropic 文本输入），归一仍由后端 UsedVars
// 规则决定；QuotaPerUnit 从服务器读取，客户端不得传入。

const (
	// BillingExprPreviewMaxItems 限制单次请求的表达式数量。
	BillingExprPreviewMaxItems = 16
	// BillingExprPreviewMaxExpressionLength 限制单个表达式长度。
	BillingExprPreviewMaxExpressionLength = 20000
)

// BillingExprPreviewSample 是一次试算的显式用量输入。
type BillingExprPreviewSample struct {
	// UsageSemantic 声明用量口径："openai"（prompt 为总输入，含缓存等）
	// 或 "anthropic"（prompt 为纯文本输入）。缺省 openai。
	UsageSemantic         string `json:"usage_semantic"`
	PromptTokens          int64  `json:"prompt_tokens"`
	CompletionTokens      int64  `json:"completion_tokens"`
	CacheReadTokens       int64  `json:"cache_read_tokens,omitempty"`
	CacheCreationTokens   int64  `json:"cache_creation_tokens,omitempty"`
	CacheCreation1hTokens int64  `json:"cache_creation_tokens_1h,omitempty"`
	ImageTokens           int64  `json:"image_tokens,omitempty"`
	ImageOutputTokens     int64  `json:"image_output_tokens,omitempty"`
	AudioInputTokens      int64  `json:"audio_input_tokens,omitempty"`
	AudioOutputTokens     int64  `json:"audio_output_tokens,omitempty"`
	// PricingTime 是可选模拟时刻（RFC3339，可含时区偏移）。缺省为服务器
	// 收到试算请求的时刻；时区换算仍由表达式参数决定。
	PricingTime string `json:"pricing_time,omitempty"`
	// Body/Headers 是显式提供的合成探针上下文（例如 param("_task.resolution")）。
	// 仅供本次试算读取，不持久化、不记录；缺失时 param 返回 nil、header 返回
	// 空串，结果按「未模拟真实请求上下文」对待。
	Body    map[string]any    `json:"body,omitempty"`
	Headers map[string]string `json:"headers,omitempty"`
	Usage   map[string]any    `json:"usage,omitempty"`
}

// BillingExprPreviewItem 是一次批量试算中的单项。
type BillingExprPreviewItem struct {
	Key        string                    `json:"key"`
	Expression string                    `json:"expression"`
	Sample     *BillingExprPreviewSample `json:"sample,omitempty"`
	// Explicit unit, including constant task prices; never infer it from u().
	TaskUsage bool `json:"task_usage,omitempty"`
}

// BillingExprPreviewEvaluation 是真实引擎执行后的只读结果。scope 是模型
// 费用试算（GroupRatio=1），不含组倍率、合同折扣或工具附加费用。
type BillingExprPreviewEvaluation struct {
	PricingTime   string                  `json:"pricing_time"`
	UsageSemantic string                  `json:"usage_semantic"`
	Normalized    billingexpr.TokenParams `json:"normalized_usage"`
	// RawCostUSD is the actual USD amount before quota rounding; Saturated
	// marks quota clamping, not a change to this monetary amount.
	RawCostUSD   float64                        `json:"raw_cost_usd"`
	Quota        int                            `json:"quota"`
	MatchedTier  string                         `json:"matched_tier"`
	RequestRules []billingexpr.RequestRuleTrace `json:"request_rules,omitempty"`
	Saturated    bool                           `json:"saturated,omitempty"`
	// UsdExchangeRate 是本次试算实际采用的系统汇率及其冻结时刻；表达式
	// 不依赖 usd_exchange_rate() 时为空。方向固定为 CNY/USD。
	UsdExchangeRate *billingexpr.ExchangeRateContext `json:"usd_exchange_rate,omitempty"`
}

// BillingExprPreviewItemResult 是单项试算结果；Error 非空表示该表达式
// 无效或不可试算，不影响其他项。
type BillingExprPreviewItemResult struct {
	Key        string                         `json:"key"`
	Projection *billingexpr.DisplayProjection `json:"projection,omitempty"`
	Evaluation *BillingExprPreviewEvaluation  `json:"evaluation,omitempty"`
	Error      string                         `json:"error,omitempty"`
}

// PreviewBillingExpressions 对批量表达式做只读投影与（可选）真实引擎试算。
// 单项失败只写入该项的 Error 字段。
func PreviewBillingExpressions(items []BillingExprPreviewItem) []BillingExprPreviewItemResult {
	results := make([]BillingExprPreviewItemResult, 0, len(items))
	for _, item := range items {
		result := previewBillingExpressionItem(item)
		results = append(results, result)
	}
	return results
}

func previewBillingExpressionItem(item BillingExprPreviewItem) BillingExprPreviewItemResult {
	result := BillingExprPreviewItemResult{Key: item.Key}
	if len(item.Expression) > BillingExprPreviewMaxExpressionLength {
		result.Error = "expression exceeds the length limit"
		return result
	}
	if item.TaskUsage {
		return previewTaskUsageExpression(item)
	}
	if len(billingexpr.UsedUsageKeys(item.Expression)) > 0 {
		result.Error = "task usage expressions use a different unit contract and are not supported by this preview"
		return result
	}
	projection, err := billingexpr.DisplayProjectionForWithRate(item.Expression, currentDisplayExchangeRate())
	if err != nil {
		result.Error = fmt.Sprintf("invalid expression: %v", err)
		return result
	}
	result.Projection = projection
	if item.Sample == nil {
		return result
	}
	evaluation, err := evaluateBillingExpressionPreview(item.Expression, item.Sample)
	if err != nil {
		result.Error = err.Error()
		result.Projection = projection
		return result
	}
	result.Evaluation = evaluation
	return result
}

// evaluateBillingExpressionPreview 复用用量归一、真实引擎与统一 quota 换算。
// 使用显式 GroupRatio=1 的只读快照，不触碰任何资金会话。
func evaluateBillingExpressionPreview(expression string, sample *BillingExprPreviewSample) (*BillingExprPreviewEvaluation, error) {
	usageSemantic := sample.UsageSemantic
	if usageSemantic == "" {
		usageSemantic = "openai"
	}
	if usageSemantic != "openai" && usageSemantic != "anthropic" {
		return nil, fmt.Errorf("usage_semantic must be openai or anthropic")
	}
	pricingTime := time.Now()
	if raw := sample.PricingTime; raw != "" {
		parsed, err := time.Parse(time.RFC3339, raw)
		if err != nil {
			return nil, fmt.Errorf("pricing_time must be an RFC3339 timestamp")
		}
		pricingTime = parsed
	}
	for name, value := range map[string]int64{
		"prompt_tokens": sample.PromptTokens, "completion_tokens": sample.CompletionTokens,
		"cache_read_tokens": sample.CacheReadTokens, "cache_creation_tokens": sample.CacheCreationTokens,
		"cache_creation_tokens_1h": sample.CacheCreation1hTokens, "image_tokens": sample.ImageTokens,
		"image_output_tokens": sample.ImageOutputTokens, "audio_input_tokens": sample.AudioInputTokens,
		"audio_output_tokens": sample.AudioOutputTokens,
	} {
		if value < 0 || value > math.MaxInt32 {
			return nil, fmt.Errorf("%s must be between 0 and %d", name, math.MaxInt32)
		}
	}
	body, err := common.Marshal(sample.Body)
	if err != nil {
		return nil, fmt.Errorf("invalid trial request context")
	}
	usage := &dto.Usage{
		PromptTokens:     int(sample.PromptTokens),
		CompletionTokens: int(sample.CompletionTokens),
		UsageSemantic:    usageSemantic,
	}
	usage.PromptTokensDetails.CachedTokens = int(sample.CacheReadTokens)
	usage.PromptTokensDetails.ImageTokens = int(sample.ImageTokens)
	usage.PromptTokensDetails.AudioTokens = int(sample.AudioInputTokens)
	usage.CompletionTokenDetails.ImageTokens = int(sample.ImageOutputTokens)
	usage.CompletionTokenDetails.AudioTokens = int(sample.AudioOutputTokens)
	if usageSemantic == "anthropic" {
		usage.ClaudeCacheCreation5mTokens = int(sample.CacheCreationTokens)
		usage.ClaudeCacheCreation1hTokens = int(sample.CacheCreation1hTokens)
	} else {
		if sample.CacheCreation1hTokens != 0 {
			return nil, fmt.Errorf("cache_creation_tokens_1h requires anthropic usage_semantic")
		}
		usage.PromptTokensDetails.CachedCreationTokens = int(sample.CacheCreationTokens)
	}

	params := BuildTieredTokenParams(usage, usageSemantic == "anthropic", billingexpr.UsedVars(expression))
	// 服务端读取当前系统设置；客户样本 body/header 不能注入汇率。
	var rate *billingexpr.ExchangeRateContext
	if billingexpr.UsesExchangeRate(expression) {
		resolved, err := operation_setting.CurrentUsdExchangeRateContext()
		if err != nil {
			return nil, err
		}
		rate = resolved
	}
	snapshot := &billingexpr.BillingSnapshot{
		BillingMode:     "tiered_expr",
		ExprString:      expression,
		ExprHash:        billingexpr.ExprHashString(expression),
		GroupRatio:      1,
		QuotaPerUnit:    common.QuotaPerUnit,
		ExprVersion:     billingexpr.ExprVersion(expression),
		UsdExchangeRate: rate,
	}
	outcome, err := billingexpr.ComputeTieredQuotaWithRequest(snapshot, params, billingexpr.RequestInput{
		PricingTime:  &pricingTime,
		Body:         body,
		Headers:      sample.Headers,
		ExchangeRate: rate,
	})
	if err != nil {
		return nil, fmt.Errorf("expression evaluation failed: %v", err)
	}
	amountUSD := outcome.ActualQuotaBeforeGroup / common.QuotaPerUnit
	if math.IsNaN(amountUSD) || math.IsInf(amountUSD, 0) || amountUSD < 0 {
		return nil, fmt.Errorf("expression must produce a finite non-negative cost")
	}
	return &BillingExprPreviewEvaluation{
		PricingTime:   pricingTime.Format(time.RFC3339),
		UsageSemantic: usageSemantic,
		Normalized:    params,
		// Monetary amount before integer quota rounding, in actual USD.
		RawCostUSD:      amountUSD,
		Quota:           outcome.ActualQuotaAfterGroup,
		MatchedTier:     outcome.MatchedTier,
		RequestRules:    outcome.RequestRules,
		Saturated:       outcome.Clamp != nil,
		UsdExchangeRate: rate,
	}, nil
}
