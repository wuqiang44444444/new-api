package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/clienterrlog"
	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/operation_setting"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestErrorReportHourFloorBoundary(t *testing.T) {
	loc := errorReportLocation()
	floor := func(hour, min, sec int) int64 {
		at := time.Date(2026, 9, 17, hour, min, sec, 0, loc)
		return errorReportHourFloor(at)
	}
	expected10 := time.Date(2026, 9, 17, 10, 0, 0, 0, loc).Unix()
	expected11 := time.Date(2026, 9, 17, 11, 0, 0, 0, loc).Unix()
	assert.Equal(t, expected10, floor(10, 20, 0), "10:20 启用时基准窗口应为 10:00")
	assert.Equal(t, expected10, floor(10, 59, 59), "小时末仍属于同一窗口")
	assert.Equal(t, expected11, floor(11, 0, 0), "恰好整点进入后一个窗口")
}

func TestErrorReportBackoffSchedule(t *testing.T) {
	assert.Equal(t, int64(60), errorReportBackoff(1))
	assert.Equal(t, int64(300), errorReportBackoff(2))
	assert.Equal(t, int64(900), errorReportBackoff(3))
	assert.Equal(t, int64(3600), errorReportBackoff(4))
	assert.Equal(t, int64(3600), errorReportBackoff(100), "超过上限后按小时重试，不丢弃")
	assert.Equal(t, int64(60), errorReportBackoff(0))
}

func TestClassifyErrorEventTable(t *testing.T) {
	cases := []struct {
		name      string
		eventType string
		reason    string
		status    int
		want      string
	}{
		{"渠道测试单独归类", clienterrlog.EventChannelTest, "", 0, errorReportClassChannelTest},
		{"上游流式失败归平台侧", clienterrlog.EventStreamError, "upstream_response_failed", 200, errorReportClassPlatform},
		{"客户端断开归客户侧", clienterrlog.EventStreamError, "client_disconnected", 200, errorReportClassClient},
		{"流式异常单独归类", clienterrlog.EventStreamError, "aws_event_stream_error", 200, errorReportClassStream},
		{"白名单原因码判客户侧", clienterrlog.EventAPIError, "token_model_forbidden", 403, errorReportClassClient},
		{"素材原因码判客户侧", clienterrlog.EventAPIError, "unsupported_asset_operation", 400, errorReportClassClient},
		{"5xx 判平台上游", clienterrlog.EventAPIError, "unclassified", 502, errorReportClassPlatform},
		{"任务失败 5xx 判平台上游", clienterrlog.EventTaskFailure, "", 500, errorReportClassPlatform},
		{"任务失败客户原因码判客户侧", clienterrlog.EventTaskFailure, "invalid_request", 400, errorReportClassClient},
		{"上游 4xx 不猜测归责", clienterrlog.EventAPIError, "unclassified", 401, errorReportClassUnknown},
		{"429 不归责任何一方", clienterrlog.EventAPIError, "rate_limit", 429, errorReportClassUnknown},
		{"空类型历史行按 api_error", "", "unclassified", 500, errorReportClassPlatform},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			event := &model.ErrorEvent{EventType: tc.eventType, Reason: tc.reason, Status: tc.status}
			assert.Equal(t, tc.want, classifyErrorEvent(event))
		})
	}
}

func TestResolveErrorReportStatusWording(t *testing.T) {
	counts := func(class string, n int) map[string]int { return map[string]int{class: n} }

	assert.Equal(t, "发现平台／上游相关异常 / Platform / upstream errors detected", resolveErrorReportStatus(1, counts(errorReportClassPlatform, 1), false, "").Headline)
	assert.Equal(t, "存在待分类错误，需排查 / Unclassified errors require investigation", resolveErrorReportStatus(1, counts(errorReportClassUnknown, 1), false, "").Headline)
	clientOnly := resolveErrorReportStatus(3, map[string]int{errorReportClassClient: 3}, false, "")
	assert.Equal(t, "已记录客户侧错误 / Client-side errors recorded", clientOnly.Headline)
	assert.Contains(t, clientOnly.Body, "未在本次数据中发现明确的平台／上游故障")

	noErrorsWithActivity := resolveErrorReportStatus(0, map[string]int{}, true, "note")
	assert.Equal(t, "本次已入库记录中未发现错误 / No errors found in persisted records", noErrorsWithActivity.Headline)

	noErrorsNoEvidence := resolveErrorReportStatus(0, map[string]int{}, false, "")
	assert.Equal(t, "数据不足，无法判断 / Insufficient evidence to assess status", noErrorsNoEvidence.Headline)
	assert.Contains(t, noErrorsNoEvidence.Body, "不能把缺少记录等同于系统正常")
}

