package service

import (
	"bytes"
	"context"
	"encoding/csv"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type stubExportStore struct {
	uploaded map[string][]byte
	deleted  []string
}

func (s *stubExportStore) ExportIdentity() string { return "stub" }

func (s *stubExportStore) ExportPutFile(_ context.Context, objectKey string, _ string, path string, _ int64) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	s.uploaded[objectKey] = data
	return nil
}

func (s *stubExportStore) ExportPresignURL(objectKey string, ttl time.Duration, filename ...string) (string, int64, error) {
	return "https://stub-object.local/" + objectKey, time.Now().Add(ttl).Unix(), nil
}

func (s *stubExportStore) ExportDeleteObject(_ context.Context, objectKey string) error {
	s.deleted = append(s.deleted, objectKey)
	delete(s.uploaded, objectKey)
	return nil
}

func setupCustomerExportServiceTest(t *testing.T) *stubExportStore {
	t.Helper()
	t.Setenv("TMPDIR", t.TempDir())
	require.NoError(t, model.DB.AutoMigrate(&model.CustomerExportJob{}, &model.CustomerExportSlot{}))
	for id := int64(1); id <= model.CustomerExportSlotCount; id++ {
		model.DB.Where("id = ?", id).FirstOrCreate(&model.CustomerExportSlot{ID: id, JobID: ""})
	}
	model.DB.Exec("DELETE FROM customer_export_jobs")
	model.DB.Exec("DELETE FROM customer_export_slots")
	for id := int64(1); id <= model.CustomerExportSlotCount; id++ {
		model.DB.Create(&model.CustomerExportSlot{ID: id, JobID: ""})
	}
	for _, user := range []model.User{
		{Id: 42, Username: "export-fixture-42", AffCode: "exp42", Status: common.UserStatusEnabled, Role: common.RoleCommonUser},
		{Id: 43, Username: "export-fixture-43", AffCode: "exp43", Status: common.UserStatusEnabled, Role: common.RoleCommonUser},
	} {
		require.NoError(t, model.DB.Where("id = ?", user.Id).FirstOrCreate(&user).Error)
	}
	store := &stubExportStore{uploaded: map[string][]byte{}}
	customerExportStoreOverride = store
	t.Cleanup(func() { customerExportStoreOverride = nil })
	return store
}

func TestNormalizeCustomerExportFilters(t *testing.T) {
	shanghai := billingExportTimezone()
	monthStart := time.Date(2026, 9, 1, 0, 0, 0, 0, shanghai)
	monthEnd := time.Date(2026, 10, 1, 0, 0, 0, 0, shanghai)

	filters, err := normalizeCustomerExportFilters(model.CustomerExportJobTypeUsageLogs, CustomerExportRequest{
		JobType:        model.CustomerExportJobTypeUsageLogs,
		StartTimestamp: monthStart.Unix(), EndTimestamp: monthStart.Unix() + int64(31*24*time.Hour/time.Second),
	})
	require.NoError(t, err)
	assert.Equal(t, customerExportFieldVersion, filters.FieldVersion)
	assert.Equal(t, "Asia/Shanghai", filters.Timezone)

	_, err = normalizeCustomerExportFilters(model.CustomerExportJobTypeUsageLogs, CustomerExportRequest{
		JobType:        model.CustomerExportJobTypeUsageLogs,
		StartTimestamp: monthStart.Unix(),
		EndTimestamp:   monthStart.Unix() + int64(31*24*time.Hour/time.Second) + 1,
	})
	assert.Error(t, err, "usage log exports cannot exceed 31 days")

	_, err = normalizeCustomerExportFilters(model.CustomerExportJobTypeUsageLogs, CustomerExportRequest{
		JobType: model.CustomerExportJobTypeUsageLogs, LogTypes: []int{99},
		StartTimestamp: monthStart.Unix(), EndTimestamp: monthEnd.Unix(),
	})
	assert.Error(t, err, "unknown log types are rejected")

	filters, err = normalizeCustomerExportFilters(model.CustomerExportJobTypeStatementDetails, CustomerExportRequest{
		JobType:        model.CustomerExportJobTypeStatementDetails,
		StartTimestamp: monthStart.Unix(), EndTimestamp: monthEnd.Unix(),
	})
	require.NoError(t, err)
	assert.Equal(t, monthStart.Unix(), filters.StartTimestamp)
	assert.Equal(t, monthEnd.Unix(), filters.EndTimestamp)

	_, err = normalizeCustomerExportFilters(model.CustomerExportJobTypeStatementSummary, CustomerExportRequest{
		JobType:        model.CustomerExportJobTypeStatementSummary,
		StartTimestamp: monthStart.Unix() + 1, EndTimestamp: monthEnd.Unix(),
	})
	assert.Error(t, err, "billing exports must be natural months")
}

