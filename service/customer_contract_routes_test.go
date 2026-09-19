package service

import (
	"errors"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestEffectiveContractRulesBatchSourcesAndFailClosed(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	raw, err := db.DB()
	require.NoError(t, err)
	previousDB := model.DB
	previousGroups := ratio_setting.GroupRatio2JSONString()
	previousUsable := setting.UserUsableGroups2JSONString()
	model.DB = db
	t.Cleanup(func() {
		model.DB = previousDB
		require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(previousGroups))
		require.NoError(t, setting.UpdateUserUsableGroupsByJSONString(previousUsable))
		require.NoError(t, raw.Close())
	})
	require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(`{"default":1,"contract-a":1,"internal":1}`))
	require.NoError(t, setting.UpdateUserUsableGroupsByJSONString(`{"default":"Default","contract-a":"Contract","removed":"Removed"}`))
	require.NoError(t, db.AutoMigrate(&model.Channel{}, &model.Ability{}))
	channel := model.Channel{Status: common.ChannelStatusEnabled, Group: "contract-a", Models: "first,second,third"}
	require.NoError(t, db.Create(&channel).Error)
	require.NoError(t, db.Create([]model.Ability{
		{ChannelId: channel.Id, Group: "contract-a", Model: "first", Enabled: true},
		{ChannelId: channel.Id, Group: "contract-a", Model: "second", Enabled: true},
		{ChannelId: channel.Id, Group: "contract-a", Model: "third", Enabled: true},
		{ChannelId: channel.Id, Group: "internal", Model: "internal", Enabled: true},
	}).Error)
	snapshot := &model.ContractEntitySnapshot{Id: 1, UserId: 1, Version: 1, Enabled: true, Rules: []model.ContractEntityRule{
		{ChannelId: channel.Id, RouteGroup: "contract-a", PublicModel: "first", RatioUnits: 80_000_000},
		{ChannelId: channel.Id, RouteGroup: "contract-a", PublicModel: "second", RatioUnits: 80_000_000},
		{ChannelId: channel.Id, RouteGroup: "contract-a", PublicModel: "third", RatioUnits: 80_000_000},
		{ChannelId: channel.Id, RouteGroup: "internal", PublicModel: "internal", RatioUnits: 80_000_000},
		{ChannelId: channel.Id, RouteGroup: "removed", PublicModel: "removed", RatioUnits: 80_000_000},
	}}
	queries := 0
	failRead := false
	require.NoError(t, db.Callback().Query().Before("gorm:query").Register("test:route_source_read", func(tx *gorm.DB) {
		queries++
		if failRead {
			tx.AddError(errors.New("source read unavailable"))
		}
	}))
	rules, err := EffectiveContractRules(snapshot)
	require.NoError(t, err)
	require.Len(t, rules, 4)
	assert.Equal(t, []string{"first", "second", "third", "internal"}, []string{rules[0].PublicModel, rules[1].PublicModel, rules[2].PublicModel, rules[3].PublicModel})
	assert.LessOrEqual(t, queries, 2)
	assert.False(t, snapshot.Rules[0].Available, "runtime reads must not mutate the frozen contract")

	failRead = true
	rules, err = EffectiveContractRules(snapshot)
	require.ErrorIs(t, err, ErrCustomerContractUnavailable)
	assert.Nil(t, rules, "a failed read must not grant native routing or return partial candidates")
}
