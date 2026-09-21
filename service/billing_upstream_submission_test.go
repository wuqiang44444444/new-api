package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestUpstreamSubmissionFreezesInheritedDiscountWithoutWriting(t *testing.T) {
	truncate(t)
	setupCustomerExportServiceTest(t)
	require.NoError(t, model.DB.AutoMigrate(&model.ProviderChannelBillingDiscount{}, &model.ProviderBillingAudit{}, &model.ProviderURLGroupDisplayName{}))
	actor := model.User{Id: 9691, Username: "submission-admin", AffCode: "submit9691", Role: common.RoleAdminUser, Status: common.UserStatusEnabled}
	require.NoError(t, model.DB.Create(&actor).Error)
	start := time.Date(2026, 9, 1, 0, 0, 0, 0, billingExportTimezone())
	for _, id := range []int{9691, 9692} {
		require.NoError(t, model.DB.Create(&model.Channel{Id: id, Name: "unvisited"}).Error)
		require.NoError(t, model.DB.Create(&model.ProviderChannelBillingDiscount{PeriodStart: start.AddDate(0, -1, 0).Unix(), ChannelId: id, Discount: decimal.RequireFromString("0.8"), Version: 1}).Error)
		require.NoError(t, model.DB.Create(&model.Log{ChannelId: id, Type: model.LogTypeConsume, CreatedAt: start.Unix() + 1}).Error)
	}
	t.Cleanup(func() {
		model.DB.Where("channel_id IN ?", []int{9691, 9692}).Delete(&model.ProviderChannelBillingDiscount{})
	})
	var auditsBefore int64
	require.NoError(t, model.DB.Model(&model.ProviderBillingAudit{}).Count(&auditsBefore).Error)
	job, err := SubmitUpstreamExportJob(context.Background(), actor.Id, CustomerExportRequest{JobType: model.CustomerExportJobTypeUpstreamSummary, StartTimestamp: start.Unix(), EndTimestamp: start.AddDate(0, 1, 0).Unix()}, []int{9691}, "channel:9691")
	require.NoError(t, err)
	filters, err := job.DecodeFilters()
	require.NoError(t, err)
	discount := filters.Upstream.Channels[9691].Discount
	require.NotNil(t, discount)
	assert.Equal(t, "0.8", discount.Value.String())
	assert.Equal(t, "previous_period", discount.Source)
	assert.Zero(t, discount.Version, "an unmaterialized coefficient must not claim a database version")
	var current, auditsAfter int64
	require.NoError(t, model.DB.Model(&model.ProviderChannelBillingDiscount{}).Where("period_start = ? AND channel_id IN ?", start.Unix(), []int{9691, 9692}).Count(&current).Error)
	require.NoError(t, model.DB.Model(&model.ProviderBillingAudit{}).Count(&auditsAfter).Error)
	assert.Zero(t, current, "export submission must not initialize selected or unrelated channels")
	assert.Equal(t, auditsBefore, auditsAfter)
}

func TestUpstreamSubmissionDoesNotReuseAnotherChannelInSameURL(t *testing.T) {
	truncate(t)
	setupCustomerExportServiceTest(t)
	require.NoError(t, model.DB.AutoMigrate(&model.ProviderChannelBillingDiscount{}, &model.ProviderBillingAudit{}, &model.ProviderURLGroupDisplayName{}))
	actor := model.User{Id: 9695, Username: "channel-scope-admin", AffCode: "scope9695", Role: common.RoleAdminUser, Status: common.UserStatusEnabled}
	require.NoError(t, model.DB.Create(&actor).Error)
	url := "https://scope.example"
	require.NoError(t, model.DB.Create(&[]model.Channel{{Id: 9695, BaseURL: &url}, {Id: 9696, BaseURL: &url}}).Error)
	start := time.Date(2026, 9, 1, 0, 0, 0, 0, billingExportTimezone())
	request := CustomerExportRequest{Language: "zh", JobType: model.CustomerExportJobTypeUpstreamDetails, StartTimestamp: start.Unix(), EndTimestamp: start.AddDate(0, 1, 0).Unix()}
	first, err := SubmitUpstreamExportJob(context.Background(), actor.Id, request, []int{9695}, url)
	require.NoError(t, err)
	_, err = SubmitUpstreamExportJob(context.Background(), actor.Id, request, []int{9696}, url)
	assert.ErrorIs(t, err, model.ErrCustomerExportUserBusy)
	_, err = SubmitUpstreamExportJob(context.Background(), actor.Id, request, []int{9695, 9696}, url)
	assert.ErrorIs(t, err, model.ErrCustomerExportUserBusy)
	retry, err := SubmitUpstreamExportJob(context.Background(), actor.Id, request, []int{9695}, url)
	require.NoError(t, err)
	assert.Equal(t, first.JobID, retry.JobID)
}