func TestRenderDetailCellNeverSplitsEntities(t *testing.T) {
	// 包含需要转义的字符与多字节字符；分段后每段单独转义，实体保持完整。
	value := "<script>alert('x')</script>中文&值" + strings.Repeat("abc中文&", 60)
	detail, err := common.Marshal(map[string]string{"msg": value, "empty": ""})
	require.NoError(t, err)
	chunks := renderDetailCell(string(detail), 512)
	require.Greater(t, len(chunks), 1)
	joined := strings.Join(chunks, "")
	assert.Contains(t, joined, "&lt;script&gt;")
	assert.Contains(t, joined, "alert(&#39;x&#39;)")
	assert.Contains(t, joined, "值")
	assert.Contains(t, joined, "empty=—")
	for _, chunk := range chunks {
		assert.LessOrEqual(t, len(chunk), 512)
		assert.NotContains(t, chunk, "<script>", "动态字段必须转义")
		assert.NotContains(t, chunk, "&abc", "转义实体不得被切破后残留")
	}
}

func TestEscapeChunkedKeepsMultibyteRunes(t *testing.T) {
	chunks := escapeChunked("中文abc中文", 5)
	require.NotEmpty(t, chunks)
	assert.Equal(t, "中文abc中文", strings.Join(chunks, ""))
	for _, chunk := range chunks {
		assert.LessOrEqual(t, len(chunk), 5*4+12) // 转义余量
	}
}

// withErrorReportTestDB 建立内存 SQLite 主库与日志库（同库拓扑），
// 只迁移本功能需要的表，隔离其它测试的模型状态。
func withErrorReportTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	previousOptions := common.OptionMap
	common.OptionMap = map[string]string{}
	t.Cleanup(func() { common.OptionMap = previousOptions })
	previousDB := model.DB
	previousLogDB := model.LOG_DB
	previousType := common.MainDatabaseType()
	previousLogType := common.LogDatabaseType()
	previousCache := common.MemoryCacheEnabled
	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "_"))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(
		&model.Option{},
		&model.ErrorReportSchedule{},
		&model.ErrorReport{},
		&model.ErrorReportPart{},
		&model.ErrorReportDelivery{},
		&model.ErrorEvent{},
		&model.Channel{},
		&model.PerfMetric{},
		&model.SystemTask{},
		&model.SystemTaskLock{},
	))
	model.DB = db
	model.LOG_DB = db
	common.SetMainDatabaseType(common.DatabaseTypeSQLite)
	common.SetLogDatabaseType(common.DatabaseTypeSQLite)
	common.MemoryCacheEnabled = false
	t.Cleanup(func() {
		model.DB = previousDB
		model.LOG_DB = previousLogDB
		common.SetMainDatabaseType(previousType)
		common.SetLogDatabaseType(previousLogType)
		common.MemoryCacheEnabled = previousCache
	})
	return db
}

func withErrorReportSetting(t *testing.T, enabled bool, recipients string) {
	t.Helper()
	setting := operation_setting.GetErrorReportSetting()
	previousEnabled, previousRecipients := setting.Enabled, setting.Recipients
	setting.Enabled = enabled
	setting.Recipients = recipients
	require.NoError(t, model.UpdateErrorReportOption("error_report_setting.recipients", recipients, beijingTime(10, 20)))
	require.NoError(t, model.UpdateErrorReportOption("error_report_setting.enabled", fmt.Sprint(enabled), beijingTime(10, 20)))
	t.Cleanup(func() {
		setting.Enabled = previousEnabled
		setting.Recipients = previousRecipients
	})
}

func withErrorReportSMTP(t *testing.T) {
	t.Helper()
	previousServer, previousAccount := common.SMTPServer, common.SMTPAccount
	common.SMTPServer = "smtp.example.com"
	common.SMTPAccount = "report@example.com"
	t.Cleanup(func() {
		common.SMTPServer = previousServer
		common.SMTPAccount = previousAccount
	})
}

type sentMail struct {
	subject string
	rcpt    string
	body    string
}

