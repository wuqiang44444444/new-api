package service

import (
	"context"
	"crypto/sha256"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/model"
)

// NotifyEvidenceCleanupHealth records one site-wide incident until a healthy
// check closes it. Reuse the resource alert outbox and wake its worker now.
func NotifyEvidenceCleanupHealth(ctx context.Context, at int64, health *model.EvidenceCleanupHealth, checkFailed bool) error {
	if err := recordEvidenceCleanupHealth(ctx, at, health, checkFailed); err != nil {
		return err
	}
	queueResourceAlertDeliveries()
	return nil
}

func recordEvidenceCleanupHealth(ctx context.Context, at int64, health *model.EvidenceCleanupHealth, checkFailed bool) error {
	key := fmt.Sprintf("%x", sha256.Sum256([]byte(model.EvidenceCleanupAlertResource)))
	if !checkFailed && len(health.Alerts) == 0 {
		return model.CloseResourceAlert(ctx, key, at)
	}

	const title = "证据清理异常 / Evidence cleanup alert"
	rows := [][2]string{{"检查时间 / Checked at (UTC+8)", time.Unix(at, 0).In(errorReportLocation()).Format("2006-01-02 15:04:05")}}
	if checkFailed {
		rows = append(rows, [2]string{"检查结果 / Check result", "无法核验证据清理状态 / Could not verify cleanup health"})
	} else {
		rows = append(rows, [2]string{"异常 / Conditions", strings.Join(health.Alerts, ", ")},
			[2]string{"连续失败 / Consecutive failures", strconv.Itoa(health.ConsecutiveFailures)},
			[2]string{"待清理索引 / Pending indexes", strconv.FormatInt(health.PendingIndexes, 10)},
			[2]string{"待回收数据库索引 / Pending database indexes", strconv.FormatInt(health.PendingDatabaseIndexes, 10)})
		if health.LastFailedTaskID != "" {
			rows = append(rows, [2]string{"最近失败任务 / Latest failed task", safeMailRequestID(health.LastFailedTaskID)})
		}
		if health.LastSucceededAt > 0 {
			rows = append(rows, [2]string{"最近成功 / Last success (UTC+8)", time.Unix(health.LastSucceededAt, 0).In(errorReportLocation()).Format("2006-01-02 15:04:05")})
		}
		if health.FreeBytes != nil {
			rows = append(rows, [2]string{"剩余磁盘空间 / Free space", fmt.Sprintf("%.1f GiB", float64(*health.FreeBytes)/(1<<30))})
		}
		if health.FreeInodes != nil {
			rows = append(rows, [2]string{"剩余 inode / Free inodes", strconv.FormatUint(*health.FreeInodes, 10)})
		}
	}
	var body strings.Builder
	body.WriteString("<p>请在系统信息的系统任务中核对证据清理监控与最近一次清理结果，并检查证据存储卷的容量和 inode。<br>Review the evidence cleanup monitor and the latest cleanup run in System Tasks, then check free space and inodes on the evidence volume.</p>")
	writeOperationsMailRows(&body, rows)
	body.WriteString("<p>系统任务页面仅限 Root。交接时请提供上面的任务编号或检查时间。<br>System Tasks requires Root access. Include the task ID or check time when escalating.</p>")
	if link := consoleMailLink("/system-info"); link != "" {
		writeReportMailLink(&body, link, "打开系统任务（Root） / Open system tasks (Root)")
	}
	_, err := model.OpenResourceAlert(ctx, model.ResourceAlertEpisode{
		OpenKey: &key, Resource: model.EvidenceCleanupAlertResource, OpenedAt: at,
		Subject: title, BodyHTML: RenderMailTemplate(title, body.String()),
	}, nil)
	return err
}
