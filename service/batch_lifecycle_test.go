package service

import (
	"bytes"
	"context"
	"errors"
	azurebatch "github.com/QuantumNous/new-api/relay/channel/azurebatch"
	"gorm.io/gorm"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/config"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type batchMemoryStore struct {
	disabledArtifactStore
	objects map[string][]byte
}

func (*batchMemoryStore) Enabled() bool { return true }
func (s *batchMemoryStore) BatchPutObject(_ context.Context, key, _ string, r io.Reader) (int64, error) {
	data, err := io.ReadAll(r)
	s.objects[key] = data
	return int64(len(data)), err
}
func (s *batchMemoryStore) BatchGetObject(_ context.Context, key string, w io.Writer) (int64, error) {
	return io.Copy(w, bytes.NewReader(s.objects[key]))
}
func (s *batchMemoryStore) HeadObject(_ context.Context, key string) (bool, error) {
	_, ok := s.objects[key]
	return ok, nil
}

type batchRoundTrip func(*http.Request) (*http.Response, error)

func (f batchRoundTrip) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func batchLifecycleFixture(t *testing.T, createStatus int, createBody string) (*gin.Context, *dto.BatchCreateRequest, *int) {
	t.Helper()
	truncate(t)
	require.NoError(t, model.DB.AutoMigrate(&model.BatchFile{}, &model.BatchJob{}, &model.BatchJobLine{}))
	for _, table := range []string{"batch_files", "batch_jobs", "batch_job_lines"} {
		require.NoError(t, model.DB.Exec("DELETE FROM "+table).Error)
	}
	oldStore := GetTaskArtifactStore()
	taskArtifactStoreRuntime.swap(&batchMemoryStore{objects: map[string][]byte{}}, "")
	t.Cleanup(func() { taskArtifactStoreRuntime.swap(oldStore, "") })
	oldTransport := http.DefaultTransport
	calls := 0
	http.DefaultTransport = batchRoundTrip(func(r *http.Request) (*http.Response, error) {
		assert.Equal(t, "frozen.invalid", r.URL.Host)
		assert.Equal(t, "test-credential", r.Header.Get("api-key"))
		assert.Equal(t, "test-version", r.URL.Query().Get("api-version"))
		status, body := 200, `{"id":"upload-1"}`
		switch {
		case r.Method == "POST" && r.URL.Path == "/openai/batches":
			calls++
			status, body = createStatus, createBody
		case r.Method == "GET" && r.URL.Path == "/openai/batches/provider-1":
			body = `{"id":"provider-1","status":"completed","output_file_id":"out-1","error_file_id":"err-1","request_counts":{"total":2,"completed":1,"failed":1}}`
		case strings.Contains(r.URL.Path, "out-1/content"):
			body = `{"custom_id":"a","response":{"status_code":200,"body":{"usage":{"prompt_tokens":10,"completion_tokens":5,"total_tokens":15}}}}` + "\n"
		case strings.Contains(r.URL.Path, "err-1/content"):
			body = `{"custom_id":"b","error":{"code":"invalid_request"}}` + "\n"
		}
		return &http.Response{StatusCode: status, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body))}, nil
	})
	t.Cleanup(func() { http.DefaultTransport = oldTransport })
	seedUser(t, 1701, 100000)
	seedToken(t, 1701, 1701, "batch-key", 100000)
	channel := model.Channel{Id: 1701, Type: constant.ChannelTypeAzureBatch, Status: common.ChannelStatusEnabled, Models: "batch-text", Group: "default", Key: "test-credential", Other: "test-version"}
	base := "https://frozen.invalid"
	channel.BaseURL = &base
	require.NoError(t, model.DB.Create(&channel).Error)
	require.NoError(t, config.GlobalConfig.LoadFromDB(map[string]string{"batch_billing_setting.batch_billing_expr": `{"batch-text":"p * 2 + c * 4"}`}))
	t.Cleanup(func() {
		_ = config.GlobalConfig.LoadFromDB(map[string]string{"batch_billing_setting.batch_billing_expr": "{}"})
	})
	input := `{"custom_id":"a","method":"POST","url":"/v1/chat/completions","body":{"model":"batch-text","messages":[{"role":"user","content":"hello"}],"max_tokens":100}}` + "\n" + `{"custom_id":"b","method":"POST","url":"/v1/chat/completions","body":{"model":"batch-text","messages":[{"role":"user","content":"world"}],"max_tokens":100}}` + "\n"
	upload, err := UploadBatchFile(context.Background(), 1701, 1701, "input.jsonl", "batch", strings.NewReader(input))
	require.NoError(t, err)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("POST", "/v1/batches", nil)
	c.Set("id", 1701)
	c.Set("token_id", 1701)
	common.SetContextKey(c, constant.ContextKeyUsingGroup, "default")
	common.SetContextKey(c, constant.ContextKeyUserGroup, "default")
	return c, &dto.BatchCreateRequest{InputFileId: upload.File.Id, Endpoint: dto.BatchEndpointChatCompletions, CompletionWindow: "24h"}, &calls
}

