package main

import (
	"bytes"
	"path/filepath"
	"testing"

	"github.com/QuantumNous/new-api/model"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestInventoryIncludesUnknownFundsWithoutTaskOrChannel(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "inventory.db")), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.Channel{}, &model.TaskCreateAttempt{}))
	require.NoError(t, db.Create(&model.TaskCreateAttempt{AttemptID: "unknown", ClientProtocol: model.TaskClientProtocolModelArkV3, Status: model.TaskCreateAttemptUnknown, BillingHoldState: model.TaskCreateAttemptBillingHeld}).Error)
	var out bytes.Buffer
	require.NoError(t, printAttemptBillingInventory(db, &out))
	assert.Contains(t, out.String(), "unknown | held | 1")
	assert.Contains(t, out.String(), "unresolved_create_attempts: 1")
}
