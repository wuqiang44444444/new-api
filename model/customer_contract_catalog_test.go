package model

import (
	"errors"
	"fmt"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// 已接入的类型化渠道(MiniMax Link / Seedance / Batch)不写 Ability,但它们的
// 客户模型必须完整进入管理目录;残留 Ability 不得补充资格或发布额外模型。
func TestCustomerContractCatalogCoversTypedChannelsWithoutAbilities(t *testing.T) {
	db := setupCustomerContractTestDB(t)
	miniMax := Channel{Name: "minimax-link", Type: constant.ChannelTypeMiniMaxLink, Status: common.ChannelStatusEnabled, Group: "default", Models: "MiniMax-H3,MiniMax-M3"}
	require.NoError(t, db.Create(&miniMax).Error)
	batch := Channel{Name: "azure-batch", Type: constant.ChannelTypeAzureBatch, Status: common.ChannelStatusEnabled, Group: "contract-b", Models: "batch-model"}
	require.NoError(t, db.Create(&batch).Error)
	seedance := Channel{Name: "seedance", Type: constant.ChannelTypeSeedanceLink, Status: common.ChannelStatusEnabled, Group: "contract-a", Models: "video-model"}
	require.NoError(t, db.Create(&seedance).Error)
	// Residual ability of a typed channel must not publish extra models.
	require.NoError(t, db.Create(&Ability{Group: "contract-a", Model: "ghost-video", ChannelId: seedance.Id, Enabled: true}).Error)

	catalog, err := GetCustomerContractCatalog()
	require.NoError(t, err)
	type key struct {
		model, group string
		channel      int
	}
	got := make(map[key]CustomerContractCatalogSource)
	for _, source := range catalog.Sources {
		got[key{source.PublicModel, source.RouteGroup, source.ChannelId}] = source
	}
	for _, expected := range []key{
		{"MiniMax-H3", "default", miniMax.Id},
		{"MiniMax-M3", "default", miniMax.Id},
		{"batch-model", "contract-b", batch.Id},
		{"video-model", "contract-a", seedance.Id},
	} {
		source, ok := got[expected]
		require.True(t, ok, "expected source %v in catalog", expected)
		assert.True(t, source.Available, "typed source %v must be available", expected)
		assert.True(t, source.FromChannelConfig)
		assert.False(t, source.FromAbility)
		assert.Empty(t, source.ConfigDiff, "typed channels do not require generic abilities")
	}
	for _, source := range catalog.Sources {
		assert.NotEqual(t, "ghost-video", source.PublicModel, "typed channels never gain models from residual abilities")
	}
}

// 普通渠道的配置/Ability 双向差异必须分别诊断:配置有而 Ability 无/停用不可选,
// Ability 有而配置缺少模型或组仍按共享校验判定,不得仅凭差异禁选。
func TestCustomerContractCatalogReportsOrdinaryConfigAbilityDiffs(t *testing.T) {
	db := setupCustomerContractTestDB(t)
	healthy := createCustomerContractAbility(t, db, "contract-a", "model-a", common.ChannelStatusEnabled)
	disabledAbility := createCustomerContractAbility(t, db, "contract-a", "disabled-ability-model", common.ChannelStatusEnabled)
	require.NoError(t, db.Model(&Ability{}).Where("channel_id = ?", disabledAbility.Id).Update("enabled", false).Error)
	drift := Channel{Name: "drift", Status: common.ChannelStatusEnabled, Group: "contract-a", Models: "config-only-model"}
	require.NoError(t, db.Create(&drift).Error)
	require.NoError(t, db.Create(&Ability{Group: "contract-a", Model: "ability-only-model", ChannelId: drift.Id, Enabled: true}).Error)
	deleted := createCustomerContractAbility(t, db, "contract-a", "deleted-channel-model", common.ChannelStatusEnabled)
	require.NoError(t, db.Delete(&Channel{}, deleted.Id).Error)

	catalog, err := GetCustomerContractCatalog()
	require.NoError(t, err)
	type key struct {
		model, group string
		channel      int
	}
	got := make(map[key]CustomerContractCatalogSource)
	for _, source := range catalog.Sources {
		got[key{source.PublicModel, source.RouteGroup, source.ChannelId}] = source
	}
	healthySource, ok := got[key{"model-a", "contract-a", healthy.Id}]
	require.True(t, ok, "healthy source must be in the catalog")
	assert.True(t, healthySource.Available)
	assert.Empty(t, healthySource.ConfigDiff, "a healthy source carries no diff")

	disabledSource, ok := got[key{"disabled-ability-model", "contract-a", disabledAbility.Id}]
	require.True(t, ok)
	assert.False(t, disabledSource.Available)
	assert.Equal(t, ContractRouteUnavailableCapabilityMissing, disabledSource.UnavailableCategory)
	assert.Equal(t, ContractCatalogDiffConfigOnly, disabledSource.ConfigDiff)

	configOnly, ok := got[key{"config-only-model", "contract-a", drift.Id}]
	require.True(t, ok)
	assert.False(t, configOnly.Available)
	assert.Equal(t, ContractRouteUnavailableCapabilityMissing, configOnly.UnavailableCategory)
	assert.Equal(t, ContractCatalogDiffConfigOnly, configOnly.ConfigDiff)

	abilityOnly, ok := got[key{"ability-only-model", "contract-a", drift.Id}]
	require.True(t, ok)
	assert.True(t, abilityOnly.Available, "enabled Ability source stays addable despite the config drift")
	assert.Equal(t, ContractCatalogDiffAbilityOnly, abilityOnly.ConfigDiff)

	for _, source := range catalog.Sources {
		assert.NotEqual(t, deleted.Id, source.ChannelId, "abilities of deleted channels are not sources")
	}
}

// 无组渠道只进入独立诊断集合,不进入组聚合、不创造 default/auto 来源;
// 真实组缺少倍率时沿用既有 route_group_invalid 诊断。
func TestCustomerContractCatalogNoGroupChannelsAndInvalidRatioGroups(t *testing.T) {
	db := setupCustomerContractTestDB(t)
	orphan := Channel{Name: "orphan", Status: common.ChannelStatusEnabled, Group: "", Models: "orphan-model,orphan-model, spaced-model "}
	require.NoError(t, db.Create(&orphan).Error)
	// The column default fills an empty group with "default" on insert; a
	// channel without route groups arises when the administrator clears it.
	require.NoError(t, db.Model(&Channel{}).Where("id = ?", orphan.Id).Update("group", "").Error)
	ghost := Channel{Name: "ghost-ratio", Status: common.ChannelStatusEnabled, Group: "ghost-group", Models: "ghost-model"}
	require.NoError(t, db.Create(&ghost).Error)

	catalog, err := GetCustomerContractCatalog()
	require.NoError(t, err)
	require.Len(t, catalog.NoGroupRecords, 1)
	record := catalog.NoGroupRecords[0]
	assert.Equal(t, orphan.Id, record.ChannelId)
	assert.Equal(t, []string{"orphan-model", "spaced-model"}, record.Models, "models are trimmed and exactly deduplicated")
	for _, source := range catalog.Sources {
		assert.NotEqual(t, orphan.Id, source.ChannelId, "no-group channels never produce group sources")
		assert.NotEqual(t, "auto", source.RouteGroup)
	}
	require.Len(t, catalog.Sources, 1)
	assert.Equal(t, "ghost-model", catalog.Sources[0].PublicModel)
	assert.Equal(t, "ghost-group", catalog.Sources[0].RouteGroup)
	assert.False(t, catalog.Sources[0].Available)
	assert.Equal(t, ContractRouteUnavailableGroupInvalid, catalog.Sources[0].UnavailableCategory)
}

// 目录读取失败必须整体报错,不能返回空目录或部分成功结果。
func TestCustomerContractCatalogReadFailureFailsWholeCatalog(t *testing.T) {
	for _, table := range []string{"channels", "abilities"} {
		t.Run(table, func(t *testing.T) {
			db := setupCustomerContractTestDB(t)
			createCustomerContractAbility(t, db, "contract-a", "model-a", common.ChannelStatusEnabled)
			failure := errors.New("catalog source unavailable")
			require.NoError(t, db.Callback().Query().Before("gorm:query").Register("test:fail_catalog_read", func(tx *gorm.DB) {
				if tx.Statement.Table == table {
					tx.AddError(failure)
				}
			}))
			catalog, err := GetCustomerContractCatalog()
			require.ErrorIs(t, err, failure)
			assert.Nil(t, catalog)
		})
	}
}

// Ability 分批装载必须覆盖批边界:同一渠道的事实跨批复用后,投影与共享资格
// 结果保持一致、组合无遗漏;任一批读取失败整体报错。
func TestCustomerContractCatalogAbilityBatchesStayComplete(t *testing.T) {
	const ordinaryChannels = 1001
	for _, failSecondBatch := range []bool{false, true} {
		t.Run(fmt.Sprintf("second_batch_failure=%t", failSecondBatch), func(t *testing.T) {
			db := setupCustomerContractTestDB(t)
			for i := 0; i < ordinaryChannels; i++ {
				channel := Channel{Name: fmt.Sprintf("bulk-%04d", i), Status: common.ChannelStatusEnabled, Group: "contract-a", Models: fmt.Sprintf("bulk-model-%04d", i)}
				require.NoError(t, db.Create(&channel).Error)
				require.NoError(t, db.Create(&Ability{Group: "contract-a", Model: fmt.Sprintf("bulk-model-%04d", i), ChannelId: channel.Id, Enabled: true}).Error)
			}
			abilityQueries := 0
			failure := errors.New("second ability batch unavailable")
			require.NoError(t, db.Callback().Query().Before("gorm:query").Register("test:catalog_ability_batches", func(tx *gorm.DB) {
				if tx.Statement.Table == "abilities" {
					abilityQueries++
					if failSecondBatch && abilityQueries == 2 {
						tx.AddError(failure)
					}
				}
			}))
			catalog, err := GetCustomerContractCatalog()
			if failSecondBatch {
				require.ErrorIs(t, err, failure)
				assert.Nil(t, catalog)
				return
			}
			require.NoError(t, err)
			assert.GreaterOrEqual(t, abilityQueries, 2, "the fixture must cross the documented batch boundary")
			require.Len(t, catalog.Sources, ordinaryChannels)
			for _, source := range catalog.Sources {
				assert.True(t, source.Available, "every batched source keeps its shared qualification")
			}
		})
	}
}

// 精确身份去重:同一来源来自配置与 Ability 时合并标志且无差异;大小写不同
// 的模型保持两条独立来源,各自携带自己的差异诊断。
func TestCustomerContractCatalogDedupesExactTriplesAndKeepsCaseIdentities(t *testing.T) {
	db := setupCustomerContractTestDB(t)
	channel := Channel{Name: "case-channel", Status: common.ChannelStatusEnabled, Group: "contract-a", Models: "Model-A"}
	require.NoError(t, db.Create(&channel).Error)
	require.NoError(t, db.Create(&Ability{Group: "contract-a", Model: "Model-A", ChannelId: channel.Id, Enabled: true}).Error)
	require.NoError(t, db.Create(&Ability{Group: "contract-a", Model: "model-a", ChannelId: channel.Id, Enabled: true}).Error)

	catalog, err := GetCustomerContractCatalog()
	require.NoError(t, err)
	require.Len(t, catalog.Sources, 2)
	byModel := make(map[string]CustomerContractCatalogSource, len(catalog.Sources))
	for _, source := range catalog.Sources {
		byModel[source.PublicModel] = source
	}
	merged := byModel["Model-A"]
	require.Equal(t, channel.Id, merged.ChannelId)
	assert.True(t, merged.FromChannelConfig)
	assert.True(t, merged.FromAbility)
	assert.Empty(t, merged.ConfigDiff)
	abilityOnly := byModel["model-a"]
	assert.False(t, abilityOnly.FromChannelConfig)
	assert.True(t, abilityOnly.FromAbility)
	assert.Equal(t, ContractCatalogDiffAbilityOnly, abilityOnly.ConfigDiff)
	assert.True(t, abilityOnly.Available)
}

func TestCustomerContractCatalogNoGroupDiagnosticsCoexistWithAbilitySources(t *testing.T) {
	db := setupCustomerContractTestDB(t)
	channel := createCustomerContractAbility(t, db, "contract-a", "model-a", common.ChannelStatusEnabled)
	require.NoError(t, db.Model(&Channel{}).Where("id = ?", channel.Id).Update("group", "").Error)
	catalog, err := GetCustomerContractCatalog()
	require.NoError(t, err)
	require.Len(t, catalog.NoGroupRecords, 1)
	assert.Equal(t, channel.Id, catalog.NoGroupRecords[0].ChannelId)
	assert.Equal(t, []string{"model-a"}, catalog.NoGroupRecords[0].Models)
	require.Len(t, catalog.Sources, 1)
	source := catalog.Sources[0]
	assert.Equal(t, channel.Id, source.ChannelId)
	assert.Equal(t, "contract-a", source.RouteGroup)
	assert.True(t, source.Available)
	assert.Equal(t, ContractCatalogDiffAbilityOnly, source.ConfigDiff)
}

func TestCustomerContractCatalogAbilityOnlyStillRequiresEnabledChannelAndValidGroup(t *testing.T) {
	for _, tc := range []struct {
		name, group, reason string
		status              int
	}{
		{"disabled channel", "contract-a", ContractRouteUnavailableChannelDisabled, common.ChannelStatusManuallyDisabled},
		{"unconfigured group", "removed-group", ContractRouteUnavailableGroupInvalid, common.ChannelStatusEnabled},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db := setupCustomerContractTestDB(t)
			channel := createCustomerContractAbility(t, db, tc.group, "model-a", tc.status)
			require.NoError(t, db.Model(&Channel{}).Where("id = ?", channel.Id).Update("models", "").Error)
			catalog, err := GetCustomerContractCatalog()
			require.NoError(t, err)
			require.Len(t, catalog.Sources, 1)
			source := catalog.Sources[0]
			assert.Equal(t, ContractCatalogDiffAbilityOnly, source.ConfigDiff)
			assert.False(t, source.Available)
			assert.Equal(t, tc.reason, source.UnavailableCategory)
		})
	}
}
