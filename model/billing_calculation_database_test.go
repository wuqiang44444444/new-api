package model

import (
	"os"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/pkg/billingexpr"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func TestBillingCalculationDatabaseRoundTrip(t *testing.T) {
	for _, tc := range []struct {
		name, env string
		open      func(string) gorm.Dialector
	}{
		{"sqlite", "", func(string) gorm.Dialector { return sqlite.Open(":memory:") }},
		{"mysql", "TEST_CALCULATION_MYSQL_DSN", mysql.Open},
		{"postgres", "TEST_CALCULATION_POSTGRES_DSN", postgres.Open},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dsn := os.Getenv(tc.env)
			if tc.env != "" && dsn == "" {
				t.Skip("database DSN is not configured")
			}
			db, err := gorm.Open(tc.open(dsn), &gorm.Config{})
			require.NoError(t, err)
			sql, err := db.DB()
			require.NoError(t, err)
			t.Cleanup(func() { _ = sql.Close() })
			require.NoError(t, db.AutoMigrate(&BatchJob{}, &BatchJobLine{}))
			job := BatchJob{Id: "calculation-roundtrip", FrozenSnapshot: BillingText(strings.Repeat("x", 100000))}
			// >64 KiB protects the MySQL TEXT regression, not an arbitrary stress load.
			require.NoError(t, db.Create(&job).Error)
			t.Cleanup(func() {
				db.Where("id = ?", job.Id).Delete(&BatchJob{})
				db.Where("job_id = ?", job.Id).Delete(&BatchJobLine{})
			})
			calculation := billingexpr.NewCalculation()
			calculation.Add("multiply", "quota", 80, 20, 4)
			raw, err := common.Marshal(calculation.Finish(80))
			require.NoError(t, err)
			line := BatchJobLine{JobId: job.Id, CustomId: "a", FinalQuota: 80, CalculationVersion: 1, Calculation: BillingText(raw)}
			require.NoError(t, db.Create(&line).Error)
			var stored BatchJob
			require.NoError(t, db.First(&stored, "id = ?", job.Id).Error)
			assert.Equal(t, job.FrozenSnapshot, stored.FrozenSnapshot)
			var saved BatchJobLine
			require.NoError(t, db.First(&saved, line.Id).Error)
			assert.Equal(t, line.Calculation, saved.Calculation)
			assert.Equal(t, 80, saved.FinalQuota)
			// Rerunning schema initialization keeps the evidence unchanged.
			require.NoError(t, db.AutoMigrate(&BatchJob{}, &BatchJobLine{}))
			require.NoError(t, db.First(&saved, line.Id).Error)
			assert.Equal(t, line.Calculation, saved.Calculation)
		})
	}
}
