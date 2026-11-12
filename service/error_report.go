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

// Shared scheduling, publication result and delivery for the daily report.

const (
	// errorReportCheckInterval 是调度间隔；内部按绝对时间计算窗口，
	// 不以「上次发送结束加一小时」定义统计范围。
	errorReportCheckInterval = time.Minute

	errorReportLocationName = "Asia/Shanghai"

	// 每轮有限工作量：大量积压或故障邮箱不会长期占用租约，未完成进度可续跑。
	errorReportMaxDeliverPerRun = 30
	errorReportMaxRequeuePerRun = 200

	// A lost lease has an unknown submission outcome.
	errorReportDeliveryLease = 90 * time.Second
)

type errorReportHandler struct {
	send mailSendFunc
	now  func() time.Time
}

func newErrorReportHandler(send mailSendFunc) *errorReportHandler {
	return &errorReportHandler{send: send, now: time.Now}
}

func (errorReportHandler) Type() string { return model.SystemTaskTypeErrorReport }

// The scheduler needs a destination, without a separate report switch.
func (errorReportHandler) Enabled() bool {
	return len(operation_setting.ParseErrorReportRecipients(operation_setting.ErrorReportSettingSnapshot().Recipients)) > 0
}

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

func (h *errorReportHandler) Run(ctx context.Context, task *model.SystemTask, runnerID string) {
	recipients, err := model.GetOperationsMailRecipients(ctx)
	if err != nil {
		failSystemTask(task, runnerID, err)
		return
	}
	if len(recipients) == 0 {
		finishErrorReportTask(task, runnerID, errorReportTaskResult{Note: "no recipients"}, nil)
		return
	}
	result := errorReportTaskResult{}
	generated, failedWindows, genErr := h.generateDailyReport(ctx)
	result.GeneratedWindows = generated
	result.FailedWindows = failedWindows
	if genErr != nil {
		logger.LogWarn(ctx, fmt.Sprintf("error report generation pass failed: %v", genErr))
	}

	expired, err := model.MarkExpiredErrorReportDeliveriesUnknown(h.now().Unix(), errorReportMaxRequeuePerRun)
	if err != nil {
		finishErrorReportTask(task, runnerID, result, err)
		return
	}
	result.ExpiredUnknown = expired
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
	ExpiredUnknown        int64    `json:"expired_unknown"`
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
	if runErr != nil || result.BacklogDeliveries > 0 || len(result.FailedWindows) > 0 || result.ExpiredUnknown > 0 {
		message = fmt.Sprintf("生成失败 %d 个窗口；投递 %d/%d 受理；积压 %d 封；租约过期结果待核对 %d 封。%s", len(result.FailedWindows), result.DeliverAccepted, result.DeliverAttempted, result.BacklogDeliveries, result.ExpiredUnknown, message)
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

// Claim immediately before each submission so later messages never wait with a running lease.
func (h *errorReportHandler) deliverClaims(ctx context.Context, runnerID string) (int, int, error) {
	owner := fmt.Sprintf("%s-%s", runnerID, common.GetRandomString(8))
	attempted, accepted := 0, 0
	var failures error
	for i := 0; i < errorReportMaxDeliverPerRun; i++ {
		recipients, err := model.GetOperationsMailRecipients(ctx)
		if err != nil {
			return attempted, accepted, errors.Join(failures, err)
		}
		if len(recipients) == 0 {
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
		recipients, err = model.GetOperationsMailRecipients(ctx)
		if err != nil {
			_, finishErr := model.FinishErrorReportDelivery(claim.ID, owner, model.MailDeliveryRetryWait, "", "", now+60, 0)
			return attempted, accepted, errors.Join(failures, err, finishErr)
		}
		if !slices.Contains(recipients, claim.Recipient) {
			if len(recipients) == 0 {
				_, err = model.FinishErrorReportDelivery(claim.ID, owner, model.MailDeliveryRetryWait, "", "", now, 0)
				return attempted, accepted, errors.Join(failures, err)
			}
			_, err = model.FinishErrorReportDelivery(claim.ID, owner, model.MailDeliveryStopped, "", model.ErrorReportStopRecipientRemoved, now, 0)
			if err != nil {
				return attempted, accepted, errors.Join(failures, err)
			}
			continue
		}
		if sendErr == nil {
			attempted++
			sendErr = h.send(ctx, part.Subject, claim.Recipient, part.BodyHTML)
		}
		state, next, acceptedAt := model.MailDeliveryAccepted, int64(0), h.now().Unix()
		message := ""
		if sendErr != nil {
			state, acceptedAt = model.MailDeliveryRetryWait, 0
			if errors.Is(sendErr, common.ErrMailSubmissionUnknown) {
				state = model.MailDeliveryUnknown
			} else {
				next = h.now().Unix() + mailDeliveryBackoff(claim.Attempts)
			}
			message = sanitizeErrorReportError(sendErr)
			failures = errors.Join(failures, sendErr)
		}
		// A canceled task must still record the known outcome of its last submission.
		ok, err := model.FinishErrorReportDelivery(claim.ID, owner, state, message, "", next, acceptedAt)
		if err != nil {
			failures = errors.Join(failures, err)
		} else if !ok {
			failures = errors.Join(failures, errors.New("投递结果写回失败：租约已变化"))
		} else if sendErr == nil {
			accepted++
		}
		if ctx.Err() != nil {
			return attempted, accepted, errors.Join(failures, ctx.Err())
		}
	}
	return attempted, accepted, failures
}
