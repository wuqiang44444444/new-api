package model

import (
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestMigrateTaskApplicationScopeUsesFrozenAppOrToken(t *testing.T) {
	previousDB := DB
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&Task{}))
	DB = db
	t.Cleanup(func() { DB = previousDB })

	tasks := []Task{
		{TaskID: "task-app", UserId: 1, ClientProtocol: TaskClientProtocolModelArkV3, PrivateData: TaskPrivateData{AppID: 101, TokenId: 201}},
		{TaskID: "task-token", UserId: 1, ClientProtocol: TaskClientProtocolModelArkV3, PrivateData: TaskPrivateData{TokenId: 202}},
		{TaskID: "task-unscoped", UserId: 1, ClientProtocol: TaskClientProtocolModelArkV3},
	}
	require.NoError(t, db.Session(&gorm.Session{SkipHooks: true}).Create(&tasks).Error)
	require.NoError(t, migrateTaskApplicationScope())

	var persisted []Task
	require.NoError(t, db.Order("id asc").Find(&persisted).Error)
	require.Len(t, persisted, 3)
	assert.Equal(t, 101, persisted[0].AppID)
	assert.Equal(t, 202, persisted[1].AppID)
	assert.Zero(t, persisted[2].AppID)
}
