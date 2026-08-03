package model

import (
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestMigrateAssetApplicationScopeBackfillsOnlyDerivableIdentity(t *testing.T) {
	previousDB := DB
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&RealPersonAuthorization{}, &Asset{}, &TaskCreateAttempt{}))
	DB = db
	t.Cleanup(func() { DB = previousDB })

	authorization := RealPersonAuthorization{
		UserID: 7, CreatedByTokenID: 11, EndUserSubjectHash: "subject-hash",
		Status: RealPersonAuthorizationAuthorized,
	}
	require.NoError(t, db.Create(&authorization).Error)
	legacyAuthorization := RealPersonAuthorization{
		UserID: 7, CreatedByTokenID: 14, Status: RealPersonAuthorizationAwaitingVerification,
	}
	require.NoError(t, db.Create(&legacyAuthorization).Error)
	realPersonAsset := Asset{
		UserID: 7, AssetKind: AssetKindRealPerson, MediaType: "image", AuthorizationID: &authorization.ID,
	}
	require.NoError(t, db.Create(&realPersonAsset).Error)
	generalAsset := Asset{UserID: 7, CreatedByTokenID: 12, AssetKind: AssetKindGeneral, MediaType: "image"}
	require.NoError(t, db.Create(&generalAsset).Error)
	attempt := TaskCreateAttempt{
		AttemptID: "attempt-scope-migration", PublicTaskID: "task-scope-migration", UserID: 7, TokenID: 13,
		ClientProtocol: TaskClientProtocolModelArkV3, RequestHash: "hash",
	}
	require.NoError(t, db.Create(&attempt).Error)

	require.NoError(t, migrateAssetApplicationScope())
	require.NoError(t, db.First(&authorization, authorization.ID).Error)
	require.NoError(t, db.First(&legacyAuthorization, legacyAuthorization.ID).Error)
	require.NoError(t, db.First(&realPersonAsset, realPersonAsset.ID).Error)
	require.NoError(t, db.First(&generalAsset, generalAsset.ID).Error)
	require.NoError(t, db.First(&attempt, attempt.ID).Error)
	assert.Equal(t, 11, authorization.AppID)
	assert.Zero(t, legacyAuthorization.AppID)
	assert.Equal(t, 11, realPersonAsset.AppID)
	assert.Equal(t, "subject-hash", realPersonAsset.EndUserSubjectHash)
	assert.Equal(t, 12, generalAsset.AppID)
	assert.Equal(t, 13, attempt.AppID)
}
