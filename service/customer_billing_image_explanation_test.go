package service

import (
	"encoding/base64"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCustomerImageExplanationUsesFrozenBillableTokens(t *testing.T) {
	const expression = `tier("base", p * 5 + cr * 1.25 + img * 8 + img_cr * 2 + c * 30)`
	for _, tc := range []struct {
		name       string
		prompt     int
		completion int
		quota      int
		facts      map[string]any
		mutate     func(map[string]any)
		want       bool
	}{
		{"generation includes text", 76, 1372, 20770, map[string]any{"p": 76, "c": 1372, "cr": 0, "img": 0, "img_cr": 0, "img_o": 1372, "ai": 0}, nil, true},
		{"edit and cache intersection", 1000, 100, 4113, map[string]any{"p": 300, "c": 100, "cr": 100, "img": 400, "img_cr": 200, "img_o": 100, "ai": 0}, nil, true},
		{"discounted model fee", 76, 1372, 8308, map[string]any{"p": 76, "c": 1372, "cr": 0, "img": 0, "img_cr": 0}, func(o map[string]any) {
			o["group_ratio"] = 0.5
			o["contract_applicable"] = true
			o["contract_discount"] = 0.8
		}, true},
		{"missing media facts", 76, 1372, 20770, nil, nil, false},
		{"partial media facts", 76, 1372, 20770, map[string]any{"p": 76, "c": 1372, "cr": 0, "img": 0}, nil, false},
		{"negative quantity", 76, 1372, 20770, map[string]any{"p": -1, "c": 1372, "cr": 0, "img": 0, "img_cr": 0}, nil, false},
		{"fractional quantity", 76, 1372, 20770, map[string]any{"p": 76.5, "c": 1372, "cr": 0, "img": 0, "img_cr": 0}, nil, false},
		{"contradictory total", 76, 1372, 20770, map[string]any{"p": 75, "c": 1372, "cr": 0, "img": 0, "img_cr": 0}, nil, false},
		{"contradictory charge", 76, 1372, 30000, map[string]any{"p": 76, "c": 1372, "cr": 0, "img": 0, "img_cr": 0}, nil, false},
		{"anthropic frozen usage is not interpreted as openai", 76, 1372, 20770, map[string]any{"p": 76, "c": 1372, "cr": 0, "img": 0, "img_cr": 0}, func(o map[string]any) {
			o["usage_semantic"] = "anthropic"
		}, false},
		{"claude frozen usage is not interpreted as openai", 76, 1372, 20770, map[string]any{"p": 76, "c": 1372, "cr": 0, "img": 0, "img_cr": 0}, func(o map[string]any) {
			o["claude"] = true
		}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			oldQuota := common.QuotaPerUnit
			common.QuotaPerUnit = 500000
			t.Cleanup(func() { common.QuotaPerUnit = oldQuota })
			other := map[string]any{"billing_mode": "tiered_expr", "billing_unit": "token", "usage_semantic": "openai", "request_path": "/v1/images/generations", "matched_tier": "base", "expr_b64": base64.StdEncoding.EncodeToString([]byte(expression)), "group_ratio": 1, "contract_applicable": false}
			if tc.facts != nil {
				other["billing_tokens"] = tc.facts
			}
			if tc.mutate != nil {
				tc.mutate(other)
			}
			raw, err := common.Marshal(other)
			require.NoError(t, err)
			log := &model.Log{Type: model.LogTypeConsume, PromptTokens: tc.prompt, CompletionTokens: tc.completion, Quota: tc.quota, Other: string(raw)}
			AttachLogsBillingDisplay([]*model.Log{log})
			var response struct {
				Explanation struct {
					Lines []customerBillingLine `json:"lines"`
				} `json:"billing_explanation"`
			}
			require.NoError(t, common.UnmarshalJsonStr(log.Other, &response))
			assert.Equal(t, tc.quota, log.Quota)
			assert.Equal(t, tc.prompt, log.PromptTokens)
			assert.Equal(t, tc.completion, log.CompletionTokens)
			if !tc.want {
				assert.Empty(t, response.Explanation.Lines)
				return
			}
			require.Len(t, response.Explanation.Lines, 5, "img_o is informational when the expression prices c")
			byLabel := make(map[string]customerBillingLine)
			for _, line := range response.Explanation.Lines {
				byLabel[line.Label] = line
			}
			if tc.name == "generation includes text" {
				assert.Equal(t, customerBillingLine{"Text Input", 76, "token", 5, 0.00038}, byLabel["Text Input"])
				assert.Equal(t, customerBillingLine{"Image Out", 1372, "token", 30, 0.04116}, byLabel["Image Out"])
			}
			if tc.name == "edit and cache intersection" {
				assert.Equal(t, float64(300), byLabel["Text Input"].Quantity)
				assert.Equal(t, float64(400), byLabel["Image In"].Quantity)
				assert.Equal(t, float64(200), byLabel["Image Cache"].Quantity)
			}
		})
	}
}

func TestCustomerImageExplanationSeparatesOutputOnlyWhenPriced(t *testing.T) {
	const expression = `tier("base", p * 5 + img_o * 30 + c * 10)`
	other := map[string]any{"billing_mode": "tiered_expr", "matched_tier": "base", "expr_b64": base64.StdEncoding.EncodeToString([]byte(expression)), "group_ratio": 1, "contract_applicable": false,
		"billing_tokens": map[string]any{"p": 10, "c": 20, "img_o": 100}}
	raw, err := common.Marshal(other)
	require.NoError(t, err)
	log := &model.Log{Type: model.LogTypeConsume, PromptTokens: 10, CompletionTokens: 120, Quota: 1625, Other: string(raw)}
	var decoded map[string]any
	require.NoError(t, common.Unmarshal(raw, &decoded))
	lines := customerBillingLines(log, decoded, model.CustomerBillingLogRow(log), 500000)
	require.Len(t, lines, 3)
	assert.Contains(t, lines, customerBillingLine{"Output", 20, "token", 10, 0.0002})
	assert.Contains(t, lines, customerBillingLine{"Image Out", 100, "token", 30, 0.003})
}
