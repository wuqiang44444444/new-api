package service

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/operation_setting"
)

// 每小时系统运行与错误邮件报告的系统任务处理器
// （docs/80-dev/2026-09-17-每小时系统运行与错误邮件报告开发方案.md）。
// 处理器注册为 ScheduledSystemTaskHandler：短周期（一分钟）检查到期窗口、
// 生成冻结报告并投递积压；窗口、正文与投递进度全部持久化在主库，重启可恢复。

const (
	// errorReportCheckInterval 是调度间隔；内部按绝对时间计算窗口，
	// 不以「上次发送结束加一小时」定义统计范围。
	errorReportCheckInterval = time.Minute

	errorReportLocationName  = "Asia/Shanghai"
	errorReportWindowSeconds = int64(3600)

	// 每轮有限工作量：大量积压或故障邮箱不会长期占用租约，未完成进度可续跑。
	errorReportMaxGeneratePerRun = 2
	errorReportMaxDeliverPerRun  = 30
	errorReportMaxRequeuePerRun  = 200

	// 投递领取租约：发送在事务外执行，结果不明（含 SMTP 已接受但写回失败）
	// 由租约过期恢复为 retry_wait，允许偶尔重复。
	errorReportDeliveryLease = 90 * time.Second

	// errorReportResendGrace 之后发送的窗口按「补发」标注标题。
	errorReportResendGrace = 30 * time.Minute
)

// errorReportBackoffSteps 是失败/结果不明的重试退避（秒）：
// 1、5、15、60 分钟，之后按小时重试；达到上限也不丢弃，保留积压与下次尝试时间。
var errorReportBackoffSteps = []int64{60, 300, 900, 3600}

func errorReportBackoff(attempts int) int64 {
	if attempts <= 0 {
		return errorReportBackoffSteps[0]
	}
	if attempts > len(errorReportBackoffSteps) {
		return errorReportBackoffSteps[len(errorReportBackoffSteps)-1]
	}
	return errorReportBackoffSteps[attempts-1]
}

type errorReportSendFunc func(ctx context.Context, subject, receiver, body string) error

type errorReportHandler struct {
	send errorReportSendFunc
	now  func() time.Time
}

func newErrorReportHandler(send errorReportSendFunc) *errorReportHandler {
	return &errorReportHandler{send: send, now: time.Now}
}

func init() {
	RegisterSystemTaskHandler(newErrorReportHandler(common.SendEmailContext))
}

func (errorReportHandler) Type() string { return model.SystemTaskTypeErrorReport }

// Enabled 直接跟随配置；调度器在关闭时不创建任务。
func (errorReportHandler) Enabled() bool { return operation_setting.GetErrorReportSetting().Enabled }

func (errorReportHandler) Interval() time.Duration { return errorReportCheckInterval }

func (errorReportHandler) NewPayload() any { return nil }

// errorReportLocation 固定北京时间；时区数据缺失时退化为 +08:00 固定偏移。
func errorReportLocation() *time.Location {
	loc, err := time.LoadLocation(errorReportLocationName)
	if err != nil {
		return time.FixedZone("CST", 8*3600)
	}
	return loc
}

// errorReportHourFloor 取时刻所在自然小时的起点（绝对时间戳）。
func errorReportHourFloor(t time.Time) int64 {
	inLoc := t.In(errorReportLocation())
	floored := time.Date(inLoc.Year(), inLoc.Month(), inLoc.Day(), inLoc.Hour(), 0, 0, 0, inLoc.Location())
	return floored.Unix()
}

// errorReportID 由窗口起点生成稳定报告标识，用于补发识别与去重关联。
func errorReportID(windowStart int64) string {
	return fmt.Sprintf("errrep-%d", windowStart)
}

