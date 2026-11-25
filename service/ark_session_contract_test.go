package service

import (
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	hosttypes "github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestArkCacheContractRequiresConfiguredApplicationChannelAndGroup(t *testing.T) {
	pilot := &ArkPrefixCachePilot{Mode: "observe", UserID: 3, TokenID: 7, Model: "seed-lite", ChannelID: 267}
	channel := &model.Channel{Id: 267, Status: common.ChannelStatusEnabled, Group: "default"}
	for _, tt := range []struct {
		name, group string
		app, id     int
		allowed     bool
	}{
		{"exact rule", "default", 7, 267, true},
		{"foreign application", "default", 8, 267, false},
		{"other channel", "default", 7, 265, false},
		{"group unavailable", "unconfigured-cache-group", 7, 267, false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest("POST", "/v1/responses", nil)
			c.Set(contractRequestKey, &model.ContractEntitySnapshot{Id: 9, UserId: pilot.UserID, Enabled: true, Version: 12,
				Rules: []model.ContractEntityRule{{PublicModel: pilot.Model, ChannelId: tt.id, RouteGroup: tt.group, RatioUnits: 80_000_000}}})
			common.SetContextKey(c, constant.ContextKeyTokenId, tt.app)
			err := AuthorizeArkCacheContract(c, pilot, channel, pilot.Model)
			if !tt.allowed {
				require.ErrorIs(t, err, ErrCustomerContractScope)
				assert.Empty(t, common.GetContextKeyString(c, constant.ContextKeyUsingGroup))
				return
			}
			require.NoError(t, err)
			assert.Equal(t, "default", common.GetContextKeyString(c, constant.ContextKeyUsingGroup))
		})
	}
}

func TestArkSessionAppliesContractOnceToHoldSettlementAndStatement(t *testing.T) {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("POST", "/v1/responses", nil)
	info := relaycommon.GenRelayInfoResponses(c, &dto.OpenAIResponsesRequest{})
	info.OriginModelName = "seed-lite-contract"
	info.ContractBillingFact = &hosttypes.ContractBillingFact{UserId: 3, ContractId: 9, ContractVersion: 12, RatioUnits: 80_000_000}
	info.PriceData = hosttypes.PriceData{ModelRatio: 1, CacheRatio: 0.2, CompletionRatio: 2,
		GroupRatioInfo: hosttypes.GroupRatioInfo{GroupRatio: 1.5}}
	info.ChannelMeta = &relaycommon.ChannelMeta{UpstreamModelName: ArkPrefixCacheModelVersion}

	held, err := ArkSessionInferenceHold(info, 100, 8)
	require.NoError(t, err)
	assert.Equal(t, 140, held)
	charge, serialized, err := ArkSessionInferenceFacts(c, info, &dto.Usage{PromptTokens: 50, InputTokens: 50, CompletionTokens: 8, TotalTokens: 58,
		PromptTokensDetails: dto.InputTokenDetails{CachedTokens: 20}})
	require.NoError(t, err)
	assert.Equal(t, 60, charge)
	var log struct {
		Discount  string `json:"contract_discount"`
		Version   int    `json:"contract_version"`
		AdminInfo struct {
			Snapshot struct {
				Version  int    `json:"contract_version"`
				Discount string `json:"contract_discount"`
			} `json:"statement_snapshot"`
		} `json:"admin_info"`
	}
	require.NoError(t, common.UnmarshalJsonStr(serialized, &log))
	assert.Equal(t, "0.8", log.Discount)
	assert.Equal(t, log.Discount, log.AdminInfo.Snapshot.Discount)
	assert.Equal(t, 12, log.Version)
	assert.Equal(t, log.Version, log.AdminInfo.Snapshot.Version)
}
