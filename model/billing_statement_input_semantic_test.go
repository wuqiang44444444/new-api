package model

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// 阶段 C：历史行缺少 usage_semantic / input_tokens_total，但行内冻结了
// admin_info.usage_billing_path=billing-usage-gemini（canonical Gemini 计费
// 语义），总输入按已记录 prompt_tokens 恢复，缓存是子项不再次相加。
func TestInputSemanticRecoveredFromFrozenGeminiBillingPath(t *testing.T) {
	db := setupBillingReconciliationTestDB(t)
	require.NoError(t, db.Create(&User{Id: 7, Username: "customer"}).Error)
	logs := []Log{
		// prompt=1000, cache=300：总输入按 1000 计，不叠加缓存。
		{UserId: 7, TokenId: 4, ModelName: "gemini-flash", Type: LogTypeConsume, CreatedAt: 1100, PromptTokens: 1000, CompletionTokens: 100, Quota: 50,
			Other: `{"cache_tokens":300,"admin_info":{"usage_billing_path":"billing-usage-gemini"}}`},
		// upstream 路径只表示未走规范化，语义未知，保持未知。
		{UserId: 7, TokenId: 4, ModelName: "deepseek", Type: LogTypeConsume, CreatedAt: 1101, PromptTokens: 900, CompletionTokens: 50, Quota: 40,
			Other: `{"cache_tokens":200,"admin_info":{"usage_billing_path":"upstream","request_conversion":["OpenAI Compatible"]}}`},
		// 直接语义字段仍然优先于标记。
		{UserId: 7, TokenId: 4, ModelName: "explicit", Type: LogTypeConsume, CreatedAt: 1102, PromptTokens: 30, CompletionTokens: 5, Quota: 1,
			Other: `{"usage_semantic":"anthropic","cache_tokens":7,"admin_info":{"usage_billing_path":"billing-usage-gemini"}}`},
		// 一致性检查：缓存读取大于记录输入时保持未知。
		{UserId: 7, TokenId: 4, ModelName: "inconsistent", Type: LogTypeConsume, CreatedAt: 1103, PromptTokens: 10, CompletionTokens: 5, Quota: 1,
			Other: `{"cache_tokens":50,"admin_info":{"usage_billing_path":"billing-usage-gemini"}}`},
	}
	require.NoError(t, db.Create(&logs).Error)
	statement, err := GetBillingCustomerStatement(context.Background(), 7, 1000, 1200, "api_key", 0, "", "")
	require.NoError(t, err)
	byModel := map[string]BillingReconciliationModelSummary{}
	for _, group := range statement.Groups {
		for _, item := range group.Models {
			byModel[item.ModelName] = item
		}
	}
	require.Len(t, byModel, 4)
	// 恢复：1000（含 300 缓存），不重复相加。
	assert.EqualValues(t, 1000, byModel["gemini-flash"].Usage.InputTokens)
	// upstream 无语义证明：保持未知。
	assert.True(t, byModel["deepseek"].DataQuality == nil ||
		byModel["deepseek"].DataQuality.InputTokensUnavailableRequests > 0)
	assert.Zero(t, byModel["deepseek"].Usage.InputTokens)
	// 直接语义优先：anthropic = prompt + cache。
	assert.EqualValues(t, 37, byModel["explicit"].Usage.InputTokens)
	// 一致性失败：未知。
	assert.Zero(t, byModel["inconsistent"].Usage.InputTokens)
}
