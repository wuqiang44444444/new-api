package controller

import (
	"fmt"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAdminEvidenceFilterGlobalScopeAndInvalidScope(t *testing.T) {
	db := setupBillingURLNameControllerTest(t)
	require.NoError(t, db.AutoMigrate(&model.Log{}, &model.Task{}))
	start := time.Date(2026, 9, 1, 0, 0, 0, 0, billingSettlementLocation).Unix()
	rows := []model.Log{{Type: model.LogTypeConsume, ChannelId: 31, CreatedAt: start + 1, Other: `{}`}, {Type: model.LogTypeConsume, ChannelId: 32, CreatedAt: start + 2, Other: `{}`}}
	require.NoError(t, db.Create(&rows).Error)
	router := newBillingAdminTestRouter()
	router.GET("/details", GetAdminUpstreamBillingDetails)
	for _, tc := range []struct {
		query   string
		success bool
		total   int64
	}{
		{"&evidence_filter=incomplete", true, 2},
		{"&evidence_filter=incomplete&channel_id=31", true, 1},
		{"&evidence_filter=incomplete&url_key=https://unknown.test", false, 0},
		{"&evidence_filter=typo", false, 0},
		{"", false, 0},
	} {
		t.Run(tc.query, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			router.ServeHTTP(recorder, httptest.NewRequest("GET", fmt.Sprintf("/details?start_timestamp=%d&end_timestamp=%d%s", start, start+2591999, tc.query), nil))
			var response struct {
				Success bool `json:"success"`
				Data    struct {
					Result model.UpstreamBillingDetails `json:"result"`
				} `json:"data"`
			}
			require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
			assert.Equal(t, tc.success, response.Success, recorder.Body.String())
			if tc.success {
				assert.Equal(t, tc.total, response.Data.Result.Total)
			}
		})
	}
}