func TestBatchCreateProgressSettlesAndDeliversBothFilesOnce(t *testing.T) {
	oldExport := common.DataExportEnabled
	common.DataExportEnabled = true
	t.Cleanup(func() { common.DataExportEnabled = oldExport; model.DB.Exec("DELETE FROM quota_data") })
	c, request, calls := batchLifecycleFixture(t, 200, `{"id":"provider-1","status":"validating"}`)
	result, err := CreateBatchJob(c, request)
	require.NoError(t, err)
	require.NotEmpty(t, result.Job.Id)
	task, err := model.GetTaskById(result.Job.TaskRowId)
	require.NoError(t, err)
	assert.Equal(t, task.TaskID, result.Job.Id)
	require.NotNil(t, task.PrivateData.AsyncBilling)
	// Editing/deleting current credentials cannot change an accepted lifecycle.
	require.NoError(t, model.DB.Delete(&model.Channel{}, 1701).Error)
	require.NoError(t, progressBatchJob(context.Background(), result.Job))
	job, err := model.GetDueBatchJobById(result.Job.Id)
	require.NoError(t, err)
	assert.True(t, model.BatchJobFullyDone(job))
	assert.NotEmpty(t, job.OutputFileId)
	assert.NotEmpty(t, job.ErrorFileId)
	assert.EqualValues(t, 1, job.CountCompleted)
	assert.EqualValues(t, 1, job.CountFailed)
	assert.EqualValues(t, 10, job.UsageInput)
	assert.EqualValues(t, 5, job.UsageOutput)
	assert.EqualValues(t, 15, job.UsageTotal)
	var consume model.Log
	require.NoError(t, model.DB.Where("request_id = ?", job.Id).First(&consume).Error)
	assert.Equal(t, 10, consume.PromptTokens)
	assert.Equal(t, 5, consume.CompletionTokens)

	task, err = model.GetTaskById(job.TaskRowId)
	require.NoError(t, err)
	assert.EqualValues(t, model.TaskStatusSuccess, task.Status)
	// 10 * 2 + 5 * 4 at 500k quota/USD = 20 quota.
	assert.Equal(t, 20, task.Quota)
	var user model.User
	require.NoError(t, model.DB.First(&user, 1701).Error)
	assert.Equal(t, 99980, user.Quota)
	assert.Equal(t, 20, user.UsedQuota)
	assert.Equal(t, 2, user.RequestCount)
	assert.False(t, model.UsesTaskBillingDelivery(task), "Batch has its own durable completion owner")
	var data struct{ Count, Quota, Tokens int }
	require.NoError(t, model.DB.Model(&model.QuotaData{}).Select("SUM(count) AS count, SUM(quota) AS quota, SUM(token_used) AS tokens").Where("user_id = ?", 1701).Scan(&data).Error)
	assert.Equal(t, 2, data.Count)
	assert.Equal(t, 20, data.Quota)
	assert.Equal(t, 15, data.Tokens)
	statement, err := model.GetBillingCustomerStatement(context.Background(), 1701, 1, common.GetTimestamp()+10, "api_key", 0, "", "")
	require.NoError(t, err)
	assert.EqualValues(t, 2, statement.Summary.Requests)
	assert.EqualValues(t, 20, statement.Summary.NetQuota)

	require.NoError(t, progressBatchJob(context.Background(), job))
	var count int64
	require.NoError(t, model.DB.Model(&model.Log{}).Where("request_id = ?", job.Id).Count(&count).Error)
	assert.EqualValues(t, 1, count)
	require.NoError(t, model.DB.Model(&model.BatchFile{}).Count(&count).Error)
	assert.EqualValues(t, 3, count)
	assert.Equal(t, 1, *calls)
}

