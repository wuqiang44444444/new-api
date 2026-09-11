package middleware

import (
	"context"
	"fmt"
	"github.com/QuantumNous/new-api/pkg/jsplugin"
	"github.com/QuantumNous/new-api/pkg/seedanceplugin"
	"github.com/QuantumNous/new-api/plugins"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSeedanceContractKeepsItsChannelWhenAvailabilityChanges(t *testing.T) {
	for _, tc := range []struct {
		name                      string
		enabled, conflictingToken bool
		status                    int
	}{
		{"bound-channel-enabled", true, false, http.StatusNoContent},
		{"bound-channel-disabled-replacement-enabled", false, false, http.StatusServiceUnavailable},
		{"token-outside-contract", true, true, http.StatusForbidden},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db, user, contract := setupCustomerContractMiddlewareDB(t)
			source := plugins.SeedanceSource()
			loaded, _, err := jsplugin.CompileSeedanceExtension(source, jsplugin.Options{}, jsplugin.SeedanceHostContract())
			require.NoError(t, err)
			previousStore := seedanceplugin.Default
			seedanceplugin.Default = seedanceplugin.NewStore()
			t.Cleanup(func() { seedanceplugin.Default = previousStore })
			require.NoError(t, seedanceplugin.Default.SyncSnapshot(context.Background(), []model.TaskPlugin{{Key: loaded.Meta.Key, Version: loaded.Meta.Version, APIVersion: loaded.Meta.APIVersion, Source: source, SourceHash: fmt.Sprintf("%x", common.Sha256Raw([]byte(source))), Enabled: true, Active: true}}))
			var bound model.Channel
			require.NoError(t, db.First(&bound, contract.Rules[0].ChannelId).Error)
			bound.Type = constant.ChannelTypeSeedanceLink
			bound.SetOtherSettings(dto.ChannelOtherSettings{
				VideoUpstreamProtocol: dto.VideoUpstreamProtocolModelArkV3Volcengine,
				AssetUpstreamProtocol: dto.AssetUpstreamProtocolNone,
			})
			replacement := model.Channel{
				Type: constant.ChannelTypeSeedanceLink, Name: "replacement", Key: "test-key",
				Models: "Model-A", Group: "contract-route", Status: common.ChannelStatusManuallyDisabled,
			}
			replacement.SetOtherSettings(bound.GetOtherSettings())
			if !tc.enabled {
				bound.Status = common.ChannelStatusManuallyDisabled
				replacement.Status = common.ChannelStatusEnabled
			}
			require.NoError(t, db.Save(&bound).Error)
			require.NoError(t, db.Create(&replacement).Error)

			enteredSubmission := false
			router := gin.New()
			router.POST("/video", func(c *gin.Context) {
				common.SetContextKey(c, constant.ContextKeyUserId, user.Id)
				common.SetContextKey(c, constant.ContextKeyAuthVersion, user.AuthVersion)
				common.SetContextKey(c, constant.ContextKeyTokenContractId, contract.Id)
				if tc.conflictingToken {
					service.GetChannelConstraints(c).AddPin(dto.ChannelPin{
						ChannelId: replacement.Id, Source: dto.PinSourceToken, Rank: dto.PinRankToken,
						RetryMode: dto.PinRetrySingleAttempt,
					})
				}
				relaycommon.SetVideoContractRequest(c, dto.VideoContractRequest{
					ContractID: dto.VideoContractModelArkV3,
					ModelArk:   &dto.ModelArkVideoCreateRequest{Model: "Model-A"},
				})
			}, ResolveSeedanceChannel(), func(c *gin.Context) {
				enteredSubmission = true
				assert.Equal(t, bound.Id, common.GetContextKeyInt(c, constant.ContextKeyChannelId))
				c.Status(http.StatusNoContent)
			})
			recorder := httptest.NewRecorder()
			router.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/video", nil))
			assert.Equal(t, tc.status == http.StatusNoContent, enteredSubmission, "rejected routing cannot reach pricing, hold or Provider submission")
			assert.Equal(t, tc.status, recorder.Code)
			if tc.status != http.StatusNoContent {
				assert.Contains(t, recorder.Body.String(), "model_not_found")
			}
		})
	}
}
