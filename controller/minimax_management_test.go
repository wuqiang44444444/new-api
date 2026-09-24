package controller

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/QuantumNous/new-api/clienterrlog"
	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMiniMaxUnsupportedHealthChecksPreserveObservations(t *testing.T) {
	for _, tc := range []struct {
		name      string
		automatic bool
		status    int
	}{
		{"manual enabled", false, common.ChannelStatusEnabled},
		{"manual disabled", false, common.ChannelStatusAutoDisabled},
		{"automatic enabled", true, common.ChannelStatusEnabled},
		{"automatic disabled", true, common.ChannelStatusAutoDisabled},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db := setupAutoCheckTest(t)
			provider := mustNotReachProviderServer(t)
			channel := &model.Channel{Type: constant.ChannelTypeMiniMaxLink, Models: "customer-video", Key: "fixture-key", BaseURL: &provider.URL, Status: tc.status, TestTime: 123, ResponseTime: 456, AutoBan: common.GetPointer(1)}
			require.NoError(t, db.Create(channel).Error)
			before := clienterrlog.CurrentHealth()
			// Even an exceeded threshold cannot turn an unsupported check into
			// an upstream failure or a channel status transition.
			summary := testChannelForHealthCheck(context.Background(), channel, 0, true, -1, tc.automatic)
			after := clienterrlog.CurrentHealth()
			assert.Equal(t, before.Accepted, after.Accepted, "unsupported checks must not submit failure events")
			assert.Equal(t, before.Dropped, after.Dropped)
			assert.Equal(t, 1, summary.Tested)
			assert.Equal(t, 1, summary.Unsupported)
			assert.Zero(t, summary.Failed)
			assert.Zero(t, summary.Succeeded)
			assert.Zero(t, summary.Disabled)
			assert.Zero(t, summary.Enabled)
			var stored model.Channel
			require.NoError(t, db.First(&stored, channel.Id).Error)
			assert.Equal(t, channel.Status, stored.Status)
			assert.Equal(t, int64(123), stored.TestTime)
			assert.Equal(t, 456, stored.ResponseTime)
			if tc.automatic {
				require.Len(t, summary.Checks, 1)
				detail := summary.Checks[0].Detail
				assert.Equal(t, "readonly_probe", detail["check_scope"])
				assert.Equal(t, "unsupported", detail["check_result"])
				assert.Equal(t, "not_sent", detail["upstream_request"])
				assert.Equal(t, "not_checked", detail["config_check"])
			}
		})
	}
}

func TestMiniMaxCopyCreatesDisabledChannelWithoutChangingIdentity(t *testing.T) {
	fx := newMiniMaxFundsFixture(t)
	require.NoError(t, fx.db.AutoMigrate(&model.Ability{}))
	var original model.Channel
	require.NoError(t, fx.db.First(&original, fx.channelID).Error)
	router := gin.New()
	router.POST("/copy/:id", CopyChannel)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/copy/"+strconv.Itoa(fx.channelID), nil))
	var response struct {
		Success bool   `json:"success"`
		Message string `json:"message"`
		Data    struct {
			ID int `json:"id"`
		} `json:"data"`
	}
	require.NoError(t, common.Unmarshal(w.Body.Bytes(), &response))
	require.True(t, response.Success, response.Message)
	var copied, unchanged model.Channel
	require.NoError(t, fx.db.First(&copied, response.Data.ID).Error)
	require.NoError(t, fx.db.First(&unchanged, original.Id).Error)
	assert.NotEqual(t, original.Id, copied.Id)
	assert.Equal(t, original.Type, copied.Type)
	assert.Equal(t, original.Models, copied.Models)
	assert.Equal(t, original.Key, copied.Key)
	assert.Equal(t, original.GetOtherSettings().VideoUpstreamProtocol, copied.GetOtherSettings().VideoUpstreamProtocol)
	assert.Equal(t, common.ChannelStatusManuallyDisabled, copied.Status)
	assert.Equal(t, original, unchanged)
	var abilities int64
	require.NoError(t, fx.db.Model(&model.Ability{}).Where("channel_id = ?", copied.Id).Count(&abilities).Error)
	assert.Zero(t, abilities)
	assert.Zero(t, fx.createCalls.Load())
}
