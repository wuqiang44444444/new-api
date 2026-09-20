package model

import (
	"fmt"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupCustomerContractTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	previousDB := DB
	previousDatabaseType := common.MainDatabaseType()
	previousRedisEnabled := common.RedisEnabled
	previousGroupRatios := ratio_setting.GroupRatio2JSONString()

	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "_"))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(
		&User{},
		&Channel{},
		&Ability{},
		&Token{},
		&CustomerModelContract{},
		&CustomerContractAudit{},
		&CustomerContract{},
		&CustomerContractEntityRule{},
		&CustomerContractEntityAudit{},
		&CustomerContractTemplate{},
		&CustomerContractTemplateRule{},
		&CustomerContractTemplateAudit{},
	))
	DB = db
	common.RedisEnabled = false
	common.SetMainDatabaseType(common.DatabaseTypeSQLite)
	initCol()
	require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(`{"default":1,"contract-a":0.87,"contract-b":1}`))

	sqlDB, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() {
		DB = previousDB
		common.RedisEnabled = previousRedisEnabled
		common.SetMainDatabaseType(previousDatabaseType)
		initCol()
		require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(previousGroupRatios))
		_ = sqlDB.Close()
	})
	return db
}

func createCustomerContractFixture(t *testing.T, db *gorm.DB) (User, User) {
	t.Helper()
	admin := User{Username: "contract-admin", AffCode: "contract-admin-aff", Role: common.RoleAdminUser, AuthVersion: 1}
	user := User{Username: "contract-user", AffCode: "contract-user-aff", Group: "default", AuthVersion: 1}
	require.NoError(t, db.Create(&admin).Error)
	require.NoError(t, db.Create(&user).Error)
	return admin, user
}

func createCustomerContractAbility(t *testing.T, db *gorm.DB, group string, modelName string, status int) Channel {
	t.Helper()
	channel := Channel{Name: group + "-channel", Group: group, Models: modelName, Key: "test-key", Status: status}
	require.NoError(t, db.Create(&channel).Error)
	priority := int64(0)
	require.NoError(t, db.Create(&Ability{
		Group: group, Model: modelName, ChannelId: channel.Id, Enabled: true, Priority: &priority,
	}).Error)
	return channel
}

func TestCustomerContractAvailabilityUsesExactCaseAndEnabledChannels(t *testing.T) {
	db := setupCustomerContractTestDB(t)
	createCustomerContractFixture(t, db)
	createCustomerContractAbility(t, db, "contract-a", "Model-A", common.ChannelStatusEnabled)
	createCustomerContractAbility(t, db, "contract-a", "disabled-model", common.ChannelStatusManuallyDisabled)

	models, err := GetCustomerContractAvailableModelsForGroup("contract-a")
	require.NoError(t, err)
	assert.Contains(t, models, "Model-A")
	assert.NotContains(t, models, "model-a")
	assert.NotContains(t, models, "disabled-model")
}