func TestCustomerExportCsvWriterGuardAndShards(t *testing.T) {
	workDir := t.TempDir()
	writer, err := newCustomerExportCsvWriter(workDir, "export", 4<<10)
	require.NoError(t, err)
	scope := customerExportScopeColumns{
		JobID: "cex_test", ExportType: model.CustomerExportJobTypeUsageLogs,
		PeriodStart: 1000, PeriodEnd: 2000, Timezone: "Asia/Shanghai",
		CustomerId: 7, CustomerName: "=cmd|' /c calc!A0",
	}
	ratio := 0.5
	contract := 0.3
	final := 0.15
	for i := 0; i < 40; i++ {
		row := model.CustomerExportRow{
			RequestId: "req", EventType: "consume", RecordTime: 1500,
			TokenId: 3, TokenName: "=1+1", CustomerModel: "模型A", BillingMode: "token",
			CountsTowardBill: true, InputTokens: 10, OutputTokens: 5,
			GroupName: "group,\"quoted\"", GroupRatio: &ratio, ContractApplicable: "yes",
			ContractName: "line\nbreak", ContractRatio: &contract, FinalRatio: &final,
			Quota: 15, OriginalEstimate: "100.0000", PricingRule: "standard_tiered",
			QualityStatus: "estimate",
		}
		require.NoError(t, writer.AppendRow(row, scope))
	}
	files, paths, lines, _, err := writer.Finish()
	require.NoError(t, err)
	assert.EqualValues(t, 40, lines)
	assert.GreaterOrEqual(t, len(files), 2, "small shard cap must roll files at row boundaries")
	assert.Len(t, paths, len(files))
	for _, file := range files {
		assert.NotEmpty(t, file.Sha256)
		assert.EqualValues(t, file.SizeBytes, file.SizeBytes)
	}
	writer.Cleanup()

	// 重新以大分片写一次，检查注入防线与 CSV 转义可被完整解析回原值。
	writer2, err := newCustomerExportCsvWriter(t.TempDir(), "export", customerExportCsvShardBytes)
	require.NoError(t, err)
	defer writer2.Cleanup()
	require.NoError(t, writer2.AppendRow(model.CustomerExportRow{
		EventType: "consume", RecordTime: 1500, TokenId: 3, TokenName: "=1+1",
		CustomerModel: "模型A", BillingMode: "token", GroupName: "group,\"quoted\"",
		ContractName: "line\nbreak", Quota: 15, QualityStatus: "estimate",
	}, scope))
	files2, paths2, lines2, _, err := writer2.Finish()
	require.NoError(t, err)
	require.Len(t, files2, 1)
	require.EqualValues(t, 1, lines2)
	content, err := os.ReadFile(paths2[0])
	require.NoError(t, err)
	assert.True(t, strings.HasPrefix(string(content), customerExportCsvBOM))
	reader := csv.NewReader(strings.NewReader(strings.TrimPrefix(string(content), customerExportCsvBOM)))
	records, err := reader.ReadAll()
	require.NoError(t, err)
	require.Len(t, records, 2)
	dataRow := records[1]
	assert.Equal(t, "'=1+1", dataRow[13], "token name gets the injection guard prefix")
	assert.Equal(t, "group,\"quoted\"", dataRow[23], "embedded quotes and commas round-trip")
	assert.Equal(t, "line\nbreak", dataRow[27], "newline text stays inside one cell")
}

