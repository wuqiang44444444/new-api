package model

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/clienterrlog"
	"github.com/QuantumNous/new-api/common"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupErrorEventTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	previousDB, previousLogDB := DB, LOG_DB
	previousMainDatabaseType, previousLogDatabaseType := common.MainDatabaseType(), common.LogDatabaseType()
	common.SetDatabaseTypes(common.DatabaseTypeSQLite, common.DatabaseTypeSQLite)
	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "_"))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	DB, LOG_DB = db, db
	require.NoError(t, db.AutoMigrate(&Channel{}))
	require.NoError(t, MigrateErrorEvents())
	sqlDB, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() {
		DB, LOG_DB = previousDB, previousLogDB
		common.SetDatabaseTypes(previousMainDatabaseType, previousLogDatabaseType)
		_ = sqlDB.Close()
	})
	return db
}

func errorEventFixture(base time.Time) clienterrlog.Event {
	return clienterrlog.Event{
		At: base, RequestID: "req-a", Module: "relay", Method: "POST",
		Route: "/v1/chat/completions", Status: 400, UserID: 11,
		Username: "ua", Model: "m1", ChannelID: 1, Reason: "new_api_error",
	}
}

// persistErrorEvent 写入结构化行；Detail 编码为 JSON 文本，可用 request_id 查回。
func TestPersistErrorEventWritesStructuredRow(t *testing.T) {
	setupErrorEventTestDB(t)
	now := time.Now()
	event := errorEventFixture(now)
	event.RequestID = "req-evt-1"
	event.TokenName = "key-prod-1"
	event.Stage = "relay"
	event.PublicCode = "invalid_request"
	event.Detail = map[string]string{"source_status": "400"}
	require.NoError(t, persistErrorEvent(event))

	events, total, err := GetErrorEvents(ErrorEventFilter{RequestId: "req-evt-1"}, 0, 10)
	require.NoError(t, err)
	require.Len(t, events, 1)
	assert.Equal(t, int64(1), total)
	row := events[0]
	assert.Equal(t, "relay", row.Module)
	assert.Equal(t, 400, row.Status)
	assert.Equal(t, 11, row.UserId)
	assert.Equal(t, "ua", row.Username)
	assert.Equal(t, "key-prod-1", row.TokenName)
	assert.Equal(t, "m1", row.ModelName)
	assert.Equal(t, 1, row.ChannelId)
	assert.Equal(t, "relay", row.Stage)
	assert.Equal(t, "new_api_error", row.Reason)
	assert.Equal(t, "invalid_request", row.PublicCode)
	assert.Equal(t, now.Unix(), row.CreatedAt)
	var detail map[string]string
	require.NoError(t, common.Unmarshal([]byte(row.Detail), &detail))
	assert.Equal(t, map[string]string{"source_status": "400"}, detail)
}

// 过滤与排序：各维度 exact 过滤，created_at DESC, id DESC 排序，Offset/Limit 分页。
func TestGetErrorEventsFiltersOrderAndPaging(t *testing.T) {
	setupErrorEventTestDB(t)
	base := time.Now().Add(-time.Hour)
	fixtures := []func(e clienterrlog.Event) clienterrlog.Event{
		func(e clienterrlog.Event) clienterrlog.Event { e.RequestID = "req-a"; e.At = base; return e },
		func(e clienterrlog.Event) clienterrlog.Event {
			e.RequestID = "req-b"
			e.At = base.Add(time.Minute)
			e.Module = "asset"
			e.Status = 403
			e.ChannelID = 2
			e.Model = "m2"
			e.Reason = "unclassified"
			return e
		},
		func(e clienterrlog.Event) clienterrlog.Event {
			e.RequestID = "req-c"
			e.At = base.Add(2 * time.Minute)
			e.Status = 502
			return e
		},
		func(e clienterrlog.Event) clienterrlog.Event {
			e.RequestID = "req-d"
			e.At = base.Add(3 * time.Minute)
			e.Status = 500
			e.Username = "uc"
			e.ChannelID = 3
			e.Model = "m3"
			e.Reason = "timeout"
			return e
		},
	}
	for _, mutate := range fixtures {
		event := mutate(errorEventFixture(base))
		event.UserID = 11
		require.NoError(t, persistErrorEvent(event))
	}

	events, total, err := GetErrorEvents(ErrorEventFilter{Module: "relay"}, 0, 10)
	require.NoError(t, err)
	assert.Equal(t, int64(3), total)
	require.Len(t, events, 3)
	assert.Equal(t, "req-d", events[0].RequestId)
	assert.Equal(t, "req-a", events[2].RequestId)

	status502 := 502
	events, total, err = GetErrorEvents(ErrorEventFilter{Status: &status502}, 0, 10)
	require.NoError(t, err)
	assert.Equal(t, int64(1), total)
	require.Len(t, events, 1)
	assert.Equal(t, "req-c", events[0].RequestId)

	events, total, err = GetErrorEvents(ErrorEventFilter{Username: "ua"}, 0, 10)
	require.NoError(t, err)
	assert.Equal(t, int64(3), total)

	events, total, err = GetErrorEvents(ErrorEventFilter{Username: "uc"}, 0, 10)
	require.NoError(t, err)
	assert.Equal(t, int64(1), total)
	assert.Equal(t, "req-d", events[0].RequestId)

	events, total, err = GetErrorEvents(ErrorEventFilter{ChannelId: 3}, 0, 10)
	require.NoError(t, err)
	assert.Equal(t, int64(1), total)
	assert.Equal(t, "req-d", events[0].RequestId)

	events, total, err = GetErrorEvents(ErrorEventFilter{RequestId: "req-b"}, 0, 10)
	require.NoError(t, err)
	assert.Equal(t, int64(1), total)
	assert.Equal(t, "req-b", events[0].RequestId)

	events, total, err = GetErrorEvents(ErrorEventFilter{Reason: "timeout"}, 0, 10)
	require.NoError(t, err)
	assert.Equal(t, int64(1), total)
	assert.Equal(t, "req-d", events[0].RequestId)

	events, total, err = GetErrorEvents(ErrorEventFilter{}, 1, 2)
	require.NoError(t, err)
	assert.Equal(t, int64(4), total)
	require.Len(t, events, 2)
	assert.Equal(t, "req-c", events[0].RequestId)
	assert.Equal(t, "req-b", events[1].RequestId)

	events, total, err = GetErrorEvents(ErrorEventFilter{
		StartTimestamp: base.Add(90 * time.Second).Unix(),
		EndTimestamp:   base.Add(150 * time.Second).Unix(),
	}, 0, 10)
	require.NoError(t, err)
	assert.Equal(t, int64(1), total)
	assert.Equal(t, "req-c", events[0].RequestId)
}

