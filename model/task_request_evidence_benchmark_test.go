package model

import (
	"fmt"
	"path/filepath"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"gorm.io/gorm/logger"
)

// Same fixed inventory and Count+Find work for both query shapes. The old
// query exists only here as the measured reference, never as a runtime fallback.
func BenchmarkEvidenceTaskLookup(b *testing.B) {
	db, err := gorm.Open(sqlite.Open(filepath.Join(b.TempDir(), "lookup.db")), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(b, err)
	require.NoError(b, db.AutoMigrate(&TaskRequestEvidence{}, &EvidenceTaskLink{}))
	previousDB, previousType := DB, common.MainDatabaseType()
	DB = db
	common.SetMainDatabaseType(common.DatabaseTypeSQLite)
	b.Cleanup(func() { DB = previousDB; common.SetMainDatabaseType(previousType) })
	indexes := make([]TaskRequestEvidence, 20000)
	for i := range indexes {
		indexes[i] = TaskRequestEvidence{RequestID: fmt.Sprintf("request-%d", i), TaskID: fmt.Sprintf("task-%d", i), CreatedAt: 1700000000}
	}
	indexes[0].TaskID = "requested-task"
	require.NoError(b, db.CreateInBatches(&indexes, 500).Error)
	require.NoError(b, db.Create(&EvidenceTaskLink{EvidenceID: indexes[len(indexes)-1].Id, TaskID: "requested-task", UserID: 7, AppID: 3}).Error)
	oldQuery := func() *gorm.DB {
		links := DB.Model(&EvidenceTaskLink{}).Select("1").Where("evidence_id = ?", clause.Column{Table: "task_request_evidences", Name: "id"}).Where("task_id = ?", "requested-task")
		return DB.Model(&TaskRequestEvidence{}).Where("task_id = ? OR EXISTS (?)", "requested-task", links)
	}
	var plans []struct{ Detail string }
	statement := oldQuery().Session(&gorm.Session{DryRun: true}).Find(&[]TaskRequestEvidence{}).Statement
	require.NoError(b, db.Raw("EXPLAIN QUERY PLAN "+statement.SQL.String(), statement.Vars...).Scan(&plans).Error)
	b.Logf("reference plan: %+v", plans)
	b.Run("correlated_exists_reference", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			var count int64
			require.NoError(b, oldQuery().Count(&count).Error)
			var rows []TaskRequestEvidence
			require.NoError(b, oldQuery().Order("id desc").Limit(20).Find(&rows).Error)
			require.EqualValues(b, 2, count)
			require.Len(b, rows, 2)
		}
	})
	b.Run("production", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			rows, count, err := QueryTaskRequestEvidence(TaskRequestEvidenceQueryParams{TaskID: "requested-task"})
			require.NoError(b, err)
			require.EqualValues(b, 2, count)
			require.Len(b, rows, 2)
		}
	})
}