type errorReportMailRecorder struct {
	mails []sentMail
	err   error
}

func (r *errorReportMailRecorder) send(_ context.Context, subject, rcpt, body string) error {
	if r.err != nil {
		return r.err
	}
	r.mails = append(r.mails, sentMail{subject: subject, rcpt: rcpt, body: body})
	return nil
}

func beijingTime(hour, min int) time.Time {
	loc := errorReportLocation()
	return time.Date(2026, 9, 17, hour, min, 0, 0, loc)
}

func insertErrorEvent(t *testing.T, db *gorm.DB, at time.Time, eventType, reason string, status int) {
	t.Helper()
	require.NoError(t, db.Create(&model.ErrorEvent{
		CreatedAt: at.Unix(),
		Module:    model.ErrorEventModuleRelay,
		EventType: eventType,
		Reason:    reason,
		Status:    status,
		RequestId: fmt.Sprintf("req-%d", at.Unix()),
	}).Error)
}

func claimErrorReportTask(t *testing.T, runnerID string) *model.SystemTask {
	t.Helper()
	task, err := model.CreateSystemTask(model.SystemTaskTypeErrorReport, nil, nil)
	require.NoError(t, err)
	claimedTask, claimed, err := model.ClaimSystemTask(task.ID, model.SystemTaskTypeErrorReport, runnerID, common.GetTimestamp()+60)
	require.NoError(t, err)
	require.True(t, claimed)
	return claimedTask
}