// Run 从持久配置读取许可，生成到期窗口，再恢复过期投递、清理收件人并发送。
// 单地址失败不影响其它地址；实际失败及积压写入现有系统任务的可见错误摘要。
func (h *errorReportHandler) Run(ctx context.Context, task *model.SystemTask, runnerID string) {
	enabled, recipients, err := model.GetErrorReportConfiguration(ctx)
	if err != nil {
		failSystemTask(task, runnerID, err)
		return
	}
	if !enabled {
		finishErrorReportTask(task, runnerID, errorReportTaskResult{Note: "disabled"}, nil)
		return
	}
	if len(recipients) == 0 {
		failSystemTask(task, runnerID, errors.New("每小时报告已启用但未配置合法收件人"))
		return
	}
	if common.SMTPServer == "" || common.SMTPAccount == "" {
		failSystemTask(task, runnerID, errors.New("SMTP 服务器未配置，无法发送每小时报告"))
		return
	}

	result := errorReportTaskResult{}
	generated, failedWindows, genErr := h.generateDueWindows(ctx, recipients)
	result.GeneratedWindows = generated
	result.FailedWindows = failedWindows
	if genErr != nil {
		logger.LogWarn(ctx, fmt.Sprintf("error report generation pass failed: %v", genErr))
	}

	requeued, err := model.RequeueStaleErrorReportDeliveries(h.now().Unix(), errorReportMaxRequeuePerRun)
	if err != nil {
		finishErrorReportTask(task, runnerID, result, err)
		return
	}
	result.RequeuedStale = requeued
	keep := make(map[string]bool, len(recipients))
	for _, recipient := range recipients {
		keep[recipient] = true
	}
	stopped, err := model.StopErrorReportDeliveriesForRecipients(keep)
	if err != nil {
		finishErrorReportTask(task, runnerID, result, err)
		return
	}
	result.StoppedRemoved = stopped

	attempted, accepted, sendErr := h.deliverClaims(ctx, runnerID)
	result.DeliverAttempted = attempted
	result.DeliverAccepted = accepted
	if sendErr != nil {
		logger.LogWarn(ctx, fmt.Sprintf("error report delivery pass failed: %v", sendErr))
	}

	stats, err := model.GetErrorReportBacklogStats()
	if err != nil {
		logger.LogWarn(ctx, fmt.Sprintf("error report backlog stats failed: %v", err))
	} else {
		result.BacklogReports = stats.ReportsWithPending
		result.BacklogDeliveries = stats.PendingDeliveries
		result.EarliestBacklogWindow = stats.EarliestWindow
	}

	finishErrorReportTask(task, runnerID, result, errors.Join(genErr, sendErr, err))
}

// errorReportTaskResult 是系统任务结果的安全汇总：不包含正文、收件人或敏感信息。
type errorReportTaskResult struct {
	Note                  string   `json:"note,omitempty"`
	GeneratedWindows      int      `json:"generated_windows"`
	FailedWindows         []string `json:"failed_windows,omitempty"`
	DeliverAttempted      int      `json:"deliver_attempted"`
	DeliverAccepted       int      `json:"deliver_accepted"`
	StoppedRemoved        int64    `json:"stopped_removed_recipients"`
	RequeuedStale         int64    `json:"requeued_stale"`
	BacklogReports        int64    `json:"backlog_reports"`
	BacklogDeliveries     int64    `json:"backlog_deliveries"`
	EarliestBacklogWindow int64    `json:"earliest_backlog_window"`
}

func finishErrorReportTask(task *model.SystemTask, runnerID string, result errorReportTaskResult, runErr error) {
	status := model.SystemTaskStatusSucceeded
	message := ""
	if runErr != nil {
		status = model.SystemTaskStatusFailed
		message = sanitizeErrorReportError(runErr)
	}
	if runErr != nil || result.BacklogDeliveries > 0 || len(result.FailedWindows) > 0 {
		message = fmt.Sprintf("生成失败 %d 个窗口；投递 %d/%d 成功；积压 %d 封。%s", len(result.FailedWindows), result.DeliverAccepted, result.DeliverAttempted, result.BacklogDeliveries, message)
	}
	if err := model.FinishSystemTask(task.TaskID, runnerID, status, result, message); err != nil {
		logSystemTaskLockError(context.Background(), task, err)
	}
}

func sanitizeErrorReportError(err error) string {
	if err == nil {
		return ""
	}
	text := strings.ReplaceAll(err.Error(), "\n", " ")
	text = strings.ReplaceAll(text, "\r", " ")
	const maxLen = 256
	if len(text) > maxLen {
		text = text[:maxLen]
	}
	return text
}

