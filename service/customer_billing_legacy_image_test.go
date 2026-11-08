package service

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLegacyImageConsumeLogKeepsChargeAndReportedUsage(t *testing.T) {
	truncate(t)
	const userID, tokenID, channelID, balance = 82, 82, 82, 10_000_000
	const tokenKey = "legacy-image-usage-test"
	seedUser(t, userID, balance)
	seedToken(t, tokenID, userID, tokenKey, balance)
	seedChannel(t, channelID)

	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/images/edits", nil)
	info := &relaycommon.RelayInfo{
		UserId: userID, UserQuota: balance, TokenId: tokenID, TokenKey: tokenKey,
		OriginModelName: "legacy-image", UsingGroup: "default", StartTime: time.Now(),
		ChannelMeta: &relaycommon.ChannelMeta{ChannelId: channelID},
		PriceData: types.PriceData{ModelRatio: 2.5, ImageRatio: 1.6, CompletionRatio: 6,
			GroupRatioInfo: types.GroupRatioInfo{GroupRatio: 1}},
	}
	details := &dto.ImageTokenUsage{
		TextInput: common.GetPointer(20), ImageInput: common.GetPointer(100),
		TextOutput: common.GetPointer(20), ImageOutput: common.GetPointer(100),
	}
	usage := &dto.Usage{
		PromptTokens: 120, CompletionTokens: 120, TotalTokens: 240,
		PromptTokensDetails: dto.InputTokenDetails{ImageTokens: 100, TextTokens: 20},
		ImageTokenUsage:     details,
	}
	PostTextConsumeQuota(ctx, info, usage, nil)

	log := getLastLog(t)
	require.NotNil(t, log)
	assert.Equal(t, 2250, log.Quota)
	wallet, err := model.GetUserQuota(userID, true)
	require.NoError(t, err)
	assert.Equal(t, balance-2250, wallet)

	var recorded struct {
		ImageTokenUsage *dto.ImageTokenUsage `json:"image_token_usage"`
	}
	require.NoError(t, common.UnmarshalJsonStr(log.Other, &recorded))
	assert.Equal(t, details, recorded.ImageTokenUsage)
}

func TestLegacyImageBillingExplainsRecordedModalitiesAtExistingRates(t *testing.T) {
	for _, tc := range []struct {
		name  string
		facts map[string]int
		want  []customerBillingLine
	}{
		{
			name:  "mixed output uses the same rate for both parts",
			facts: map[string]int{"text_input": 20, "image_input": 100, "text_output": 20, "image_output": 100},
			want: []customerBillingLine{
				{"Text Input", 20, "token", 5, 0.0001},
				{"Text Output", 20, "token", 30, 0.0006},
				{"Image Out", 100, "token", 30, 0.003},
				{"Image In", 100, "token", 8, 0.0008},
			},
		},
		{
			name:  "partial output stays aggregate",
			facts: map[string]int{"text_input": 20, "image_input": 100, "image_output": 100},
			want: []customerBillingLine{
				{"Text Input", 20, "token", 5, 0.0001},
				{"Output", 120, "token", 30, 0.0036},
				{"Image In", 100, "token", 8, 0.0008},
			},
		},
		{
			name: "historical logs keep their original explanation",
			want: []customerBillingLine{
				{"Input", 20, "token", 5, 0.0001},
				{"Output", 120, "token", 30, 0.0036},
				{"Image In", 100, "token", 8, 0.0008},
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			other := map[string]any{
				"request_path": "/v1/images/edits", "model_ratio": 2.5, "completion_ratio": 6,
				"group_ratio": 1, "contract_applicable": false, "model_price": -1,
				"image": true, "image_output": 100, "image_ratio": 1.6,
			}
			if tc.facts != nil {
				other["image_token_usage"] = tc.facts
			}
			raw, err := common.Marshal(other)
			require.NoError(t, err)
			log := &model.Log{Type: model.LogTypeConsume, PromptTokens: 120, CompletionTokens: 120, Quota: 2250, Other: string(raw)}
			require.NoError(t, common.Unmarshal(raw, &other))
			assert.Equal(t, tc.want, customerBillingLines(log, other, model.CustomerBillingLogRow(log), 500000))
			assert.Equal(t, 2250, log.Quota)
		})
	}
}
