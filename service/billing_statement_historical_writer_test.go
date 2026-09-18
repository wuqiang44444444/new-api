package service

import (
	"net/http/httptest"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

// Protect the writer evidence used for historical recovery: even an OpenAI
// request chain must not conceal the special Claude/OpenRouter input convention.
func TestStatementHistoricalRecoveryHonorsWriterMarkers(t *testing.T) {
	for _, claude := range []bool{false, true} {
		name := "OpenAI"
		if claude {
			name = "Claude through OpenAI"
		}
		t.Run(name, func(t *testing.T) {
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest("POST", "/v1/chat/completions", nil)
			info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{}, RelayFormat: types.RelayFormatOpenAI, RequestConversionChain: []types.RelayFormat{types.RelayFormatOpenAI}, StartTime: time.Now(), FirstResponseTime: time.Now()}
			var other *model.LogOther
			if claude {
				other = GenerateClaudeOtherInfo(c, info, 1, 1, 1, 300, 0.1, 0, 1, 0, 0, 0, 0, 0, -1)
			} else {
				other = GenerateTextOtherInfo(c, info, 1, 1, 1, 300, 0.1, 0, -1)
			}
			appendUsageBillingPathForLog(other, false, &dto.Usage{})
			// No new usage_semantic field: exercise the historical write contract.
			row := model.CustomerBillingLogRow(&model.Log{Type: model.LogTypeConsume, PromptTokens: 1000, Quota: 100, Other: other.JSONString()})
			assert.Equal(t, claude, row.InputTokensUnavailable)
			if claude {
				assert.Zero(t, row.InputTokens)
			} else {
				assert.EqualValues(t, 1000, row.InputTokens)
			}
			assert.EqualValues(t, 300, row.CacheReadTokens)
			assert.EqualValues(t, 100, row.Quota)
		})
	}
}