// TestErrorReportFullFlow 验证：整点窗口到期生成 → 冻结分卷发布 → 逐地址
// 投递 → SMTP 失败退避 → 租约过期恢复 → 收件人移除停止，以及 11:00 边界
// 事件只进入后一个窗口。
func TestErrorReportFullFlow(t *testing.T) {
	db := withErrorReportTestDB(t)
	withErrorReportSetting(t, true, "ops@example.com;backup@example.com")
	withErrorReportSMTP(t)

	loc := errorReportLocation()
	window10 := time.Date(2026, 9, 17, 10, 0, 0, 0, loc).Unix()
	insertErrorEvent(t, db, time.Unix(window10, 0).In(loc), clienterrlog.EventAPIError, "unclassified", 502)
	insertErrorEvent(t, db, time.Unix(window10+120, 0).In(loc), clienterrlog.EventAPIError, "token_model_forbidden", 403)
	// 恰好 11:00 的事件属于后一个窗口。
	boundary := time.Date(2026, 9, 17, 11, 0, 0, 0, loc).Unix()
	insertErrorEvent(t, db, time.Unix(boundary, 0).In(loc), clienterrlog.EventAPIError, "unclassified", 500)

	// 预设调度基准为 10:00，模拟 10:20 开启后 11:05 的首轮处理。
	require.NoError(t, model.UpdateErrorReportOption("error_report_setting.enabled", "true", time.Unix(window10, 0)))
	recorder := &errorReportMailRecorder{}
	handler := newErrorReportHandler(recorder.send)
	now := beijingTime(11, 5)
	handler.now = func() time.Time { return now }

	task := claimErrorReportTask(t, "runner-1")
	handler.Run(context.Background(), task, "runner-1")

	finished, err := model.GetSystemTaskByTaskID(task.TaskID)
	require.NoError(t, err)
	require.Equal(t, model.SystemTaskStatusSucceeded, finished.Status)

	report, err := model.GetErrorReportByWindowStart(window10)
	require.NoError(t, err)
	require.NotNil(t, report)
	assert.Equal(t, model.ErrorReportStatusReady, report.Status)
	assert.Equal(t, 2, report.TotalEvents, "11:00 边界事件不得进入 10-11 窗口")
	require.Positive(t, report.TotalParts)

	var parts []*model.ErrorReportPart
	require.NoError(t, db.Where("report_id = ?", report.ReportID).Order("part_no asc").Find(&parts).Error)
	require.Len(t, parts, report.TotalParts)
	rowSum := 0
	for _, part := range parts {
		rowSum += part.DetailRows
	}
	assert.Equal(t, 2, rowSum, "每条错误在分卷中只计一次")
	for _, part := range parts {
		assert.LessOrEqual(t, len(part.BodyHTML), errorReportPartBudgetBytes+1024, "分卷应保持有界")
	}

	require.Len(t, recorder.mails, 2*report.TotalParts, "每个收件人每卷一封")
	assert.Contains(t, recorder.mails[0].subject, "系统运行与错误报告")
	assert.Contains(t, recorder.mails[0].subject, "Operations and errors")
	assert.NotContains(t, recorder.mails[0].subject, "[补发]", "窗口结束后 30 分钟内不算补发")
	firstBody := ""
	for _, mail := range recorder.mails {
		if mail.rcpt == "ops@example.com" {
			firstBody = mail.body
		}
	}
	require.NotEmpty(t, firstBody)
	assert.Contains(t, firstBody, "发现平台／上游相关异常 / Platform / upstream errors detected")
	assert.Contains(t, firstBody, "errrep-")
	assert.Contains(t, firstBody, "截至生成时刻已入库")
	for _, label := range []string{"Operational summary", "Data scope", "Reason code", "HTTP status", "fixed snapshot", "sanitized, safe fields"} {
		assert.Contains(t, firstBody, label, "both languages must be frozen before publication")
	}
	assert.NotContains(t, firstBody, "req-"+fmt.Sprint(boundary), "边界事件不得出现在本窗口正文")

	schedule, err := model.GetErrorReportSchedule()
	require.NoError(t, err)
	assert.Equal(t, window10+3600, schedule.NextWindowStart)

	// 第二轮：12:05 生成 11-12 窗口（含 11:00 边界事件），但 SMTP 持续失败。
	nextWindow := window10 + 3600
	failing := &errorReportMailRecorder{err: errors.New("connection refused")}
	handler.send = failing.send
	now = beijingTime(12, 5)
	recorder.mails = nil
	task = claimErrorReportTask(t, "runner-1")
	handler.Run(context.Background(), task, "runner-1")
	report2, err := model.GetErrorReportByWindowStart(nextWindow)
	require.NoError(t, err)
	require.NotNil(t, report2)
	require.Empty(t, recorder.mails, "发送失败不应产生邮件记录")

	// 失败投递进入 retry_wait 并带退避时间。
	var retryDelivery model.ErrorReportDelivery
	require.NoError(t, db.Where("report_id = ? AND recipient = ?", report2.ReportID, "ops@example.com").First(&retryDelivery).Error)
	assert.Equal(t, model.ErrorReportDeliveryRetryWait, retryDelivery.Status)
	assert.Equal(t, "connection refused", retryDelivery.LastError)
	assert.Greater(t, retryDelivery.NextAttemptAt, now.Unix())

	// 过期 sending 恢复：直接把一行置为过期 sending，Requeue 应拉回 retry_wait。
	expired := now.Unix() - 10
	require.NoError(t, db.Model(&model.ErrorReportDelivery{}).
		Where("id = ?", retryDelivery.ID).
		Updates(map[string]any{"status": model.ErrorReportDeliverySending, "lease_until": expired, "lease_owner": "stale-runner"}).Error)
	requeued, err := model.RequeueStaleErrorReportDeliveries(now.Unix(), 10)
	require.NoError(t, err)
	assert.Equal(t, int64(1), requeued)
	require.NoError(t, db.First(&retryDelivery, retryDelivery.ID).Error)
	assert.Equal(t, model.ErrorReportDeliveryRetryWait, retryDelivery.Status)

	// 收件人移除：停止对已移除地址的后续尝试，保留事实；已移除地址
	// 不参与后续补发。
	withErrorReportSetting(t, true, "ops@example.com")
	stopped, err := model.StopErrorReportDeliveriesForRecipients(map[string]bool{"ops@example.com": true})
	require.NoError(t, err)
	assert.Equal(t, int64(report2.TotalParts), stopped)
	var kept, removed model.ErrorReportDelivery
	require.NoError(t, db.Where("report_id = ? AND recipient = ?", report2.ReportID, "ops@example.com").First(&kept).Error)
	require.NoError(t, db.Where("report_id = ? AND recipient = ?", report2.ReportID, "backup@example.com").First(&removed).Error)
	assert.NotEqual(t, model.ErrorReportDeliveryStopped, kept.Status)
	assert.Equal(t, model.ErrorReportDeliveryStopped, removed.Status)
	assert.Equal(t, model.ErrorReportStopRecipientRemoved, removed.StopReason)

	// 第三轮：13:00（超出 30 分钟宽限）恢复发送，积压窗口按补发标注，
	// 正文与分卷不漂移，且只发给保留的收件人。
	handler.send = recorder.send
	now = beijingTime(13, 0)
	recorder.mails = nil
	task = claimErrorReportTask(t, "runner-1")
	handler.Run(context.Background(), task, "runner-1")
	var resendMails []sentMail
	combined := ""
	for _, mail := range recorder.mails {
		combined += mail.body
		if strings.HasPrefix(mail.subject, "[补发]") {
			resendMails = append(resendMails, mail)
		}
	}
	require.Len(t, resendMails, report2.TotalParts, "仅保留的收件人收到补发")
	assert.Contains(t, resendMails[0].subject, "11:00–12:00")
	assert.Contains(t, resendMails[0].subject, "[Resent]")
	assert.Contains(t, combined, "req-"+fmt.Sprint(boundary))
}

