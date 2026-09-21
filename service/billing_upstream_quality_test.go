package service

import (
	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestUpstreamExportExplainsHistoricalTestExclusions(t *testing.T) {
	quality := &model.BillingReconciliationDataQuality{UsageWithoutAmountRows: 3, TestAmountPendingReasons: map[string]int64{"missing_cache_write": 1, "missing_usage_semantic": 1, "estimated_usage": 1}}
	en := upstreamExportQualityReasons(quality, "en")
	zh := upstreamExportQualityReasons(quality, "zh")
	assert.Contains(t, en, "cache-write usage")
	assert.Contains(t, en, "usage breakdown")
	assert.Contains(t, en, "estimated usage")
	assert.NotContains(t, en, "lack reliable pricing records")
	assert.Contains(t, zh, "缓存写入用量")
	assert.Contains(t, zh, "用量分类依据")
	assert.Contains(t, zh, "估算值")
	assert.Contains(t, zh, "费用待确认")
	assert.Contains(t, zh, "不代表费用为零")
	assert.Contains(t, en, "excluded from confirmed totals")
}
