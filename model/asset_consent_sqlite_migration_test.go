package model

import (
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestMigrateSQLiteVerificationTokenHash(t *testing.T) {
	legacyDB, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := legacyDB.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)

	require.NoError(t, legacyDB.Exec(`
		CREATE TABLE real_person_verification_sessions (
			id integer PRIMARY KEY,
			authorization_id integer,
			status varchar(32)
		)
	`).Error)

	originalDB := DB
	DB = legacyDB
	t.Cleanup(func() {
		DB = originalDB
		require.NoError(t, sqlDB.Close())
	})

	require.NoError(t, migrateSQLiteVerificationTokenHash())
	require.NoError(t, migrateSQLiteVerificationTokenHash())
	require.True(t, DB.Migrator().HasColumn(&RealPersonVerificationSession{}, "VerificationTokenHash"))
	require.True(t, DB.Migrator().HasIndex(&RealPersonVerificationSession{}, "VerificationTokenHash"))
	require.NoError(t, DB.AutoMigrate(&RealPersonVerificationSession{}))

	tokenHash := "verification-token-hash"
	require.NoError(t, DB.Create(&RealPersonVerificationSession{VerificationTokenHash: &tokenHash}).Error)
	require.Error(t, DB.Create(&RealPersonVerificationSession{VerificationTokenHash: &tokenHash}).Error)
	require.NoError(t, DB.Create(&RealPersonVerificationSession{}).Error)
	require.NoError(t, DB.Create(&RealPersonVerificationSession{}).Error)
}