// generateDueWindows 按持久化调度进度生成到期窗口。窗口进度只能在报告
// 持久化可恢复后推进；源数据故障时记录生成失败并停止本轮，不跳过窗口。
func (h *errorReportHandler) generateDueWindows(ctx context.Context, recipients []string) (int, []string, error) {
	loc := errorReportLocation()
	generated := 0
	var failed []string
	for i := 0; i < errorReportMaxGeneratePerRun; i++ {
		if ctx.Err() != nil {
			return generated, failed, ctx.Err()
		}
		windowStart, err := model.NextErrorReportWindow(ctx)
		if err != nil {
			return generated, failed, err
		}
		if windowStart == 0 {
			break
		}
		if windowStart+errorReportWindowSeconds > h.now().Unix() {
			break // 窗口未到期
		}
		if err := h.processWindow(ctx, windowStart, recipients); err != nil {
			failed = append(failed, fmt.Sprintf("%s: %s",
				time.Unix(windowStart, 0).In(loc).Format("2006-01-02 15:04"), sanitizeErrorReportError(err)))
			return generated, failed, err
		}
		generated++
	}
	return generated, failed, nil
}

// processWindow 生成单个小时报告：已有 ready 报告只推进窗口；building/
// failed 报告或新窗口先做单次一致快照读取，再在单个事务内发布分卷与
// 投递行；发布成功后 CAS 推进窗口。崩溃后 ready 报告只推进进度，不重建正文。
func (h *errorReportHandler) processWindow(ctx context.Context, windowStart int64, recipients []string) error {
	windowEnd := windowStart + errorReportWindowSeconds
	report, err := model.GetErrorReportByWindowStart(windowStart)
	if err != nil {
		return err
	}
	if report != nil && report.Status == model.ErrorReportStatusReady {
		_, advErr := model.AdvanceErrorReportWindow(windowStart)
		return advErr
	}
	ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	view, parts, err := h.buildErrorReport(ctx, windowStart, windowEnd, func(visit func(*model.ErrorEvent) error) (map[int]string, error) {
		return model.WalkErrorEventsForReport(ctx, windowStart, windowEnd, visit)
	})
	if err != nil {
		h.markWindowFailed(windowStart, windowEnd, err)
		return err
	}
	defer parts.file.Close()
	if report == nil {
		report = &model.ErrorReport{
			ReportID:    errorReportID(windowStart),
			WindowStart: windowStart,
			WindowEnd:   windowEnd,
			Timezone:    errorReportLocationName,
			Status:      model.ErrorReportStatusBuilding,
		}
		inserted, err := model.InsertErrorReportBuilding(report)
		if err != nil {
			return err
		}
		if !inserted {
			// 并发节点已创建本窗口报告：按既有报告处理，下一轮换会重新评估。
			existing, lookupErr := model.GetErrorReportByWindowStart(windowStart)
			if lookupErr != nil {
				return lookupErr
			}
			if existing != nil && existing.Status == model.ErrorReportStatusReady {
				_, advErr := model.AdvanceErrorReportWindow(windowStart)
				return advErr
			}
			return fmt.Errorf("concurrent report creation for window %d", windowStart)
		}
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	enabled, currentRecipients, err := model.GetErrorReportConfiguration(ctx)
	if err != nil {
		return err
	}
	if !enabled {
		return context.Canceled
	}
	recipients = currentRecipients
	partNo := 0
	if err := model.PublishErrorReportParts(ctx, view.ReportID, func() (*model.ErrorReportPart, error) {
		partNo++
		return parts.nextPart(view, partNo)
	}, recipients, view.TotalEvents, view.SummaryJSON, view.DataNote); err != nil {
		h.markWindowFailed(windowStart, windowEnd, err)
		return err
	}
	_, err = model.AdvanceErrorReportWindow(windowStart)
	return err
}

// markWindowFailed 把窗口生成失败记录为 failed 报告行（不回退窗口进度），
// 保证故障在系统任务与报告状态中可见，恢复后可重建。
func (h *errorReportHandler) markWindowFailed(windowStart, windowEnd int64, cause error) {
	report := &model.ErrorReport{
		ReportID:    errorReportID(windowStart),
		WindowStart: windowStart,
		WindowEnd:   windowEnd,
		Timezone:    errorReportLocationName,
		Status:      model.ErrorReportStatusBuilding,
	}
	_, err := model.InsertErrorReportBuilding(report)
	if err != nil {
		logger.LogWarn(context.Background(), fmt.Sprintf("error report failed-window record failed: %v", err))
		return
	}
	if err := model.MarkErrorReportFailed(report.ReportID, sanitizeErrorReportError(cause)); err != nil {
		logger.LogWarn(context.Background(), fmt.Sprintf("error report failed-window mark failed: %v", err))
	}
}

// deliverClaims 领取到期投递并逐条发送。一个地址失败不影响其他地址；
// ctx 取消（租约丢失）立即停止后续发送，未写实结果的行按结果不明由
// 过期恢复重试。
func (h *errorReportHandler) deliverClaims(ctx context.Context, runnerID string) (int, int, error) {
	owner := fmt.Sprintf("%s-%s", runnerID, common.GetRandomString(8))
	attempted, accepted := 0, 0
	var failures error
	for i := 0; i < errorReportMaxDeliverPerRun; i++ {
		enabled, recipients, err := model.GetErrorReportConfiguration(ctx)
		if err != nil {
			return attempted, accepted, errors.Join(failures, err)
		}
		if !enabled {
			break
		}
		keep := make(map[string]bool, len(recipients))
		for _, r := range recipients {
			keep[r] = true
		}
		if _, err := model.StopErrorReportDeliveriesForRecipients(keep); err != nil {
			return attempted, accepted, errors.Join(failures, err)
		}
		now := h.now().Unix()
		// Claim just before sending: later items must not spend their lease waiting
		// behind earlier SMTP calls. Every send has a fresh 90-second lease.
		claims, err := model.ClaimErrorReportDeliveries(now, owner, now+int64(errorReportDeliveryLease.Seconds()), 1)
		if err != nil {
			return attempted, accepted, errors.Join(failures, err)
		}
		if len(claims) == 0 {
			break
		}
		claim := claims[0]
		part, sendErr := model.GetErrorReportPartByID(claim.PartID)
		if sendErr == nil && part == nil {
			sendErr = errors.New("report part unavailable")
		}
		var report *model.ErrorReport
		if sendErr == nil {
			report, sendErr = model.GetErrorReportByReportID(claim.ReportID)
			if report == nil && sendErr == nil {
				sendErr = errors.New("report unavailable")
			}
		}
		enabled, recipients, err = model.GetErrorReportConfiguration(ctx)
		if err != nil {
			return attempted, accepted, errors.Join(failures, err)
		}
		if !enabled || !slices.Contains(recipients, claim.Recipient) {
			state, reason := model.ErrorReportDeliveryRetryWait, ""
			if enabled {
				state, reason = model.ErrorReportDeliveryStopped, model.ErrorReportStopRecipientRemoved
			}
			_, err = model.FinishErrorReportDelivery(claim.ID, owner, state, "", reason, now, 0)
			if err != nil {
				return attempted, accepted, errors.Join(failures, err)
			}
			if !enabled {
				break
			}
			continue
		}
		if sendErr == nil {
			subject := part.Subject
			if now > report.WindowEnd+int64(errorReportResendGrace.Seconds()) {
				subject = "[补发] [Resent] " + subject
			}
			attempted++
			sendErr = h.send(ctx, subject, claim.Recipient, part.BodyHTML)
		}
		if ctx.Err() != nil {
			return attempted, accepted, errors.Join(failures, ctx.Err())
		}
		state, next, acceptedAt := model.ErrorReportDeliveryAccepted, int64(0), h.now().Unix()
		message := ""
		if sendErr != nil {
			state, next, acceptedAt = model.ErrorReportDeliveryRetryWait, h.now().Unix()+errorReportBackoff(claim.Attempts), 0
			message = sanitizeErrorReportError(sendErr)
			failures = errors.Join(failures, sendErr)
		}
		ok, err := model.FinishErrorReportDelivery(claim.ID, owner, state, message, "", next, acceptedAt)
		if err != nil {
			failures = errors.Join(failures, err)
		} else if !ok {
			failures = errors.Join(failures, errors.New("投递结果写回失败：租约已变化"))
		} else if sendErr == nil {
			accepted++
		}
	}
	return attempted, accepted, failures
}
