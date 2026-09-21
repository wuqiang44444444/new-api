package service

import (
	"context"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNativeTextWriterZeroCacheWriteReconcilesWithoutFalseGap(t *testing.T) {
	for _, path := range []string{"/v1/chat/completions", "/v1/images/generations", "/v1/images/edits", "/pg/chat/completions"} {
		t.Run(path, func(t *testing.T) {
			db := setupVersionServiceTestDB(t)
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest("POST", path, nil)
			info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{}, StartTime: time.Now(), FirstResponseTime: time.Now()}
			other := GenerateTextOtherInfo(c, info, 1, 1, 1, 0, 0.1, -1, -1)
			other.SetPublic("contract_applicable", false)
			require.NoError(t, db.Create(&model.Log{Type: model.LogTypeConsume, ChannelId: 25, ModelName: "text", CreatedAt: 1100, PromptTokens: 100, Quota: 100, Other: other.JSONString()}).Error)
			details, err := model.GetUpstreamBillingDetails(context.Background(), model.UpstreamBillingDetailFilter{Start: 1000, End: 1200}, 1, 10, false)
			require.NoError(t, err)
			require.Len(t, details.Items, 1)
			row := details.Items[0]
			assert.Zero(t, row.CacheWriteTokens)
			assert.Zero(t, row.DataQuality.CacheWriteUnavailableRequests)
			assert.Equal(t, "complete", row.DataQuality.Status)
			require.NotNil(t, row.OriginalAmount)
			assert.EqualValues(t, 100, *row.OriginalAmount)
		})
	}
}
