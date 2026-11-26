package service

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
	"testing/synctest"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func captureDailyReportLogs(t *testing.T) *bytes.Buffer {
	t.Helper()
	var logs bytes.Buffer
	common.LogWriterMu.Lock()
	previous := gin.DefaultWriter
	gin.DefaultWriter = &logs
	common.LogWriterMu.Unlock()
	t.Cleanup(func() {
		common.LogWriterMu.Lock()
		gin.DefaultWriter = previous
		common.LogWriterMu.Unlock()
	})
	return &logs
}

func TestDailyReportYieldLogsPressureDiagnostics(t *testing.T) {
	for _, tc := range []struct {
		name     string
		samples  []model.DailyReportDatabaseLoad
		err      error
		expected []string
	}{
		{
			name: "both database probes reach the pause threshold",
			samples: []model.DailyReportDatabaseLoad{
				{Probe: 100 * time.Millisecond}, {Probe: 110 * time.Millisecond},
			},
			expected: []string{
				"source=main_db trigger=db_probe_latency sample=100ms threshold=100ms",
				"source=log_db trigger=db_probe_latency sample=110ms threshold=100ms",
			},
		},
		{
			name: "main database failure omits raw error text",
			err: &model.DailyReportProbeError{
				Source: model.DailyReportMainDatabaseSource, Elapsed: 12 * time.Millisecond,
				Timeout: 200 * time.Millisecond, Cause: errors.New("private driver diagnostic"),
			},
			expected: []string{"source=main_db trigger=db_sample_error sample=error elapsed=12ms timeout=200ms threshold=successful_sample"},
		},
		{
			name: "log database timeout reports the elapsed duration and deadline",
			err: &model.DailyReportProbeError{
				Source: model.DailyReportLogDatabaseSource, Elapsed: 201 * time.Millisecond,
				Timeout: 200 * time.Millisecond, Cause: context.DeadlineExceeded,
			},
			expected: []string{"source=log_db trigger=db_sample_error sample=timeout elapsed=201ms timeout=200ms threshold=successful_sample"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db := setupVersionServiceTestDB(t)
			require.NoError(t, db.AutoMigrate(&model.SystemInstance{}))
			logs := captureDailyReportLogs(t)

			budget := dailyReportBudget{admission: func(ctx context.Context) reportAdmissionDecision {
				return checkReportAdmission(ctx, func(context.Context) ([]model.DailyReportDatabaseLoad, error) {
					return tc.samples, tc.err
				})
			}}
			require.ErrorIs(t, budget.validationGate(t.Context()), model.ErrDailyReportResourcePressure)
			require.ErrorIs(t, budget.validationGate(t.Context()), model.ErrDailyReportResourcePressure)
			logged := logs.String()
			assert.Equal(t, 1, strings.Count(logged, "daily report yielded to resource pressure"))
			for _, expected := range tc.expected {
				assert.Contains(t, logged, expected)
			}
			assert.NotContains(t, logged, "private driver diagnostic")
		})
	}
}

func TestDailyReportProbeFailureLogsWithoutYieldTransition(t *testing.T) {
	for _, phase := range []string{"parent deadline", "already paused"} {
		t.Run(phase, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				db := setupVersionServiceTestDB(t)
				require.NoError(t, db.AutoMigrate(&model.SystemInstance{}))
				logs := captureDailyReportLogs(t)
				ctx, cancel := context.WithTimeout(t.Context(), 200*time.Millisecond)
				defer cancel()
				budget := dailyReportBudget{
					pressure: phase == "already paused",
					admission: func(ctx context.Context) reportAdmissionDecision {
						return checkReportAdmission(ctx, func(ctx context.Context) ([]model.DailyReportDatabaseLoad, error) {
							if phase == "parent deadline" {
								<-ctx.Done()
							}
							return nil, &model.DailyReportProbeError{
								Source: model.DailyReportLogDatabaseSource, Elapsed: 200 * time.Millisecond,
								Timeout: 200 * time.Millisecond, Cause: context.DeadlineExceeded,
							}
						})
					},
				}

				expected := model.ErrDailyReportResourcePressure
				if phase == "parent deadline" {
					expected = context.DeadlineExceeded
				}
				require.ErrorIs(t, budget.validationGate(ctx), expected)
				if phase == "already paused" {
					require.ErrorIs(t, budget.validationGate(ctx), expected)
				}
				line := "report database probe failed | source=log_db trigger=db_sample_error sample=timeout elapsed=200ms timeout=200ms threshold=successful_sample"
				assert.Equal(t, 1, strings.Count(logs.String(), line))
				assert.NotContains(t, logs.String(), "daily report yielded to resource pressure")
			})
		})
	}
}