// TestErrorReportSourceFailureDoesNotAdvanceWindow 验证源日志库故障时：
// 窗口被记录为 failed、调度不推进，恢复后同一窗口可重建且内容稳定。
func TestErrorReportSourceFailureDoesNotAdvanceWindow(t *testing.T) {
	db := withErrorReportTestDB(t)
	withErrorReportSetting(t, true, "ops@example.com")
	withErrorReportSMTP(t)

	loc := errorReportLocation()
	window10 := time.Date(2026, 9, 17, 10, 0, 0, 0, loc).Unix()
	insertErrorEvent(t, db, time.Unix(window10, 0).In(loc), clienterrlog.EventAPIError, "unclassified", 500)

	require.NoError(t, model.UpdateErrorReportOption("error_report_setting.enabled", "true", time.Unix(window10, 0)))
	recorder := &errorReportMailRecorder{}
	handler := newErrorReportHandler(recorder.send)
	now := beijingTime(11, 5)
	handler.now = func() time.Time { return now }

	// 日志库不可用：Run 不推进窗口。
	model.LOG_DB = nil
	task := claimErrorReportTask(t, "runner-1")
	handler.Run(context.Background(), task, "runner-1")
	finished, err := model.GetSystemTaskByTaskID(task.TaskID)
	require.NoError(t, err)
	assert.Equal(t, model.SystemTaskStatusFailed, finished.Status)
	assert.Contains(t, finished.Error, "生成失败 1 个窗口")

	report, err := model.GetErrorReportByWindowStart(window10)
	require.NoError(t, err)
	require.NotNil(t, report)
	assert.Equal(t, model.ErrorReportStatusFailed, report.Status)
	assert.NotEmpty(t, report.FailureReason)
	assert.Empty(t, recorder.mails, "失败窗口不得发送半份报告")

	schedule, err := model.GetErrorReportSchedule()
	require.NoError(t, err)
	assert.Equal(t, window10, schedule.NextWindowStart, "源数据故障不得推进窗口")

	// 恢复后同一窗口重建为 ready 并正常投递。
	model.LOG_DB = db
	task = claimErrorReportTask(t, "runner-1")
	handler.Run(context.Background(), task, "runner-1")
	report, err = model.GetErrorReportByWindowStart(window10)
	require.NoError(t, err)
	assert.Equal(t, model.ErrorReportStatusReady, report.Status)
	require.NotEmpty(t, recorder.mails)
	schedule, err = model.GetErrorReportSchedule()
	require.NoError(t, err)
	assert.Equal(t, window10+3600, schedule.NextWindowStart)
}

// TestErrorReportPartsSplitWithManyEvents 验证数千字节级明细按预算分卷：
// 每卷有界、明细总数一致、主题分卷编号正确。
func TestErrorReportPartsSplitWithManyEvents(t *testing.T) {
	withErrorReportTestDB(t)
	loc := errorReportLocation()
	windowStart := time.Date(2026, 9, 17, 10, 0, 0, 0, loc).Unix()

	events := make([]*model.ErrorEvent, 0, 400)
	for i := 0; i < 400; i++ {
		detail, err := common.Marshal(map[string]string{
			"stream_reason":     fmt.Sprintf("upstream reset #%03d <b>&\"中文\"", i),
			"stream_end_reason": "error",
		})
		require.NoError(t, err)
		events = append(events, &model.ErrorEvent{
			CreatedAt: windowStart + int64(i),
			Module:    model.ErrorEventModuleRelay,
			EventType: clienterrlog.EventAPIError,
			Reason:    "unclassified",
			Status:    502,
			RequestId: fmt.Sprintf("req-many-%03d", i),
			Detail:    string(detail),
		})
	}
	handler := newErrorReportHandler(nil)
	view, err := handler.renderErrorReportWindow(windowStart, windowStart+3600, events)
	require.NoError(t, err)
	require.Greater(t, view.TotalParts, 1, "大量明细应拆成多卷")
	rowSum := 0
	combined := ""
	for index, draft := range view.Drafts {
		rowSum += draft.DetailRows
		combined += draft.BodyHTML
		assert.LessOrEqual(t, len(draft.BodyHTML), errorReportPartBudgetBytes+1024)
		assert.Contains(t, draft.Subject, fmt.Sprintf("Part %d/%d", index+1, view.TotalParts))
	}
	assert.NotContains(t, combined, "<b>", "动态字段必须转义")
	assert.Contains(t, combined, "&lt;b&gt;")
	assert.Equal(t, 400, rowSum, "每个快照事件只计一次，总数一致")
	assert.Equal(t, 400, view.TotalEvents)
}

