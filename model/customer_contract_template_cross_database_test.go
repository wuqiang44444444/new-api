package model

import (
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func templateTestSQLiteDialector(dsn string) gorm.Dialector {
	return sqlite.Open(dsn)
}

func templateTestMySQLDialector(dsn string) gorm.Dialector {
	return mysql.Open(dsn)
}

func templateTestPostgresDialector(dsn string) gorm.Dialector {
	return postgres.Open(dsn)
}

func TestCustomerContractTemplatePersistenceAcrossSupportedServerDatabases(t *testing.T) {
	tests := []struct {
		name      string
		env       string
		database  common.DatabaseType
		dialector func(string) gorm.Dialector
	}{
		{name: "sqlite", env: "", database: common.DatabaseTypeSQLite, dialector: templateTestSQLiteDialector},
		{name: "mysql", env: "TEST_CUSTOMER_CONTRACT_MYSQL_DSN", database: common.DatabaseTypeMySQL, dialector: templateTestMySQLDialector},
		{name: "postgres", env: "TEST_CUSTOMER_CONTRACT_POSTGRES_DSN", database: common.DatabaseTypePostgreSQL, dialector: templateTestPostgresDialector},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var db *gorm.DB
			var err error
			if test.env != "" {
				dsn := os.Getenv(test.env)
				if dsn == "" {
					t.Skipf("%s is not configured", test.env)
				}
				db, err = gorm.Open(test.dialector(dsn), &gorm.Config{})
				require.NoError(t, err)
			} else {
				dsn := "file:" + strings.ReplaceAll(t.Name(), "/", "_") + "?mode=memory&cache=shared"
				db, err = gorm.Open(test.dialector(dsn), &gorm.Config{})
				require.NoError(t, err)
			}
			for _, table := range []any{&User{}, &Channel{}, &Ability{}, &Token{}, &CustomerModelContract{}, &CustomerContract{}, &CustomerContractEntityRule{}, &CustomerContractEntityAudit{}, &CustomerContractTemplate{}, &CustomerContractTemplateRule{}, &CustomerContractTemplateAudit{}} {
				if db.Migrator().HasTable(table) {
					t.Skipf("refusing to use non-empty %s test database", test.name)
				}
			}
			previousDB := DB
			previousMainType := common.MainDatabaseType()
			previousLogType := common.LogDatabaseType()
			previousRedis := common.RedisEnabled
			previousRatios := ratio_setting.GroupRatio2JSONString()
			DB = db
			common.RedisEnabled = false
			common.SetDatabaseTypes(test.database, test.database)
			initCol()
			require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(`{"default":1,"contract-cross-db":0.87}`))
			managedTables := false
			t.Cleanup(func() {
				if managedTables {
					_ = db.Migrator().DropTable(&CustomerContractTemplateAudit{}, &CustomerContractTemplateRule{}, &CustomerContractTemplate{}, &CustomerContractEntityAudit{}, &CustomerContractEntityRule{}, &CustomerContract{}, &CustomerModelContract{}, &Token{}, &Ability{}, &Channel{}, &User{})
				}
				DB = previousDB
				common.RedisEnabled = previousRedis
				common.SetDatabaseTypes(previousMainType, previousLogType)
				initCol()
				_ = ratio_setting.UpdateGroupRatioByJSONString(previousRatios)
				sqlDB, sqlErr := db.DB()
				if sqlErr == nil {
					_ = sqlDB.Close()
				}
			})
			require.NoError(t, db.AutoMigrate(
				&User{}, &Channel{}, &Ability{}, &Token{},
				&CustomerModelContract{}, &CustomerContract{}, &CustomerContractEntityRule{}, &CustomerContractEntityAudit{},
				&CustomerContractTemplate{}, &CustomerContractTemplateRule{}, &CustomerContractTemplateAudit{},
			))
			managedTables = true
			admin := User{Username: "template-admin-" + test.name, AffCode: "template-admin-aff-" + test.name, Role: common.RoleAdminUser, AuthVersion: 1}
			user := User{Username: "template-user-" + test.name, AffCode: "template-user-aff-" + test.name, Group: "default", AuthVersion: 1}
			require.NoError(t, db.Create(&admin).Error)
			require.NoError(t, db.Create(&user).Error)
			channel := Channel{Name: "template-cross-channel", Group: "contract-cross-db", Models: "cross-template-model", Key: "test-key", Status: common.ChannelStatusEnabled}
			require.NoError(t, db.Create(&channel).Error)
			priority := int64(0)
			require.NoError(t, db.Create(&Ability{
				Group: "contract-cross-db", Model: "cross-template-model", ChannelId: channel.Id, Enabled: true, Priority: &priority,
			}).Error)

			template, err := CreateCustomerContractTemplate(CreateCustomerContractTemplateParams{
				AdminUserId: admin.Id, Name: "Cross DB Template", Enabled: true,
				Reason: "cross database template fixture",
				Rules: []CustomerContractEntityRuleInput{
					{PublicModel: "cross-template-model", ChannelId: channel.Id, RouteGroup: "contract-cross-db", RatioUnits: 80_000_000},
				},
			})
			require.NoError(t, err)

			// Order 1: template change wins first, so the stale confirmed
			// version cannot create a contract.
			_, err = ReplaceCustomerContractTemplate(ReplaceCustomerContractTemplateParams{
				TemplateId: template.Id, AdminUserId: admin.Id, ExpectedVersion: template.Version,
				Name: "Cross DB Template", Reason: "bump template first",
				Rules: []CustomerContractEntityRuleInput{
					{PublicModel: "cross-template-model", ChannelId: channel.Id, RouteGroup: "contract-cross-db", RatioUnits: 80_000_000},
				},
			})
			require.NoError(t, err)

			// With the template now at version 2, the stale confirmed version 1
			// must not create a contract.
			_, err = CreateCustomerContractEntity(CreateCustomerContractParams{
				UserId: user.Id, AdminUserId: admin.Id, Name: "Stale", Enabled: true,
				Reason: "stale template version", Rules: templateRulesForCrossDb(channel.Id),
				SourceTemplateId: template.Id, SourceTemplateVersion: 1,
			})
			require.ErrorIs(t, err, ErrCustomerContractTemplateVersionConflict)

			// Order 2: a committed contract is never rewritten by later
			// template changes.
			created, err := CreateCustomerContractEntity(CreateCustomerContractParams{
				UserId: user.Id, AdminUserId: admin.Id, Name: "Committed", Enabled: true,
				Reason: "created at version 2", Rules: templateRulesForCrossDb(channel.Id),
				SourceTemplateId: template.Id, SourceTemplateVersion: 2,
			})
			require.NoError(t, err)
			assert.EqualValues(t, 2, created.SourceTemplateVersion)
			assert.Equal(t, template.Id, created.SourceTemplateId)
			assert.Equal(t, "Cross DB Template", created.SourceTemplateName)

			_, err = ReplaceCustomerContractTemplate(ReplaceCustomerContractTemplateParams{
				TemplateId: template.Id, AdminUserId: admin.Id, ExpectedVersion: 2,
				Name: "Cross DB Template", Reason: "later template change",
				Rules: templateRulesForCrossDb(channel.Id),
			})
			require.NoError(t, err)
			after, err := GetContractEntitySnapshot(created.Id, false)
			require.NoError(t, err)
			assert.EqualValues(t, 2, after.SourceTemplateVersion, "later template changes never rewrite committed contracts")
			assert.EqualValues(t, 1, after.Version, "the contract itself is unchanged")
			assert.EqualValues(t, 70_000_000, after.Rules[0].RatioUnits)

			// Controlled concurrency: two real goroutines race on the same
			// expected version; exactly one wins, the other gets a version
			// conflict, and the loser never overwrites the winner. SQLite runs
			// the sequential fencing above; the cross-connection race needs a
			// real server database.
			if test.env == "" {
				return
			}
			// Each transaction leases its own pool connection. Never mutate the
			// package DB pointer from concurrent workers.
			start := make(chan struct{})
			results := make(chan error, 2)
			for _, ratio := range []int64{60_000_000, 50_000_000} {
				go func(ratioUnits int64) {
					<-start
					_, replaceErr := ReplaceCustomerContractTemplate(ReplaceCustomerContractTemplateParams{
						TemplateId: template.Id, AdminUserId: admin.Id, ExpectedVersion: 3,
						Name: "Cross DB Template", Reason: "concurrent template editor",
						Rules: []CustomerContractEntityRuleInput{
							{PublicModel: "cross-template-model", ChannelId: channel.Id, RouteGroup: "contract-cross-db", RatioUnits: ratioUnits},
						},
					})
					results <- replaceErr
				}(ratio)
			}
			close(start)
			successes, conflicts := 0, 0
			for range 2 {
				replaceErr := <-results
				switch {
				case replaceErr == nil:
					successes++
				case errors.Is(replaceErr, ErrCustomerContractTemplateVersionConflict):
					conflicts++
				default:
					require.NoError(t, replaceErr)
				}
			}
			assert.Equal(t, 1, successes, "exactly one concurrent editor wins")
			assert.Equal(t, 1, conflicts, "the stale editor loses with a version conflict")
			finalTemplate, err := GetContractTemplateSnapshot(template.Id, false)
			require.NoError(t, err)
			assert.EqualValues(t, 4, finalTemplate.Version)
		})
	}
}

func templateRulesForCrossDb(channelId int) []CustomerContractEntityRuleInput {
	return []CustomerContractEntityRuleInput{
		{PublicModel: "cross-template-model", ChannelId: channelId, RouteGroup: "contract-cross-db", RatioUnits: 70_000_000},
	}
}