// 事件类型、任务 ID 与可空状态过滤：nil=不筛选；0=精确筛选「无 HTTP 状态」事件。
func TestGetErrorEventsTypeTaskAndStatusFilters(t *testing.T) {
	setupErrorEventTestDB(t)
	rows := []clienterrlog.Event{
		{At: time.Now(), RequestID: "req-api", Module: "relay", Status: 502},
		{At: time.Now(), RequestID: "req-stream", Module: "relay", Status: 200},
		{At: time.Now(), RequestID: "req-task", Module: "relay", Status: 0},
	}
	eventTypes := []string{"api_error", "stream_error", "task_failure"}
	taskIDs := []string{"", "", "task_abc"}
	for i := range rows {
		rows[i].EventType = eventTypes[i]
		rows[i].TaskID = taskIDs[i]
		require.NoError(t, persistErrorEvent(rows[i]))
	}

	status200 := 200
	events, total, err := GetErrorEvents(ErrorEventFilter{Status: &status200}, 0, 10)
	require.NoError(t, err)
	assert.Equal(t, int64(1), total)
	assert.Equal(t, "req-stream", events[0].RequestId)

	status0 := 0
	events, total, err = GetErrorEvents(ErrorEventFilter{Status: &status0}, 0, 10)
	require.NoError(t, err)
	assert.Equal(t, int64(1), total)
	assert.Equal(t, "req-task", events[0].RequestId)

	events, total, err = GetErrorEvents(ErrorEventFilter{EventType: "stream_error"}, 0, 10)
	require.NoError(t, err)
	assert.Equal(t, int64(1), total)
	assert.Equal(t, "req-stream", events[0].RequestId)

	events, total, err = GetErrorEvents(ErrorEventFilter{TaskId: "task_abc"}, 0, 10)
	require.NoError(t, err)
	assert.Equal(t, int64(1), total)
	assert.Equal(t, "req-task", events[0].RequestId)

	events, total, err = GetErrorEvents(ErrorEventFilter{}, 0, 10)
	require.NoError(t, err)
	assert.Equal(t, int64(3), total)
}

// 迁移升级：旧表存量行（event_type 为空）在迁移后按 api_error 归类。
func TestMigrateErrorEventsBackfillsEmptyEventType(t *testing.T) {
	db := setupErrorEventTestDB(t)
	require.NoError(t, persistErrorEvent(clienterrlog.Event{At: time.Now(), RequestID: "req-legacy", Module: "relay", Status: 400}))
	require.NoError(t, db.Model(&ErrorEvent{}).Where("1 = 1").Update("event_type", "").Error)
	require.NoError(t, MigrateErrorEvents())
	var count int64
	require.NoError(t, db.Model(&ErrorEvent{}).Where("event_type = ?", clienterrlog.EventAPIError).Count(&count).Error)
	assert.Equal(t, int64(1), count)
}

// LOG_DB 不可用时持久化与查询必须显式报错，不得静默成功。
func TestErrorEventFailsWhenLogDatabaseUnavailable(t *testing.T) {
	previousLogDB := LOG_DB
	LOG_DB = nil
	t.Cleanup(func() { LOG_DB = previousLogDB })

	err := persistErrorEvent(clienterrlog.Event{RequestID: "req-x", Module: "relay", Status: 500})
	require.Error(t, err)

	_, _, err = GetErrorEvents(ErrorEventFilter{}, 0, 10)
	require.Error(t, err)
}
