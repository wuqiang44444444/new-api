package model

import (
	"context"
	"encoding/base64"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCustomerStatementReportsUnclassifiedExpressionWithoutLosingAmounts(t *testing.T) {
	db := setupBillingReconciliationTestDB(t)
	require.NoError(t, db.Create(&User{Id: 91, Username: "randy"}).Error)
	other, err := common.Marshal(map[string]any{
		"expr_b64":    base64.StdEncoding.EncodeToString([]byte(`tier("custom", param("custom_meter") * 145747.800587)`)),
		"group_ratio": 0.87, "contract_applicable": false,
	})
	require.NoError(t, err)
	require.NoError(t, db.Create(&Log{UserId: 91, TokenId: 40, ChannelId: 76, ModelName: "video", Type: LogTypeConsume, CreatedAt: 1100, Quota: 870, Other: string(other)}).Error)
	s, err := GetBillingCustomerStatement(context.Background(), 91, 1000, 1200, "channel", 0, "", "")
	require.NoError(t, err)
	require.Len(t, s.Groups, 1)
	require.Len(t, s.Groups[0].Models, 1)
	m := s.Groups[0].Models[0]
	assert.Equal(t, BillingReconciliationModeUnknown, m.BillingMode)
	require.NotNil(t, m.DataQuality)
	assert.Equal(t, "partial", m.DataQuality.Status)
	assert.EqualValues(t, 1, m.DataQuality.UnknownBillingModeRequests)
	assert.Zero(t, m.DataQuality.UnavailableRequests)
	assert.Zero(t, m.DataQuality.MissingHistoricalPriceRows)
	assert.Equal(t, m.DataQuality, s.DataQuality)
	assert.EqualValues(t, 870, s.Summary.NetQuota)
	require.NotNil(t, s.OriginalQuota)
	assert.EqualValues(t, 1000, *s.OriginalQuota)
}

func TestCustomerStatementDistinguishesMissingAndUnnamedChannels(t *testing.T) {
	db := setupBillingReconciliationTestDB(t)
	require.NoError(t, db.Create(&User{Id: 91, Username: "randy"}).Error)
	require.NoError(t, db.Create(&[]Channel{{Id: 76, Name: "current channel"}, {Id: 98, Name: " "}}).Error)
	for _, id := range []int{76, 97, 98} {
		require.NoError(t, db.Create(&Log{UserId: 91, TokenId: 40, ChannelId: id, ModelName: "video", Type: LogTypeConsume, CreatedAt: 1100, Quota: 100, Other: `{"model_price":1,"group_ratio":1}`}).Error)
	}
	s, err := GetBillingCustomerStatement(context.Background(), 91, 1000, 1200, "channel", 0, "", "")
	require.NoError(t, err)
	require.Len(t, s.Groups, 3)
	for _, g := range s.Groups {
		assert.Equal(t, g.Id == 97, g.Deleted)
		if g.Id == 76 {
			assert.Equal(t, "current channel", g.Name)
		}
		assert.EqualValues(t, 100, g.Usage.NetQuota)
	}
	assert.EqualValues(t, 300, s.Summary.NetQuota)
}
