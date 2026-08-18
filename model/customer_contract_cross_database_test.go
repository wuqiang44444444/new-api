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

func TestCustomerContractPersistenceAcrossSupportedServerDatabases(t *testing.T) {
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
			for _, table := range []any{&User{}, &Channel{}, &Ability{}, &CustomerModelContract{}, &CustomerContractAudit{}} {
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
					_ = db.Migrator().DropTable(&CustomerContractAudit{}, &CustomerModelContract{}, &Ability{}, &Channel{}, &User{})
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
				&CustomerModelContract{},
				&CustomerContractAudit{},
			))
			managedTables = true

			admin, user := createCustomerContractFixture(t, db)
			createCustomerContractAbility(t, db, "contract-cross-db", "cross-db-model", common.ChannelStatusEnabled)
			_, err = ReplaceCustomerContract(ReplaceCustomerContractParams{
				UserId: user.Id, AdminUserId: admin.Id, ExpectedVersion: 0, Enabled: true,
				Reason: "case-only duplicates are rejected consistently",
				Rules: []CustomerContractRule{
					{PublicModel: "Cross-DB-Model", RouteGroup: "contract-cross-db", RatioUnits: 80_000_000},
					{PublicModel: "cross-db-model", RouteGroup: "contract-cross-db", RatioUnits: 80_000_000},
				},
			})
			require.ErrorIs(t, err, ErrCustomerContractInvalidRule)

			snapshot, err := ReplaceCustomerContract(ReplaceCustomerContractParams{
				UserId: user.Id, AdminUserId: admin.Id, ExpectedVersion: 0, Enabled: true,
				Reason: "cross database contract transaction",
				Rules:  []CustomerContractRule{{PublicModel: "cross-db-model", RouteGroup: "contract-cross-db", RatioUnits: 80_000_000}},
			})
			require.NoError(t, err)
			assert.True(t, snapshot.Enabled)
			assert.EqualValues(t, 1, snapshot.Version)
			require.Len(t, snapshot.Rules, 1)

			duplicate := CustomerModelContract{
				UserId: user.Id, PublicModel: "cross-db-model", RouteGroup: "contract-cross-db", RatioUnits: 50_000_000,
			}
			require.Error(t, db.Create(&duplicate).Error, "the user/model unique index must be enforced")

			_, err = ReplaceCustomerContract(ReplaceCustomerContractParams{
				UserId: user.Id, AdminUserId: admin.Id, ExpectedVersion: 0, Enabled: true,
				Reason: "stale editor must lose",
				Rules:  []CustomerContractRule{{PublicModel: "cross-db-model", RouteGroup: "contract-cross-db", RatioUnits: 50_000_000}},
			})
			require.ErrorIs(t, err, ErrCustomerContractVersionConflict)

			audits, total, err := GetCustomerContractAudits(user.Id, 0, 20)
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
					_, replaceErr := ReplaceCustomerContract(ReplaceCustomerContractParams{
						UserId: concurrentUser.Id, AdminUserId: admin.Id, ExpectedVersion: 0, Enabled: true,
						Reason: "concurrent editor",
						Rules:  []CustomerContractRule{{PublicModel: "cross-db-model", RouteGroup: "contract-cross-db", RatioUnits: ratioUnits}},
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
