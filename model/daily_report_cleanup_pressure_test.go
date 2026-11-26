package model

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDailyReportCleanupYieldsBeforeGenerationWrites(t *testing.T) {
	for _, kind := range []string{"usage", "billing"} {
		for _, retire := range []bool{true, false} {
			phase := "delete"
			if retire {
				phase = "retire"
			}
			t.Run(kind+"/"+phase, func(t *testing.T) {
				db, start, _ := setupBillingReportTest(t)
				require.NoError(t, db.AutoMigrate(&CustomerExportJob{}, &SystemInstance{}))
				sqlDB, err := db.DB()
				require.NoError(t, err)
				sqlDB.SetMaxOpenConns(1)

				const generation = "cleanup-pressure"
				now := time.Now().Unix()
				old := now - 9*billingReportDaySeconds
				if kind == "usage" {
					marker := usageReportDayRow{Generation: generation, UserID: -1, Payload: "{}", CreatedAt: old, RetiredAt: old}
					if !retire {
						marker.UnavailableAt = old
					}
					require.NoError(t, db.Create(&marker).Error)
				} else {
					require.NoError(t, db.Create(&billingReportGeneration{
						ID: generation, Start: start, CreatedAt: old, RetiredAt: old, Published: retire,
					}).Error)
				}
				require.NoError(t, db.Create(&dailyReportExportReference{Generation: generation, JobID: "expired"}).Error)

				ctx := t.Context()
				sample, err := DailyReportDatabaseSampler(ctx)
				require.NoError(t, err)
				guard := func(ctx context.Context) error {
					samples, err := sample(ctx)
					require.NoError(t, err)
					require.NotEmpty(t, samples)
					_, err = ReportForegroundInstances(ctx, now)
					require.NoError(t, err)
					return ErrDailyReportResourcePressure
				}
				pace := func(ctx context.Context, _ int) error {
					if retire {
						return nil
					}
					return guard(ctx)
				}
				limited, cancel := context.WithTimeout(WithDailyReportWorkerBudget(ctx, guard, pace), time.Second)
				defer cancel()

				prune := PruneUsageReportRows
				if kind == "billing" {
					prune = PruneBillingReportGenerations
				}
				pruned, err := prune(limited, now)
				require.ErrorIs(t, err, ErrDailyReportResourcePressure)
				assert.False(t, pruned)
				var refs int64
				require.NoError(t, db.Model(&dailyReportExportReference{}).Where("generation = ?", generation).Count(&refs).Error)
				assert.EqualValues(t, 1, refs)
				if kind == "usage" {
					var marker usageReportDayRow
					require.NoError(t, db.Where("generation = ? AND user_id = -1", generation).Take(&marker).Error)
					if retire {
						assert.Zero(t, marker.UnavailableAt)
					} else {
						assert.Equal(t, old, marker.UnavailableAt)
					}
				} else {
					var row billingReportGeneration
					require.NoError(t, db.Where("id = ?", generation).Take(&row).Error)
					assert.Equal(t, retire, row.Published)
					assert.Equal(t, old, row.RetiredAt)
				}
				pruned, err = prune(ctx, now)
				require.NoError(t, err)
				assert.Equal(t, !retire, pruned)
			})
		}
	}
}
