package model

import (
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestReadWriteIndexSortRepairKeepsIndexOnDDLFailure(t *testing.T) {
	db := setupBillingStatementVersionTestDB(t)
	require.NoError(t, db.Migrator().DropIndex(&Log{}, "idx_created_at_id"))
	require.NoError(t, db.Exec("CREATE INDEX idx_created_at_id ON logs(id,created_at)").Error)
	logs := []Log{{CreatedAt: 200, Quota: 70}, {CreatedAt: 100, Quota: 80}}
	require.NoError(t, db.Create(&logs).Error)
	injected := errors.New("interrupted sort index replacement")
	require.NoError(t, db.Callback().Raw().Before("gorm:raw").Register("test:interrupt_sort_index", func(tx *gorm.DB) {
		if strings.HasPrefix(tx.Statement.SQL.String(), "CREATE INDEX") && strings.Contains(tx.Statement.SQL.String(), "idx_created_at_id") {
			tx.AddError(injected)
		}
	}))
	err := migrateLogReadWriteIndexes(db)
	require.NoError(t, db.Callback().Raw().Remove("test:interrupt_sort_index"))
	require.ErrorIs(t, err, injected)
	repair, err := inspectManagedIndex(db, &Log{}, "idx_logs_sort_repair")
	require.NoError(t, err)
	require.NotNil(t, repair, "replacement remains available even across implicit DDL commits")
	assert.Equal(t, []string{"created_at", "id"}, repair.columns)
	var saved []Log
	require.NoError(t, db.Order("created_at DESC, id DESC").Find(&saved).Error)
	assert.Equal(t, logs, saved)
	require.NoError(t, migrateLogReadWriteIndexes(db))
	assert.False(t, db.Migrator().HasIndex(&Log{}, "idx_logs_sort_repair"))
}

func TestReadWriteIndexUpgradePreservesLogsAndPolicies(t *testing.T) {
	db := setupBillingStatementVersionTestDB(t)
	require.NoError(t, db.AutoMigrate(&CasbinRule{}, &ErrorEvent{}))
	// Reproduce the deployed schema, including the historical sort-order drift.
	require.NoError(t, db.Migrator().DropIndex(&Log{}, "idx_created_at_id"))
	for _, sql := range []string{
		"CREATE INDEX idx_created_at_id ON logs(id, created_at)",
		"CREATE INDEX idx_logs_user_id ON logs(user_id)",
		"CREATE INDEX idx_logs_model_name ON logs(model_name)",
		"CREATE INDEX idx_error_events_created_at ON error_events(created_at)",
		"CREATE INDEX idx_casbin_rule ON casbin_rule(ptype,v0,v1,v2,v3,v4,v5)",
	} {
		require.NoError(t, db.Exec(sql).Error)
	}
	logs := []Log{
		{CreatedAt: 300, UserId: 7, ModelName: "model-a", Username: "a", Type: LogTypeConsume, Quota: 50, Other: "{}"},
		{CreatedAt: 100, UserId: 7, ModelName: "model-b", Username: "a", Type: LogTypeRefund, Quota: 20, Other: "{}"},
		{CreatedAt: 300, UserId: 8, ModelName: "model-a", Username: "b", Type: LogTypeConsume, Quota: 70, Other: "{}"},
		{CreatedAt: 200, UserId: 7, ModelName: "model-a", Username: "a", Type: LogTypeConsume, Quota: 90, Other: "{}"},
	}
	require.NoError(t, db.Create(&logs).Error)
	rules := []CasbinRule{
		{Ptype: "p", V0: "role:admin", V1: "tasks", V2: "read", V3: "allow"},
		{Ptype: "p", V0: "user:7", V1: "tasks", V2: "read", V3: "deny"},
		{Ptype: "g", V0: "user:7", V1: "role:admin"},
	}
	require.NoError(t, db.Create(&rules).Error)
	events := []ErrorEvent{
		{CreatedAt: 300, UserId: 7, Status: 400, EventType: "api_error", Detail: "{}"},
		{CreatedAt: 100, UserId: 8, Status: 500, EventType: "api_error", Detail: "{}"},
		{CreatedAt: 300, UserId: 7, Status: 500, EventType: "api_error", Detail: "{}"},
	}
	require.NoError(t, db.Create(&events).Error)
	before, totalBefore, err := GetAllLogs(LogTypeUnknown, 100, 300, "model-a", "", "", 0, 1, 2, 0, "", "", "")
	require.NoError(t, err)
	assert.EqualValues(t, 3, totalBefore)
	require.Len(t, before, 2)
	assert.Equal(t, []int{1, 4}, []int{before[0].Id, before[1].Id})
	eventsBefore, eventTotal, err := GetErrorEvents(ErrorEventFilter{StartTimestamp: 100, EndTimestamp: 300, UserId: 7}, 0, 2)
	require.NoError(t, err)

	require.NoError(t, migrateBillingStatementLogIndex(db))
	require.NoError(t, migrateCasbinRuleIndex(db))
	require.NoError(t, MigrateErrorEvents())
	after, totalAfter, err := GetAllLogs(LogTypeUnknown, 100, 300, "model-a", "", "", 0, 1, 2, 0, "", "", "")
	require.NoError(t, err)
	assert.Equal(t, before, after)
	assert.Equal(t, totalBefore, totalAfter)
	eventsAfter, eventTotalAfter, err := GetErrorEvents(ErrorEventFilter{StartTimestamp: 100, EndTimestamp: 300, UserId: 7}, 0, 2)
	require.NoError(t, err)
	assert.Equal(t, eventsBefore, eventsAfter)
	assert.Equal(t, eventTotal, eventTotalAfter)
	var savedLogs []Log
	var savedRules []CasbinRule
	var savedEvents []ErrorEvent
	require.NoError(t, db.Order("id").Find(&savedLogs).Error)
	require.NoError(t, db.Order("id").Find(&savedRules).Error)
	require.NoError(t, db.Order("id").Find(&savedEvents).Error)
	assert.Equal(t, logs, savedLogs, "no original billing facts rewritten")
	assert.Equal(t, rules, savedRules, "allow/deny/group policies retain IDs and order")
	assert.Equal(t, events, savedEvents)
	duplicate := rules[0]
	duplicate.Id = 0
	require.Error(t, db.Create(&duplicate).Error, "unique policy constraint is preserved")
	require.NoError(t, db.Create(&Log{UserId: 7, ModelName: "model-a", CreatedAt: 300}).Error, "log identities remain non-unique")
	var userIDs []int
	require.NoError(t, db.Model(&Log{}).Where("user_id = ?", 7).Order("id").Pluck("id", &userIDs).Error)
	assert.Equal(t, []int{1, 2, 4, 5}, userIDs)

	index, err := inspectManagedIndex(db, &Log{}, "idx_created_at_id")
	require.NoError(t, err)
	require.NotNil(t, index)
	assert.Equal(t, []string{"created_at", "id"}, index.columns)
	for _, pair := range []struct {
		value any
		name  string
	}{
		{&Log{}, "idx_logs_user_id"}, {&Log{}, "idx_logs_model_name"}, {&Log{}, "idx_logs_sort_repair"},
		{&CasbinRule{}, "idx_casbin_rule"}, {&ErrorEvent{}, "idx_error_events_created_at"},
	} {
		assert.False(t, db.Migrator().HasIndex(pair.value, pair.name), pair.name)
	}
	if db.Dialector.Name() == "sqlite" {
		var plan []struct{ Detail string }
		require.NoError(t, db.Raw("EXPLAIN QUERY PLAN SELECT * FROM logs WHERE created_at >= ? AND created_at <= ? ORDER BY created_at DESC, id DESC LIMIT 2", 100, 300).Scan(&plan).Error)
		for _, step := range plan {
			assert.NotContains(t, step.Detail, "TEMP B-TREE", "time-ordered paging must use the repaired index")
		}
	}
	// AutoMigrate must not recreate retired indexes on every restart.
	recorder := &migrationSQLRecorder{}
	restarted := db.Session(&gorm.Session{Logger: recorder})
	require.NoError(t, restarted.AutoMigrate(&Log{}, &CasbinRule{}, &ErrorEvent{}))
	require.NoError(t, migrateBillingStatementLogIndex(restarted))
	require.NoError(t, migrateCasbinRuleIndex(restarted))
	assert.Empty(t, recorder.schemaMutations())
}

func TestReadWriteIndexRepairResumesAfterInterruption(t *testing.T) {
	for _, state := range []string{"old_and_repair", "repair_only", "new_and_repair"} {
		t.Run(state, func(t *testing.T) {
			db := setupBillingStatementVersionTestDB(t)
			require.NoError(t, db.Exec("CREATE INDEX idx_logs_sort_repair ON logs(created_at,id)").Error)
			if state != "new_and_repair" {
				require.NoError(t, db.Migrator().DropIndex(&Log{}, "idx_created_at_id"))
			}
			if state == "old_and_repair" {
				require.NoError(t, db.Exec("CREATE INDEX idx_created_at_id ON logs(id,created_at)").Error)
			}
			require.NoError(t, migrateLogReadWriteIndexes(db))
			require.NoError(t, migrateLogReadWriteIndexes(db))
			index, err := inspectManagedIndex(db, &Log{}, "idx_created_at_id")
			require.NoError(t, err)
			require.NotNil(t, index)
			assert.Equal(t, []string{"created_at", "id"}, index.columns)
			assert.False(t, db.Migrator().HasIndex(&Log{}, "idx_logs_sort_repair"))
		})
	}
}

func TestReadWriteIndexRefusesUnexpectedConstraints(t *testing.T) {
	for _, scenario := range []string{"unique_old", "wrong_replacement", "missing_replacement", "unique_sort"} {
		t.Run(scenario, func(t *testing.T) {
			db := setupBillingStatementVersionTestDB(t)
			switch scenario {
			case "unique_old":
				require.NoError(t, db.Exec("CREATE UNIQUE INDEX idx_logs_user_id ON logs(user_id)").Error)
			case "wrong_replacement", "missing_replacement":
				require.NoError(t, db.Exec("CREATE INDEX idx_logs_user_id ON logs(user_id)").Error)
				require.NoError(t, db.Migrator().DropIndex(&Log{}, "idx_user_id_id"))
				if scenario == "wrong_replacement" {
					require.NoError(t, db.Exec("CREATE INDEX idx_user_id_id ON logs(id,user_id)").Error)
				}
			case "unique_sort":
				require.NoError(t, db.Migrator().DropIndex(&Log{}, "idx_created_at_id"))
				require.NoError(t, db.Exec("CREATE UNIQUE INDEX idx_created_at_id ON logs(id,created_at)").Error)
			}
			require.Error(t, migrateLogReadWriteIndexes(db))
			if scenario == "unique_sort" {
				index, err := inspectManagedIndex(db, &Log{}, "idx_created_at_id")
				require.NoError(t, err)
				require.NotNil(t, index)
				assert.True(t, index.unique)
			} else {
				assert.True(t, db.Migrator().HasIndex(&Log{}, "idx_logs_user_id"))
			}
		})
	}
}

func TestReadWriteIndexTaskReplacementPreservesLookupSemantics(t *testing.T) {
	db := setupBillingStatementVersionTestDB(t)
	require.NoError(t, db.AutoMigrate(&Task{}))
	require.NoError(t, db.Exec("CREATE INDEX idx_tasks_task_id ON tasks(task_id)").Error)
	require.NoError(t, db.Exec("CREATE INDEX idx_tasks_user_app_task ON tasks(user_id,app_id,task_id)").Error)
	// Index order differs from row ID order; the legacy first-row lookup must
	// still select the same row, while capability reads must reject ambiguity.
	tasks := []Task{
		{TaskID: "shared", UserId: 8, AppID: 12, Quota: 90},
		{TaskID: "shared", UserId: 7, AppID: 11, Quota: 80},
		{TaskID: "shared", UserId: 7, AppID: 12, Quota: 70},
		{TaskID: "unique", UserId: 7, AppID: 11, Quota: 60},
	}
	require.NoError(t, db.Create(&tasks).Error)
	var before []Task
	require.NoError(t, db.Order("id").Find(&before).Error)
	require.NoError(t, migrateBillingStatementTaskIndex(db))
	require.NoError(t, migrateBillingStatementTaskIndex(db))
	var after []Task
	require.NoError(t, db.Order("id").Find(&after).Error)
	assert.Equal(t, before, after)
	legacy, found, err := GetByOnlyTaskId("shared")
	require.NoError(t, err)
	require.True(t, found)
	assert.Equal(t, tasks[0].ID, legacy.ID)
	owned, found, err := GetByTaskId(7, "shared")
	require.NoError(t, err)
	require.True(t, found)
	assert.Equal(t, tasks[1].ID, owned.ID)
	scoped, found, err := GetByTaskIDForApp(7, 12, "shared")
	require.NoError(t, err)
	require.True(t, found)
	assert.Equal(t, tasks[2].ID, scoped.ID)
	_, found, err = GetByTaskIDForApp(8, 11, "shared")
	require.NoError(t, err)
	assert.False(t, found)
	ambiguous, found, err := GetUniqueByOnlyTaskId("shared")
	require.NoError(t, err)
	assert.False(t, found)
	assert.Nil(t, ambiguous)
	unique, found, err := GetUniqueByOnlyTaskId("unique")
	require.NoError(t, err)
	require.True(t, found)
	assert.Equal(t, tasks[3].ID, unique.ID)
	duplicate := tasks[1]
	duplicate.ID = 0
	require.NoError(t, db.Create(&duplicate).Error, "no new uniqueness rule")
	assert.False(t, db.Migrator().HasIndex(&Task{}, "idx_tasks_task_id"))
	assert.False(t, db.Migrator().HasIndex(&Task{}, "idx_tasks_user_app_task"))
	index, err := inspectManagedIndex(db, &Task{}, "idx_tasks_task_user_app")
	require.NoError(t, err)
	require.NotNil(t, index)
	assert.Equal(t, []string{"task_id", "user_id", "app_id"}, index.columns)
	recorder := &migrationSQLRecorder{}
	restarted := db.Session(&gorm.Session{Logger: recorder})
	require.NoError(t, restarted.AutoMigrate(&Task{}))
	require.NoError(t, migrateBillingStatementTaskIndex(restarted))
	assert.Empty(t, recorder.schemaMutations())
}