func TestBatchCreateUncertainAcceptanceKeepsHold(t *testing.T) {
	for _, tc := range []struct {
		name   string
		status int
		body   string
	}{{"server-error", 503, `{}`}, {"missing-id", 200, `{"status":"validating"}`}} {
		t.Run(tc.name, func(t *testing.T) {
			c, request, calls := batchLifecycleFixture(t, tc.status, tc.body)
			_, err := CreateBatchJob(c, request)
			require.Error(t, err)
			var attempt model.TaskCreateAttempt
			require.NoError(t, model.DB.First(&attempt).Error)
			assert.Equal(t, model.TaskCreateAttemptUnknown, attempt.Status)
			assert.Equal(t, model.TaskCreateAttemptBillingHeld, attempt.BillingHoldState)
			assert.NotEmpty(t, attempt.RecoverySnapshot)
			var count int64
			require.NoError(t, model.DB.Model(&model.Task{}).Count(&count).Error)
			assert.Zero(t, count)
			assert.Equal(t, 1, *calls)
		})
	}
}

func TestBatchModelLimitDenialCreatesNoAttempt(t *testing.T) {
	c, request, calls := batchLifecycleFixture(t, 200, `{"id":"provider-1","status":"validating"}`)
	c.Set("token_model_limit_enabled", true)
	c.Set("token_model_limit", map[string]bool{"other": true})
	_, err := CreateBatchJob(c, request)
	require.ErrorIs(t, err, ErrBatchJobModelDenied)
	var count int64
	require.NoError(t, model.DB.Model(&model.TaskCreateAttempt{}).Count(&count).Error)
	assert.Zero(t, count)
	assert.Zero(t, *calls)
}

func TestBatchAcceptedCommitFailureRecoversJobAndTaskAtomically(t *testing.T) {
	c, request, calls := batchLifecycleFixture(t, 200, `{"id":"provider-1","status":"validating"}`)
	callback := "batch_recovery_failure"
	require.NoError(t, model.DB.Callback().Create().Before("gorm:create").Register(callback, func(tx *gorm.DB) {
		if tx.Statement.Table == "batch_jobs" {
			tx.AddError(errors.New("injected persistence failure"))
		}
	}))
	t.Cleanup(func() { _ = model.DB.Callback().Create().Remove(callback) })
	_, err := CreateBatchJob(c, request)
	require.Error(t, err)
	var count int64
	require.NoError(t, model.DB.Model(&model.Task{}).Count(&count).Error)
	assert.Zero(t, count)
	var attempt model.TaskCreateAttempt
	require.NoError(t, model.DB.First(&attempt).Error)
	assert.Equal(t, model.TaskCreateAttemptUpstreamSucceeded, attempt.Status)
	assert.Equal(t, model.TaskCreateAttemptBillingHeld, attempt.BillingHoldState)
	require.NoError(t, model.DB.Callback().Create().Remove(callback))
	recovered, err := model.RecoverTaskCreateAttempt(attempt.ID)
	require.NoError(t, err)
	job, err := model.GetBatchJobByTaskRowId(recovered.ID)
	require.NoError(t, err)
	assert.Equal(t, attempt.PublicTaskID, job.Id)
	assert.Equal(t, "provider-1", job.UpstreamBatchId)
	again, err := model.RecoverTaskCreateAttempt(attempt.ID)
	require.NoError(t, err)
	assert.Equal(t, recovered.ID, again.ID)
	assert.Equal(t, 1, *calls)
}

