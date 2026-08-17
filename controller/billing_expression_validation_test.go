package controller

import (
	"fmt"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestValidateBillingExpressionsOptionUsesConfiguredSeedanceProtocol(t *testing.T) {
	previousDB := model.DB
	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "_"))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.Channel{}))
	model.DB = db
	t.Cleanup(func() {
		model.DB = previousDB
		sqlDB, dbErr := db.DB()
		if dbErr == nil {
			require.NoError(t, sqlDB.Close())
		}
	})

	feicaiChannel := model.Channel{
		Type:   constant.ChannelTypeSeedanceLink,
		Key:    "test-key",
		Status: common.ChannelStatusEnabled,
		Name:   "feicai",
		Models: "custom-feicai-model",
		Group:  "default",
	}
	feicaiChannel.SetOtherSettings(dto.ChannelOtherSettings{
		VideoUpstreamProtocol: dto.VideoUpstreamProtocolFeicaiVideosV1,
	})
	require.NoError(t, db.Create(&feicaiChannel).Error)

	officialChannel := model.Channel{
		Type:   constant.ChannelTypeSeedanceLink,
		Key:    "test-key",
		Status: common.ChannelStatusEnabled,
		Name:   "official",
		Models: "custom-official-model",
		Group:  "default",
	}
	officialChannel.SetOtherSettings(dto.ChannelOtherSettings{
		VideoUpstreamProtocol: dto.VideoUpstreamProtocolModelArkV3Volcengine,
	})
	require.NoError(t, db.Create(&officialChannel).Error)

	feicaiExpression := `{"custom-feicai-model":"v1:tier(\"base\", param(\"_task.duration_seconds\") * param(\"_task.size_multiplier\"))"}`
	officialExpression := `{"custom-official-model":"v1:tier(\"base\", param(\"_task.duration_seconds\") * param(\"_task.size_multiplier\"))"}`
	officialDurationExpression := `{"custom-official-model":"v1:tier(\"base\", param(\"_task.duration_seconds\") * 741114)"}`

	require.NoError(t, validateBillingExpressionsOption(feicaiExpression))
	require.ErrorContains(t, validateBillingExpressionsOption(officialExpression), "size_multiplier")
	require.NoError(t, validateBillingExpressionsOption(officialDurationExpression))
}
