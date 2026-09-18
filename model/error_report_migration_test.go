package model

import (
	"fmt"
	"github.com/QuantumNous/new-api/common"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"testing"
)

func TestErrorReportPeriodMigrationPreservesKnownProgress(t *testing.T) {
	for _, enabled := range []bool{true, false} {
		t.Run(fmt.Sprint(enabled), func(t *testing.T) {
			oldDB, oldType := DB, common.MainDatabaseType()
			db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
			require.NoError(t, err)
			conn, err := db.DB()
			require.NoError(t, err)
			conn.SetMaxOpenConns(1)
			DB = db
			common.SetMainDatabaseType(common.DatabaseTypeSQLite)
			t.Cleanup(func() { DB = oldDB; common.SetMainDatabaseType(oldType); conn.Close() })
			require.NoError(t, db.AutoMigrate(&Option{}, &ErrorReportSchedule{}))
			require.NoError(t, db.Create(&Option{Key: "error_report_setting.enabled", Value: fmt.Sprint(enabled)}).Error)
			require.NoError(t, db.Create(&ErrorReportSchedule{Scope: ErrorReportScheduleScopeGlobal, Enabled: true, BaseWindowStart: 3600, NextWindowStart: 7200}).Error)
			require.NoError(t, migrateErrorReportPeriods())
			schedule, err := GetErrorReportSchedule()
			require.NoError(t, err)
			var periods []errorReportPeriod
			require.NoError(t, common.UnmarshalJsonStr(schedule.Periods, &periods))
			assert.Equal(t, enabled, schedule.Enabled)
			assert.Equal(t, int64(7200), schedule.NextWindowStart)
			if enabled {
				assert.Equal(t, []errorReportPeriod{{Start: 7200}}, periods)
			} else {
				assert.Empty(t, periods)
			}
			require.NoError(t, migrateErrorReportPeriods())
			after, err := GetErrorReportSchedule()
			require.NoError(t, err)
			assert.Equal(t, schedule, after)
		})
	}
}