func TestUpstreamGlobalSubmissionDoesNotScanLogsBeforeQueue(t *testing.T) {
	truncate(t)
	setupCustomerExportServiceTest(t)
	require.NoError(t, model.DB.AutoMigrate(&model.ProviderChannelBillingDiscount{}, &model.ProviderBillingAudit{}, &model.ProviderURLGroupDisplayName{}))
	actor := model.User{Id: 9697, Username: "global-scope-admin", AffCode: "global9697", Role: common.RoleAdminUser, Status: common.UserStatusEnabled}
	require.NoError(t, model.DB.Create(&actor).Error)
	start := time.Date(2026, 9, 1, 0, 0, 0, 0, billingExportTimezone())
	require.NoError(t, model.DB.Create(&model.Log{ChannelId: 9698, Type: model.LogTypeConsume, CreatedAt: start.Unix() + 1}).Error)
	// Only the indexed ID boundary may be read before admission. Period reads
	// must run in the executor, where cancellation and pressure gates apply.
	const guard = "test:upstream_no_prequeue_log_scan"
	require.NoError(t, model.LOG_DB.Callback().Query().Before("gorm:query").Register(guard, func(tx *gorm.DB) {
		if tx.Statement.Table == "logs" {
			if _, filtered := tx.Statement.Clauses["WHERE"]; filtered {
				tx.AddError(errors.New("log scan before queue admission"))
			}
		}
	}))
	t.Cleanup(func() { model.LOG_DB.Callback().Query().Remove(guard) })
	request := CustomerExportRequest{JobType: model.CustomerExportJobTypeUpstreamDetails, UpstreamEvidenceFilter: "missing_historical_price_rows", StartTimestamp: start.Unix(), EndTimestamp: start.AddDate(0, 1, 0).Unix()}
	job, err := SubmitUpstreamExportJob(context.Background(), actor.Id, request, nil, "")
	require.NoError(t, err)
	assert.Equal(t, model.CustomerExportJobStatusQueued, job.Status)
	_, err = model.CancelCustomerExportJob(job.JobID, actor.Id)
	require.NoError(t, err)
	require.NoError(t, model.DB.Model(&model.CustomerExportSlot{}).Where("id > 0").Update("job_id", "occupied").Error)
	_, err = SubmitUpstreamExportJob(context.Background(), actor.Id, request, nil, "")
	assert.ErrorIs(t, err, model.ErrCustomerExportQueueBusy, "a full queue must reject without a source log scan")
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = SubmitUpstreamExportJob(cancelled, actor.Id, request, nil, "")
	assert.ErrorIs(t, err, context.Canceled, "submission metadata reads respect request cancellation")
}

func TestUpstreamSubmissionRetryKeepsOriginalLogBoundary(t *testing.T) {
	truncate(t)
	setupCustomerExportServiceTest(t)
	require.NoError(t, model.DB.AutoMigrate(&model.ProviderChannelBillingDiscount{}, &model.ProviderBillingAudit{}, &model.ProviderURLGroupDisplayName{}))
	actor := model.User{Id: 9693, Username: "retry-admin", AffCode: "retry9693", Role: common.RoleAdminUser, Status: common.UserStatusEnabled}
	require.NoError(t, model.DB.Create(&actor).Error)
	require.NoError(t, model.DB.Create(&model.Channel{Id: 9693, Name: "retry"}).Error)
	start := time.Date(2026, 9, 1, 0, 0, 0, 0, billingExportTimezone())
	request := CustomerExportRequest{Language: "zh", JobType: model.CustomerExportJobTypeUpstreamSummary, StartTimestamp: start.Unix(), EndTimestamp: start.AddDate(0, 1, 0).Unix()}
	first, err := SubmitUpstreamExportJob(context.Background(), actor.Id, request, []int{9693}, "channel:9693")
	require.NoError(t, err)
	require.NoError(t, model.DB.Create(&model.Log{ChannelId: 9694, Type: model.LogTypeConsume, CreatedAt: start.Unix() + 1}).Error)
	retry, err := SubmitUpstreamExportJob(context.Background(), actor.Id, request, []int{9693}, "channel:9693")
	require.NoError(t, err)
	assert.Equal(t, first.JobID, retry.JobID)
	assert.Equal(t, first.Filters, retry.Filters, "retry must retain the accepted data boundary")
	_, total, err := model.ListCustomerExportJobs(context.Background(), actor.Id, 1, 20)
	require.NoError(t, err)
	assert.EqualValues(t, 1, total)
	request.Language = "en"
	_, err = SubmitUpstreamExportJob(context.Background(), actor.Id, request, []int{9693}, "channel:9693")
	assert.ErrorIs(t, err, model.ErrCustomerExportUserBusy, "a different requested language must not reuse the file")
	_, err = model.CancelCustomerExportJob(first.JobID, actor.Id)
	require.NoError(t, err)
	request.Language = "zh"
	newer, err := SubmitUpstreamExportJob(context.Background(), actor.Id, request, []int{9693}, "channel:9693")
	require.NoError(t, err)
	assert.NotEqual(t, first.Filters, newer.Filters, "a new request captures the newer log boundary")
	_, err = ResubmitUpstreamExportJob(actor.Id, first.JobID)
	assert.ErrorIs(t, err, model.ErrCustomerExportUserBusy, "regeneration must not return a different frozen snapshot")
}
