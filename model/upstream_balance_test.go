package model

import (
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestUpstreamBalanceURLGroupsReuseReconciliationIdentityAndNames(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	require.NoError(t, db.AutoMigrate(&ProviderURLGroupDisplayName{}))
	previousDB := DB
	DB = db
	t.Cleanup(func() { DB = previousDB; require.NoError(t, sqlDB.Close()) })
	key := "https://upstream.example/v1"
	alias := ProviderURLGroupDisplayName{URLHash: providerURLGroupHash(key), URLKey: key, Name: "Shared upstream"}
	require.NoError(t, db.Create(&alias).Error)
	urls := []string{"https://UPSTREAM.example:443/v1/", key, "https://upstream.example/v2", "http://upstream.example/v1", "https://upstream.example/?key=secret"}
	channels := make([]*Channel, len(urls))
	for i := range urls {
		channels[i] = &Channel{Id: i + 1, Name: "Fallback channel", BaseURL: &urls[i]}
	}
	groups, err := GetUpstreamBalanceURLGroups(channels)
	require.NoError(t, err)
	assert.Equal(t, UpstreamBalanceURLGroup{URLKey: key, Name: "Shared upstream"}, groups[1])
	assert.Equal(t, groups[1], groups[2])
	assert.Equal(t, UpstreamBalanceURLGroup{URLKey: urls[2], Name: urls[2]}, groups[3])
	assert.NotEqual(t, groups[1].URLKey, groups[4].URLKey)
	assert.Equal(t, UpstreamBalanceURLGroup{URLKey: "channel:5", Name: "Fallback channel"}, groups[5])
	require.NoError(t, db.Model(&alias).Update("name", "Renamed upstream").Error)
	groups, err = GetUpstreamBalanceURLGroups(channels)
	require.NoError(t, err)
	assert.Equal(t, "Renamed upstream", groups[1].Name)
	require.NoError(t, db.Delete(&alias).Error)
	groups, err = GetUpstreamBalanceURLGroups(channels)
	require.NoError(t, err)
	assert.Equal(t, key, groups[1].Name)
}