func TestBatchCancelledCountsAccountForUnexecutedLines(t *testing.T) {
	frozen := &model.BatchFrozenSnapshot{LineInputs: map[string]int{"a": 10, "b": 10}}
	job := &model.BatchJob{LineCount: 2}
	status := &azurebatch.BatchStatus{CountsPresent: true, Status: "cancelled", CountTotal: 2, CountCompleted: 1, CountCancelled: 1}
	require.NoError(t, validateBatchResultCompleteness(job, status, frozen, []azurebatch.LineResult{{CustomId: "a", Status: "completed"}}))
	status.CountCancelled = 0
	require.NoError(t, validateBatchResultCompleteness(job, status, frozen, []azurebatch.LineResult{{CustomId: "a", Status: "completed"}}))
	status.CountCancelled = 1
	require.Error(t, validateBatchResultCompleteness(job, status, frozen, nil))
}

func TestBatchPollClaimRejectsStaleObservation(t *testing.T) {
	c, request, _ := batchLifecycleFixture(t, 200, `{"id":"provider-1","status":"validating"}`)
	result, err := CreateBatchJob(c, request)
	require.NoError(t, err)
	job := result.Job
	claimed, err := model.ClaimBatchJobPoll(job.Id, 0, 0, 0, 60)
	require.NoError(t, err)
	require.True(t, claimed)
	job.PublicStatus = "failed"
	require.Error(t, model.SaveBatchJobObservation(job, 0))
	require.NoError(t, model.SaveBatchJobObservation(job, 1))
	claimed, err = model.ClaimBatchJobPoll(job.Id, 60, 0, 1, 120)
	require.NoError(t, err)
	require.True(t, claimed)
	require.Error(t, model.SaveBatchJobObservation(job, 1))
}

func TestBatchResourcesRequireTheOwningApp(t *testing.T) {
	c, request, calls := batchLifecycleFixture(t, 200, `{"id":"provider-1","status":"validating"}`)
	_, err := model.GetBatchFileOwned(request.InputFileId, 1701, 1702)
	require.ErrorIs(t, err, model.ErrBatchFileNotFound)
	c.Set("token_id", 1702)
	_, err = CreateBatchJob(c, request)
	require.Error(t, err)
	assert.Zero(t, *calls)
}

