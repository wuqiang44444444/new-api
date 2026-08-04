package model

import (
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func TestVideoAssetContractsOnExternalDatabases(t *testing.T) {
	tests := []struct {
		name         string
		dsnEnv       string
		databaseType common.DatabaseType
		open         func(string) gorm.Dialector
	}{
		{name: "MySQL", dsnEnv: "TEST_VIDEO_CONTRACT_MYSQL_DSN", databaseType: common.DatabaseTypeMySQL, open: func(dsn string) gorm.Dialector { return mysql.Open(dsn) }},
		{name: "PostgreSQL", dsnEnv: "TEST_VIDEO_CONTRACT_POSTGRES_DSN", databaseType: common.DatabaseTypePostgreSQL, open: func(dsn string) gorm.Dialector { return postgres.Open(dsn) }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			dsn := strings.TrimSpace(os.Getenv(test.dsnEnv))
			if dsn == "" {
				t.Skipf("set %s to a disposable test database DSN", test.dsnEnv)
			}
			require.Contains(t, strings.ToLower(dsn), "_test", "external contract tests require an explicitly named disposable *_test database")
			db, err := gorm.Open(test.open(dsn), &gorm.Config{})
			require.NoError(t, err)
			sqlDB, err := db.DB()
			require.NoError(t, err)
			t.Cleanup(func() { _ = sqlDB.Close() })

			originalDB, originalLogDB := DB, LOG_DB
			originalMainType, originalLogType := common.MainDatabaseType(), common.LogDatabaseType()
			DB, LOG_DB = db, db
			common.SetDatabaseTypes(test.databaseType, test.databaseType)
			t.Cleanup(func() {
				DB, LOG_DB = originalDB, originalLogDB
				common.SetDatabaseTypes(originalMainType, originalLogType)
			})

			require.NoError(t, db.AutoMigrate(
				&Task{},
				&TaskCreateIdempotency{},
				&Asset{},
				&AssetSource{},
				&AssetBinding{},
				&AssetGroupBinding{},
				&AssetGroupOwnershipClaim{},
				&ChannelAssetCredential{},
			))
			exerciseVideoAssetDialectContracts(t)
		})
	}
}

func exerciseVideoAssetDialectContracts(t *testing.T) {
	t.Helper()
	suffix := fmt.Sprintf("%d", time.Now().UnixNano())
	userID := int(time.Now().UnixNano()%1_000_000) + 10_000
	require.NoError(t, DB.Create(&ChannelAssetCredential{
		ChannelID:       userID,
		AccessKeyID:     "access-" + suffix,
		SecretAccessKey: "secret-" + suffix,
		CreatedTime:     common.GetTimestamp(),
		UpdatedTime:     common.GetTimestamp(),
	}).Error)
	claim, created, err := ClaimTaskCreateIdempotency(
		userID,
		TaskClientProtocolModelArkV3,
		"key-"+suffix,
		"request-"+suffix,
		common.GetTimestamp()+3600,
	)
	require.NoError(t, err)
	require.True(t, created)
	task := &Task{
		TaskID: "task_" + suffix, UserId: userID, ChannelId: 77,
		ClientProtocol: TaskClientProtocolModelArkV3, Status: TaskStatusQueued,
		PrivateData: TaskPrivateData{UpstreamTaskID: "upstream-" + suffix},
	}
	require.NoError(t, RecordTaskCreateUpstreamSuccess(claim.ID, task))
	recovered, err := RecoverTaskCreateIdempotency(claim.ID)
	require.NoError(t, err)
	assert.Equal(t, task.TaskID, recovered.TaskID)

	groups := []AssetGroupBinding{
		{UserID: userID, ScopeKey: "user:" + suffix, ChannelID: 1, CredentialFingerprint: "account-" + suffix, UpstreamProfile: "official_action_assets", ProviderProject: "project", Region: "region", GroupKind: "general_aigc"},
		{UserID: userID + 1, ScopeKey: "user-other:" + suffix, ChannelID: 1, CredentialFingerprint: "account-" + suffix, UpstreamProfile: "official_action_assets", ProviderProject: "project", Region: "region", GroupKind: "general_aigc"},
	}
	for i := range groups {
		require.NoError(t, DB.Create(&groups[i]).Error)
	}
	require.NoError(t, DB.Transaction(func(tx *gorm.DB) error {
		return ClaimAssetGroupOwnership(tx, &groups[0], "group-"+suffix)
	}))
	err = DB.Transaction(func(tx *gorm.DB) error {
		return ClaimAssetGroupOwnership(tx, &groups[1], "group-"+suffix)
	})
	assert.ErrorIs(t, err, ErrAssetGroupOwnershipConflict)
}
