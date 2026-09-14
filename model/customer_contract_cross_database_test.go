package model

import (
	"errors"
	"os"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func TestCustomerContractEntityPersistenceAcrossSupportedServerDatabases(t *testing.T) {
	tests := []struct {
		name      string
		env       string
		database  common.DatabaseType
		dialector func(string) gorm.Dialector
	}{
		{name: "mysql", env: "TEST_CUSTOMER_CONTRACT_MYSQL_DSN", database: common.DatabaseTypeMySQL, dialector: func(dsn string) gorm.Dialector { return mysql.Open(dsn) }},
		{name: "postgres", env: "TEST_CUSTOMER_CONTRACT_POSTGRES_DSN", database: common.DatabaseTypePostgreSQL, dialector: func(dsn string) gorm.Dialector { return postgres.Open(dsn) }},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			dsn := os.Getenv(test.env)
			if dsn == "" {
				t.Skipf("%s is not configured", test.env)
			}

			db, err := gorm.Open(test.dialector(dsn), &gorm.Config{})
			require.NoError(t, err)
			for _, table := range []any{&User{}, &Channel{}, &Ability{}, &Token{}, &CustomerModelContract{}, &CustomerContract{}, &CustomerContractEntityRule{}, &CustomerContractEntityAudit{}} {
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
					_ = db.Migrator().DropTable(&CustomerContractEntityAudit{}, &CustomerContractEntityRule{}, &CustomerContract{}, &CustomerModelContract{}, &Token{}, &Ability{}, &Channel{}, &User{})
				}
				DB = previousDB
				common.RedisEnabled = previousRedis
				common.SetDatabaseTypes(previousMainType, previousLogType)
				initCol()
				require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(previousRatios))
				sqlDB, sqlErr := db.DB()
				if sqlErr == nil {
					_ = sqlDB.Close()
				}
			})

			require.NoError(t, db.AutoMigrate(
				&User{},
				&Channel{},
				&Ability{},
				&Token{},
				&CustomerModelContract{},
				&CustomerContract{},
				&CustomerContractEntityRule{},
				&CustomerContractEntityAudit{},
			))
			managedTables = true
			// Recreate the pre-upgrade index with a real persisted single-channel rule.
			require.NoError(t, db.Migrator().DropIndex(&CustomerContractEntityRule{}, "idx_cc_entity_rule_contract_model_channel"))
			require.NoError(t, db.Exec("CREATE UNIQUE INDEX idx_cc_entity_rule_contract_model ON customer_contract_entity_rules (contract_id, public_model)").Error)

			admin, user := createCustomerContractFixture(t, db)
			channel := createCustomerContractAbility(t, db, "contract-cross-db", "cross-db-model", common.ChannelStatusEnabled)

			_, err = CreateCustomerContractEntity(CreateCustomerContractParams{
				UserId: user.Id, AdminUserId: admin.Id, Name: "Cross DB", Enabled: true,
				Reason: "case-only duplicates are rejected consistently",
				Rules: []CustomerContractEntityRuleInput{
					{PublicModel: "Cross-DB-Model", ChannelId: channel.Id, RouteGroup: "contract-cross-db", RatioUnits: 80_000_000},
					{PublicModel: "cross-db-model", ChannelId: channel.Id, RouteGroup: "contract-cross-db", RatioUnits: 80_000_000},
				},
			})
			require.ErrorIs(t, err, ErrCustomerContractInvalidRule)

			snapshot, err := CreateCustomerContractEntity(CreateCustomerContractParams{
				UserId: user.Id, AdminUserId: admin.Id, Name: "Cross DB", Enabled: true,
				Reason: "cross database contract transaction",
				Rules:  []CustomerContractEntityRuleInput{{PublicModel: "cross-db-model", ChannelId: channel.Id, RouteGroup: "contract-cross-db", RatioUnits: 80_000_000}},
			})
			require.NoError(t, err)
			assert.True(t, snapshot.Enabled)
			assert.EqualValues(t, 1, snapshot.Version)
			require.Len(t, snapshot.Rules, 1)
			assert.Equal(t, channel.Id, snapshot.Rules[0].ChannelId)

			var original CustomerContractEntityRule
			require.NoError(t, db.Where("contract_id = ?", snapshot.Id).First(&original).Error)
			for range 2 {
				require.NoError(t, migrateCustomerContractEntityRuleIndex(db))
				require.NoError(t, db.AutoMigrate(&CustomerContractEntityRule{}))
				assert.False(t, db.Migrator().HasIndex(&CustomerContractEntityRule{}, "idx_cc_entity_rule_contract_model"))
				var preserved CustomerContractEntityRule
				require.NoError(t, db.First(&preserved, original.Id).Error)
				assert.Equal(t, original, preserved, "upgrade must not rewrite historical rule facts")
			}

			listedUsers, _, err := SearchUsers(user.Username, "", nil, nil, 0, 20)
			require.NoError(t, err)
			require.Len(t, listedUsers, 1)
			require.NotNil(t, listedUsers[0].ContractSummary)
			assert.Equal(t, UserContractSummary{Total: 1, Enabled: 1}, *listedUsers[0].ContractSummary)

			duplicate := CustomerContractEntityRule{
				ContractId: snapshot.Id, PublicModel: "cross-db-model", ChannelId: channel.Id,
				RouteGroup: "contract-cross-db", RatioUnits: 50_000_000,
			}
			require.Error(t, db.Create(&duplicate).Error, "the contract/model/channel unique index must be enforced")

			// The same model on another channel and another model on the same
			// channel are both legal under the new unique index.
			otherChannel := createCustomerContractAbility(t, db, "contract-cross-db-b", "cross-db-model", common.ChannelStatusEnabled)
			require.NoError(t, db.Create(&CustomerContractEntityRule{
				ContractId: snapshot.Id, PublicModel: "cross-db-model", ChannelId: otherChannel.Id,
				RouteGroup: "contract-cross-db", RatioUnits: 80_000_000,
			}).Error)
			require.NoError(t, db.Create(&CustomerContractEntityRule{
				ContractId: snapshot.Id, PublicModel: "cross-db-model-2", ChannelId: channel.Id,
				RouteGroup: "contract-cross-db", RatioUnits: 80_000_000,
			}).Error)

			_, err = ReplaceCustomerContractEntity(ReplaceCustomerContractEntityParams{
				ContractId: snapshot.Id, AdminUserId: admin.Id, ExpectedVersion: 0, Name: "Cross DB",
				Reason: "stale editor must lose",
				Rules:  []CustomerContractEntityRuleInput{{PublicModel: "cross-db-model", ChannelId: channel.Id, RouteGroup: "contract-cross-db", RatioUnits: 50_000_000}},
			})
			require.ErrorIs(t, err, ErrCustomerContractVersionConflict)

			audits, total, err := GetContractEntityAudits(snapshot.Id, 0, 20)
			require.NoError(t, err)
			assert.EqualValues(t, 1, total)
			require.Len(t, audits, 1)
			assert.Equal(t, "cross database contract transaction", audits[0].Reason)

			items, listTotal, summary, err := GetCustomerContractAdminList(CustomerContractAdminListFilter{
				AdminRole: common.RoleRootUser, Keyword: "cross-db-model", Limit: 20,
			})
			require.NoError(t, err)
			assert.EqualValues(t, 1, listTotal)
			assert.EqualValues(t, 1, summary.Active)
			require.Len(t, items, 1)
			assert.Equal(t, snapshot.Id, items[0].ContractId)
			assert.Equal(t, user.Id, items[0].UserId)
			assert.Equal(t, CustomerContractAdminStatusActive, items[0].ContractStatus)

			concurrentUser := User{
				Username: "contract-concurrent-" + test.name, AffCode: "contract-concurrent-aff-" + test.name,
				Group: "default", AuthVersion: 1,
			}
			require.NoError(t, db.Create(&concurrentUser).Error)
			start := make(chan struct{})
			results := make(chan error, 2)
			for _, ratio := range []int64{60_000_000, 70_000_000} {
				go func(ratioUnits int64) {
					<-start
					_, replaceErr := ReplaceCustomerContractEntity(ReplaceCustomerContractEntityParams{
						ContractId: snapshot.Id, AdminUserId: admin.Id, ExpectedVersion: 1, Name: "Cross DB",
						Reason: "concurrent editor",
						Rules:  []CustomerContractEntityRuleInput{{PublicModel: "cross-db-model", ChannelId: channel.Id, RouteGroup: "contract-cross-db", RatioUnits: ratioUnits}},
					})
					results <- replaceErr
				}(ratio)
			}
			close(start)
			successes := 0
			conflicts := 0
			for range 2 {
				replaceErr := <-results
				switch {
				case replaceErr == nil:
					successes++
				case errors.Is(replaceErr, ErrCustomerContractVersionConflict):
					conflicts++
				default:
					require.NoError(t, replaceErr)
				}
			}
			assert.Equal(t, 1, successes)
			assert.Equal(t, 1, conflicts)
		})
	}
}
