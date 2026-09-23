package model

import (
	"fmt"
	"strconv"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// auditParam decodes one round-tripped audit param to its JSON text so
// assertions compare logical values instead of storage representations.
func auditParam(t *testing.T, params AuditFields, key string) string {
	t.Helper()
	encoded, err := common.Marshal(params[key])
	require.NoError(t, err)
	return string(encoded)
}

// One audit row per committed channel-level transition; repeat requests,
// commit failures and per-key multi-key changes never claim a transition.
func TestChannelStatusTransitionsAreAudited(t *testing.T) {
	previousDB := DB
	previousLogDB := LOG_DB
	t.Cleanup(func() { DB = previousDB; LOG_DB = previousLogDB })

	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "_"))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&Channel{}, &Ability{}, &User{}))
	DB = db
	LOG_DB = db
	require.NoError(t, LOG_DB.AutoMigrate(&AuditLog{}))

	channel := Channel{Name: "audited", Group: "default", Models: "model-a", Key: "test-key", Status: common.ChannelStatusEnabled}
	require.NoError(t, db.Create(&channel).Error)
	admin := User{Username: "admin", Role: common.RoleAdminUser, Status: 1}
	require.NoError(t, db.Create(&admin).Error)

	auditRows := func() []AuditLog {
		var rows []AuditLog
		require.NoError(t, LOG_DB.Where("action = ?", "channel_status_change").Order("id ASC").Find(&rows).Error)
		return rows
	}

	// Auto disable (request error or channel test path).
	require.True(t, UpdateChannelStatus(channel.Id, "", common.ChannelStatusAutoDisabled, "upstream 503 exceeded threshold"))
	rows := auditRows()
	require.Len(t, rows, 1)
	assert.Equal(t, AuditCategoryOperation, rows[0].Category)
	assert.Equal(t, `"auto"`, auditParam(t, rows[0].Other.Op.Params, "source"))
	assert.Equal(t, `"enabled"`, auditParam(t, rows[0].Other.Op.Params, "before"))
	assert.Equal(t, `"auto_disabled"`, auditParam(t, rows[0].Other.Op.Params, "after"))
	assert.Equal(t, `"automatic_status_change"`, auditParam(t, rows[0].Other.Op.Params, "reason"))
	assert.Equal(t, `0`, auditParam(t, rows[0].Other.Op.Params, "actor_id"))

	// Repeat request while already disabled: no change, no row.
	require.False(t, UpdateChannelStatus(channel.Id, "", common.ChannelStatusAutoDisabled, "repeat"))
	assert.Len(t, auditRows(), 1)

	// Manual recovery with an actor.
	changed, err := UpdateChannelStatusWithActor(channel.Id, "", common.ChannelStatusEnabled, "manual operation", admin.Id)
	require.NoError(t, err)
	require.True(t, changed)
	rows = auditRows()
	require.Len(t, rows, 2)
	assert.Equal(t, `"manual"`, auditParam(t, rows[1].Other.Op.Params, "source"))
	assert.Equal(t, `"auto_disabled"`, auditParam(t, rows[1].Other.Op.Params, "before"))
	assert.Equal(t, `"enabled"`, auditParam(t, rows[1].Other.Op.Params, "after"))
	assert.Equal(t, strconv.Itoa(admin.Id), auditParam(t, rows[1].Other.Op.Params, "actor_id"))
	assert.Equal(t, common.RoleAdminUser, rows[1].ActorRole)

	// Batch manual disable writes one row per actually changed channel.
	other := Channel{Name: "audited-2", Group: "default", Models: "model-a", Key: "test-key", Status: common.ChannelStatusEnabled}
	require.NoError(t, db.Create(&other).Error)
	changedCount, err := UpdateChannelStatusesWithActor([]int{channel.Id, other.Id}, common.ChannelStatusManuallyDisabled, "manual batch operation", admin.Id)
	require.NoError(t, err)
	require.Equal(t, 2, changedCount)
	rows = auditRows()
	require.Len(t, rows, 4)
	assert.Equal(t, `"manual_batch"`, auditParam(t, rows[2].Other.Op.Params, "source"))
	assert.Equal(t, `"manual_batch"`, auditParam(t, rows[3].Other.Op.Params, "source"))

	// Per-key multi-key change keeps the channel status and records nothing.
	multiKey := Channel{Name: "multi", Group: "default", Models: "model-a", Key: "key-a\nkey-b", Status: common.ChannelStatusEnabled}
	multiKey.ChannelInfo.IsMultiKey = true
	require.NoError(t, db.Create(&multiKey).Error)
	require.True(t, UpdateChannelStatus(multiKey.Id, "key-a", common.ChannelStatusAutoDisabled, "key failure"))
	assert.Len(t, auditRows(), 4)
	var stillEnabled Channel
	require.NoError(t, db.First(&stillEnabled, multiKey.Id).Error)
	assert.Equal(t, common.ChannelStatusEnabled, stillEnabled.Status)
	assert.Equal(t, common.ChannelStatusAutoDisabled, stillEnabled.ChannelInfo.MultiKeyStatusList[0])
	changed, err = UpdateChannelStatusWithActor(multiKey.Id, "", common.ChannelStatusEnabled, "repeat", admin.Id)
	require.NoError(t, err)
	assert.False(t, changed)
	assert.Len(t, auditRows(), 4)
	changedCount, err = UpdateChannelStatusesWithActor([]int{multiKey.Id, multiKey.Id}, common.ChannelStatusManuallyDisabled, "batch", admin.Id)
	require.NoError(t, err)
	assert.Equal(t, 1, changedCount)
	assert.Len(t, auditRows(), 5)
}

// Tag enable/disable is audited per channel with the manual_tag source.
func TestChannelStatusTagChangeIsAudited(t *testing.T) {
	previousDB := DB
	previousLogDB := LOG_DB
	t.Cleanup(func() { DB = previousDB; LOG_DB = previousLogDB })

	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "_"))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&Channel{}, &Ability{}, &User{}))
	DB = db
	LOG_DB = db
	require.NoError(t, LOG_DB.AutoMigrate(&AuditLog{}))

	tag := "pool-a"
	tagged := Channel{Name: "tagged", Tag: &tag, Group: "default", Models: "model-a", Key: "test-key", Status: common.ChannelStatusEnabled}
	require.NoError(t, db.Create(&tagged).Error)
	admin := User{Username: "admin-tag", Role: common.RoleRootUser, Status: 1}
	require.NoError(t, db.Create(&admin).Error)

	require.NoError(t, DisableChannelByTagWithActor(tag, admin.Id))
	var rows []AuditLog
	require.NoError(t, LOG_DB.Where("action = ?", "channel_status_change").Find(&rows).Error)
	require.Len(t, rows, 1)
	assert.Equal(t, `"manual_tag"`, auditParam(t, rows[0].Other.Op.Params, "source"))
	assert.Equal(t, `"enabled"`, auditParam(t, rows[0].Other.Op.Params, "before"))
	assert.Equal(t, `"manually_disabled"`, auditParam(t, rows[0].Other.Op.Params, "after"))
	assert.Equal(t, strconv.Itoa(admin.Id), auditParam(t, rows[0].Other.Op.Params, "actor_id"))
}