// TestErrorReportEmptyWindowStillSendsSummary 验证无错误时仍生成运行摘要，
// 且统计缺失时状态为「数据不足，无法判断」。
func TestErrorReportEmptyWindowStillSendsSummary(t *testing.T) {
	withErrorReportTestDB(t)
	loc := errorReportLocation()
	windowStart := time.Date(2026, 9, 17, 10, 0, 0, 0, loc).Unix()

	handler := newErrorReportHandler(nil)
	view, err := handler.renderErrorReportWindow(windowStart, windowStart+3600, nil)
	require.NoError(t, err)
	assert.Equal(t, 0, view.TotalEvents)
	assert.Equal(t, "数据不足，无法判断 / Insufficient evidence to assess status", view.Status.Headline)
	require.NotEmpty(t, view.Drafts)
	combined := view.Drafts[0].BodyHTML
	assert.Contains(t, combined, "本窗口已入库记录中未发现错误")
	assert.Contains(t, combined, "错误分类汇总 / Error classification summary")
}

// TestErrorReportClaimExclusivity 验证投递领取的互斥性：已被租约持有的行
// 不能被其他执行者领走；租约过期后才能被重新领取。这是多节点不重复发送
// 同一分卷的关键不变量。
func TestErrorReportClaimExclusivity(t *testing.T) {
	withErrorReportTestDB(t)
	withErrorReportSetting(t, true, "ops@example.com")
	withErrorReportSMTP(t)

	windowStart := beijingTime(10, 0).Unix()
	require.NoError(t, model.UpdateErrorReportOption("error_report_setting.enabled", "true", time.Unix(windowStart, 0)))
	report := &model.ErrorReport{
		ReportID:    "errrep-claim-test",
		WindowStart: windowStart,
		WindowEnd:   windowStart + 3600,
		Timezone:    "Asia/Shanghai",
		Status:      model.ErrorReportStatusBuilding,
	}
	inserted, err := model.InsertErrorReportBuilding(report)
	require.NoError(t, err)
	require.True(t, inserted)
	require.NoError(t, publishErrorReportTestParts(report.ReportID, []*model.ErrorReportPart{{
		ReportID: report.ReportID,
		PartNo:   1,
		Subject:  "s",
		BodyHTML: "b",
	}}, []string{"ops@example.com"}, 0, "{}", "note"))

	now := time.Now().Unix()
	ownerA := "runner-a"
	claimsA, err := model.ClaimErrorReportDeliveries(now, ownerA, now+90, 10)
	require.NoError(t, err)
	require.Len(t, claimsA, 1, "首个执行者应领到期投递")

	// 同一时刻另一执行者不能领走已持有租约的行。
	claimsB, err := model.ClaimErrorReportDeliveries(now, "runner-b", now+90, 10)
	require.NoError(t, err)
	assert.Empty(t, claimsB, "持有租约的行不得被其他执行者领取")

	// 未到下次尝试时间的已接受行同样不可重复领取。
	_, err = model.FinishErrorReportDelivery(claimsA[0].ID, ownerA, model.ErrorReportDeliveryAccepted, "", "", 0, now)
	require.NoError(t, err)
	claimsC, err := model.ClaimErrorReportDeliveries(now, "runner-c", now+90, 10)
	require.NoError(t, err)
	assert.Empty(t, claimsC, "已接受投递不得重复领取")

}