func TestExecuteCustomerExportLogScanEndToEnd(t *testing.T) {
	store := setupCustomerExportServiceTest(t)

	logs := make([]model.Log, 0, 25)
	for i := 0; i < 25; i++ {
		logs = append(logs, model.Log{
			UserId: 42, CreatedAt: int64(9000 + i), Type: model.LogTypeConsume,
			TokenId: 5, TokenName: "key", ModelName: "m-a", Quota: 100, Group: "vip",
			Other: `{"group_ratio":0.5,"contract_discount":"0.3","contract_name":"年度合同","contract_version":2}`,
		})
	}
	require.NoError(t, model.DB.Create(&logs).Error)

	filters := model.CustomerExportFilters{
		FieldVersion: 1, StartTimestamp: 9000, EndTimestamp: 9100, Timezone: "Asia/Shanghai",
	}
	job, created, err := model.CreateCustomerExportJob(42, 42, model.CustomerExportJobTypeUsageLogs, filters)
	require.NoError(t, err)
	require.True(t, created)
	claimed, err := model.ClaimNextQueuedCustomerExportJob("exec-test", time.Now().Add(time.Minute).Unix(), 1800)
	require.NoError(t, err)
	require.NotNil(t, claimed)

	artifact, err := executeCustomerExportLogScan(context.Background(), claimed, filters, t.TempDir(), "exec-test", nil)
	require.NoError(t, err)
	require.NotNil(t, artifact)
	require.Len(t, artifact.Files, 1)
	assert.EqualValues(t, 25, artifact.LineCount)
	assert.Equal(t, "exports/jobs/"+job.JobID+"/"+artifact.Files[0].FileName, artifact.Files[0].ObjectKey)
	require.Len(t, artifact.Files[0].Sha256, 64)

	uploaded, ok := store.uploaded[artifact.Files[0].ObjectKey]
	require.True(t, ok, "artifact must be uploaded to the private namespace")
	assert.True(t, strings.HasPrefix(string(uploaded), customerExportCsvBOM))
	assert.Contains(t, string(uploaded), "年度合同")
}

func TestExecuteCustomerExportLogScanEmptyResult(t *testing.T) {
	setupCustomerExportServiceTest(t)

	filters := model.CustomerExportFilters{
		FieldVersion: 1, StartTimestamp: 9500, EndTimestamp: 9600, Timezone: "Asia/Shanghai",
	}
	job, created, err := model.CreateCustomerExportJob(43, 43, model.CustomerExportJobTypeUsageLogs, filters)
	require.NoError(t, err)
	require.True(t, created)
	claimed, err := model.ClaimNextQueuedCustomerExportJob("exec-test", time.Now().Add(time.Minute).Unix(), 1800)
	require.NoError(t, err)
	require.NotNil(t, claimed)

	artifact, err := executeCustomerExportLogScan(context.Background(), claimed, filters, t.TempDir(), "exec-test", nil)
	require.NoError(t, err)
	require.NotNil(t, artifact)
	assert.Empty(t, artifact.Files, "empty range publishes a zero-file artifact, not a fake bill")
	assert.EqualValues(t, 0, artifact.LineCount)
	assert.Equal(t, model.CustomerExportJobStatusRunning, claimed.Status)
	_ = job
	_ = filepath.Join
}

func (s *stubExportStore) ExportOpenObject(_ context.Context, key string) (io.ReadCloser, error) {
	data, ok := s.uploaded[key]
	if !ok {
		return nil, ErrBillingStatementArtifactUnavailable
	}
	return io.NopCloser(bytes.NewReader(data)), nil
}
func (s *stubExportStore) ExportObjectExists(_ context.Context, key string) (bool, error) {
	_, ok := s.uploaded[key]
	return ok, nil
}

func TestCustomerExportBoundsAndDeduplicatesLogTypes(t *testing.T) {
	request := CustomerExportRequest{JobType: model.CustomerExportJobTypeUsageLogs, StartTimestamp: 1000, EndTimestamp: 2000, LogTypes: []int{model.LogTypeConsume, model.LogTypeRefund, model.LogTypeConsume}}
	filters, err := normalizeCustomerExportFilters(request.JobType, request)
	require.NoError(t, err)
	assert.Equal(t, []int{model.LogTypeConsume, model.LogTypeRefund}, filters.LogTypes)
	request.LogTypes = make([]int, 8)
	_, err = normalizeCustomerExportFilters(request.JobType, request)
	require.Error(t, err)
	assert.Equal(t, "too many log type filters", err.Error())
}
