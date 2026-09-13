package main

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestMigrationProtocolSelectionMatchesActivePricingContract(t *testing.T) {
	for _, activeFirst := range []bool{false, true} {
		t.Run(map[bool]string{true: "enabled first", false: "disabled first"}[activeFirst], func(t *testing.T) {
			db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
			require.NoError(t, err)
			sqlDB, err := db.DB()
			require.NoError(t, err)
			t.Cleanup(func() { require.NoError(t, sqlDB.Close()) })
			require.NoError(t, db.AutoMigrate(&model.Channel{}))
			enabledID, disabledID := 2, 1
			if activeFirst {
				enabledID, disabledID = 1, 2
			}
			enabled := model.Channel{Id: enabledID, Type: constant.ChannelTypeSeedanceLink, Status: common.ChannelStatusEnabled, Models: "shared"}
			enabled.SetOtherSettings(dto.ChannelOtherSettings{VideoUpstreamProtocol: dto.VideoUpstreamProtocolFeicaiVideosV1})
			disabled := model.Channel{Id: disabledID, Type: constant.ChannelTypeSeedanceLink, Status: common.ChannelStatusManuallyDisabled, Models: "shared,inactive"}
			disabled.SetOtherSettings(dto.ChannelOtherSettings{VideoUpstreamProtocol: dto.VideoUpstreamProtocolModelArkV3Volcengine})
			secondDisabled := model.Channel{Id: 3, Type: constant.ChannelTypeSeedanceLink, Status: common.ChannelStatusManuallyDisabled, Models: "inactive"}
			secondDisabled.SetOtherSettings(dto.ChannelOtherSettings{VideoUpstreamProtocol: dto.VideoUpstreamProtocolFeicaiVideosV1})
			require.NoError(t, db.Create(&enabled).Error)
			require.NoError(t, db.Create(&disabled).Error)
			require.NoError(t, db.Create(&secondDisabled).Error)
			protocols := readSeedanceProtocols(db)
			assert.Equal(t, map[dto.VideoUpstreamProtocol]bool{dto.VideoUpstreamProtocolFeicaiVideosV1: true}, protocols["shared"])
			assert.Equal(t, map[dto.VideoUpstreamProtocol]bool{dto.VideoUpstreamProtocolFeicaiVideosV1: true, dto.VideoUpstreamProtocolModelArkV3Volcengine: true}, protocols["inactive"])
		})
	}
}