func TestBatchContractSelectsAllowedChannelAndAppliesDiscount(t *testing.T) {
	c, request, _ := batchLifecycleFixture(t, 200, `{"id":"provider-1","status":"validating"}`)
	require.NoError(t, model.DB.Model(&model.User{}).Where("id = ?", 1701).Update("auth_version", 1).Error)
	// A contract-selected Batch channel replaces the lowest native channel,
	// while keeping typed selection and its model discount.
	var allowedChannel model.Channel
	require.NoError(t, model.DB.First(&allowedChannel, 1701).Error)
	allowedChannel.Id = 1702
	require.NoError(t, model.DB.Create(&allowedChannel).Error)
	contract := model.CustomerContract{UserId: 1701, Name: "Batch contract", Enabled: true, Version: 1}
	require.NoError(t, model.DB.Create(&contract).Error)
	require.NoError(t, model.DB.Create(&model.CustomerContractEntityRule{ContractId: contract.Id, PublicModel: "batch-text", ChannelId: 1702, RouteGroup: "default", RatioUnits: 50000000}).Error)
	ResetContractEntityCacheForTest()
	t.Cleanup(ResetContractEntityCacheForTest)
	t.Cleanup(func() {
		model.DB.Delete(&model.CustomerContractEntityRule{}, "contract_id = ?", contract.Id)
		model.DB.Delete(&contract)
	})
	common.SetContextKey(c, constant.ContextKeyTokenContractId, contract.Id)
	common.SetContextKey(c, constant.ContextKeyAuthVersion, int64(1))
	fact, err := ResolveContractEntityRule(1701, 1, contract.Id, "batch-text")
	require.NoError(t, err)
	require.NotNil(t, fact)
	assert.EqualValues(t, 50000000, fact.RatioUnits)
	result, err := CreateBatchJob(c, request)
	require.NoError(t, err)
	assert.Equal(t, 1702, result.Job.ChannelId, "batch selection stays inside the contract")
	require.NoError(t, progressBatchJob(context.Background(), result.Job))
	task, err := model.GetTaskById(result.Job.TaskRowId)
	require.NoError(t, err)
	assert.Equal(t, 10, task.Quota, "the contract discount still applies to the batch job")
	_, err = LoadContractEntityForRequest(1702, 1, contract.Id)
	require.ErrorIs(t, err, ErrCustomerContractUnavailable)
}

func TestBatchFileCursorPagesAreStableAndOwned(t *testing.T) {
	_, request, _ := batchLifecycleFixture(t, 200, `{"id":"provider-1","status":"validating"}`)
	require.NoError(t, model.DB.Create(&model.BatchFile{Id: "file-z", UserId: 1701, AppID: 1701, CreatedAt: common.GetTimestamp() + 1}).Error)
	page, err := model.ListBatchFilesOwned(1701, 1701, 1, "")
	require.NoError(t, err)
	require.Len(t, page.Items, 1)
	assert.True(t, page.HasMore)
	assert.Equal(t, "file-z", page.Items[0].Id)
	page, err = model.ListBatchFilesOwned(1701, 1701, 1, page.Items[0].Id)
	require.NoError(t, err)
	require.Len(t, page.Items, 1)
	assert.False(t, page.HasMore)
	assert.Equal(t, request.InputFileId, page.Items[0].Id)
	_, err = model.ListBatchFilesOwned(1701, 1702, 1, request.InputFileId)
	require.ErrorIs(t, err, model.ErrBatchFileNotFound)
}

func TestBatchCancelledAndExpiredJobsSettleExecutedRowsWithoutSyntheticErrors(t *testing.T) {
	for _, status := range []string{"cancelled", "expired"} {
		t.Run(status, func(t *testing.T) {
			c, request, _ := batchLifecycleFixture(t, 200, `{"id":"provider-1","status":"validating"}`)
			result, err := CreateBatchJob(c, request)
			require.NoError(t, err)
			previous := http.DefaultTransport
			http.DefaultTransport = batchRoundTrip(func(r *http.Request) (*http.Response, error) {
				if r.URL.Path != "/openai/batches/provider-1" {
					return previous.RoundTrip(r)
				}
				body := `{"id":"provider-1","status":"` + status + `","output_file_id":"out-1","request_counts":{"total":2,"completed":1,"failed":0}}`
				return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body))}, nil
			})
			require.NoError(t, progressBatchJob(context.Background(), result.Job))
			job, err := model.GetDueBatchJobById(result.Job.Id)
			require.NoError(t, err)
			assert.True(t, model.BatchJobFullyDone(job))
			assert.Equal(t, status, job.PublicStatus)
			task, err := model.GetTaskById(job.TaskRowId)
			require.NoError(t, err)
			assert.Equal(t, 20, task.Quota)
			assert.Empty(t, job.ErrorFileId)
		})
	}
}

func TestBatchSaturationIsAuditedInTheSettledLog(t *testing.T) {
	c, request, _ := batchLifecycleFixture(t, 200, `{"id":"provider-1","status":"validating"}`)
	result, err := CreateBatchJob(c, request)
	require.NoError(t, err)
	remaining := common.MaxQuota - result.Job.EstimateQuota
	require.NoError(t, model.DB.Model(&model.User{}).Where("id = ?", 1701).Update("quota", remaining).Error)
	require.NoError(t, model.DB.Model(&model.Token{}).Where("id = ?", 1701).Update("remain_quota", remaining).Error)
	previous := http.DefaultTransport
	http.DefaultTransport = batchRoundTrip(func(r *http.Request) (*http.Response, error) {
		if !strings.Contains(r.URL.Path, "out-1/content") {
			return previous.RoundTrip(r)
		}
		body := `{"custom_id":"a","response":{"status_code":200,"body":{"usage":{"prompt_tokens":9000000000,"completion_tokens":0,"total_tokens":9000000000}}}}` + "\n"
		return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body))}, nil
	})
	require.NoError(t, progressBatchJob(context.Background(), result.Job))
	var log model.Log
	require.NoError(t, model.LOG_DB.Where("request_id = ?", result.Job.Id).First(&log).Error)
	assert.Equal(t, common.MaxQuota, log.Quota)
	assert.Equal(t, common.MaxQuota, log.PromptTokens)
	assert.Contains(t, log.Other, `"token_saturation"`)
	job, err := model.GetDueBatchJobById(result.Job.Id)
	require.NoError(t, err)
	assert.EqualValues(t, 9000000000, job.UsageInput)
	assert.Contains(t, log.Other, `"quota_saturation"`)
	assert.Contains(t, log.Other, `"admin_info"`)
}

func TestBatchReadyEvidenceRecoversSettlementWithoutProviderOrObjects(t *testing.T) {
	c, request, _ := batchLifecycleFixture(t, 200, `{"id":"provider-1","status":"validating"}`)
	result, err := CreateBatchJob(c, request)
	require.NoError(t, err)
	callback := "batch_log_crash"
	require.NoError(t, model.DB.Callback().Create().Before("gorm:create").Register(callback, func(tx *gorm.DB) {
		if tx.Statement.Table == "logs" {
			tx.AddError(errors.New("simulated crash after line facts and funding"))
		}
	}))
	t.Cleanup(func() { _ = model.DB.Callback().Create().Remove(callback) })
	require.Error(t, progressBatchJob(context.Background(), result.Job))
	require.NoError(t, model.DB.Callback().Create().Remove(callback))
	job, err := model.GetDueBatchJobById(result.Job.Id)
	require.NoError(t, err)
	require.Equal(t, model.BatchDeliveryReady, job.DeliveryState)
	require.NotEqual(t, model.BatchSettleSettled, job.SettleState)
	// Both external dependencies disappear after the complete evidence commit.
	taskArtifactStoreRuntime.swap(&batchMemoryStore{objects: map[string][]byte{}}, "")
	http.DefaultTransport = batchRoundTrip(func(*http.Request) (*http.Response, error) {
		t.Error("settlement recovery must not contact Azure")
		return nil, errors.New("provider unavailable")
	})
	require.NoError(t, progressBatchJob(context.Background(), job))
	job, err = model.GetDueBatchJobById(job.Id)
	require.NoError(t, err)
	assert.True(t, model.BatchJobFullyDone(job))
	var user model.User
	require.NoError(t, model.DB.First(&user, 1701).Error)
	assert.Equal(t, 99980, user.Quota)
	assert.Equal(t, 20, user.UsedQuota)
	var count int64
	require.NoError(t, model.DB.Model(&model.Log{}).Where("request_id = ?", job.Id).Count(&count).Error)
	assert.EqualValues(t, 1, count)
}
