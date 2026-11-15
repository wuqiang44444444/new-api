package service

import (
	"context"
	"fmt"
	"html"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
)

const channelMailLogGrace = time.Minute

func renderChannelStatusMail(content string, at time.Time, channelID int, disabled bool, audit []model.ChannelStatusAudit) string {
	var body strings.Builder
	path, label := "/channels", "打开渠道管理 / Open channel management"
	timeLabel := "恢复时间（北京时间） / Recovered at (Beijing time)"
	if disabled {
		path, label = fmt.Sprintf("/usage-logs/error?channel=%d", channelID), "打开渠道错误日志 / Open channel error logs"
		timeLabel = "停用时间（北京时间） / Disabled at (Beijing time)"
	}
	fmt.Fprintf(&body, "<p>%s</p>", html.EscapeString(content))
	rows := [][2]string{{timeLabel, at.In(errorReportLocation()).Format("2006-01-02 15:04:05")}}
	if len(audit) > 0 {
		if id := safeMailRequestID(audit[0].RequestID); id != "" {
			rows = append(rows, [2]string{"请求 ID / Request ID", id})
		}
	}
	writeOperationsMailRows(&body, rows)
	if link := consoleMailLink(path); link != "" {
		writeReportMailLink(&body, link, label)
	}
	return body.String()
}

func enrichChannelAlertMail(episode *model.ChannelAlertEpisode) {
	if err := common.UnmarshalJsonStr(episode.MailSamplesJSON, &episode.MailSamples); err != nil {
		common.SysError("could not read frozen channel mail observations")
		return
	}
	episode.MailEvidenceHTML = freezeChannelMailEvidence(context.Background(), *episode)
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	db := model.DB.WithContext(ctx).Model(&model.ChannelAlertEpisode{}).Where("id = ?", episode.ID)
	result := db.Where("mail_evidence_html IS NULL OR mail_evidence_html = ''").
		UpdateColumn("mail_evidence_html", episode.MailEvidenceHTML)
	if result.Error == nil && result.RowsAffected == 0 {
		var stored model.ChannelAlertEpisode
		if db.Select("mail_evidence_html").Take(&stored).Error == nil {
			episode.MailEvidenceHTML = stored.MailEvidenceHTML
		}
	}
}

func freezeChannelMailEvidence(ctx context.Context, episode model.ChannelAlertEpisode) string {
	if len(episode.MailSamples) == 0 {
		return ""
	}
	var body strings.Builder
	body.WriteString("<h3>最近计入告警的失败（最多 5 条） / Latest incident failures (up to 5)</h3>")
	requests := make([]string, 0, len(episode.MailSamples))
	for _, row := range episode.MailSamples {
		if row.RequestID != "" {
			requests = append(requests, row.RequestID)
		}
	}
	details := mailErrorDetails{}
	if len(requests) > 0 {
		readCtx, cancel := context.WithTimeout(ctx, 300*time.Millisecond)
		details = loadMailErrorDetails(readCtx, model.MailErrorScope{
			Start: time.UnixMilli(episode.FirstAt).Unix(), End: time.UnixMilli(episode.ConfirmedAt).Add(channelMailLogGrace).Unix() + 1,
			RequestIDs: requests, ChannelID: episode.ChannelID,
		}, nil, false)
		cancel()
	}
	missingLinks := 0
	for _, row := range episode.MailSamples {
		label := time.UnixMilli(row.At).In(errorReportLocation()).Format("01-02 15:04:05")
		fields := make([]string, 0, 4)
		matched := false
		for _, log := range details.rows {
			if log.RequestID != row.RequestID || log.RequestID == "" {
				continue
			}
			if log.UserID > 0 {
				label += " · " + mailAccountLabel(log.UserID, details.accounts[log.UserID], log.Username)
			}
			if route := safeMailText(strings.SplitN(log.Route, "?", 2)[0], 96); route != "" {
				fields = append(fields, strings.TrimSpace(safeMailText(log.Method, 12)+" "+route))
			}
			matched = true
			break
		}
		if !matched && row.RequestID != "" {
			missingLinks++
		}
		if name := safeMailText(episode.Model, 72); name != "" {
			fields = append(fields, name)
		}
		if row.Status > 0 {
			fields = append(fields, "HTTP "+strconv.Itoa(row.Status))
		}
		if id := safeMailRequestID(row.RequestID); id != "" {
			fields = append(fields, "请求 / Request "+id)
		}
		reason := channelAlertErrorReason(row.Reason).mailLabel()
		if reason == "" {
			reason = mailErrorReason(model.MailErrorSample{Status: row.Status})
		}
		writeMailEvidenceCard(&body, label, reason, strings.Join(fields, " · "))
	}
	if details.unavailable {
		body.WriteString("<p>关联错误日志暂不可用。 / Linked error logs are temporarily unavailable.</p>")
	} else if missingLinks > 0 {
		body.WriteString("<p>部分告警样本缺少关联日志。 / Some incident samples have no matching error log.</p>")
	}
	body.WriteString("<p>先按上方渠道和操作核对故障范围，再查看对应模型的错误日志。<br>Check the channel and operation above, then review error logs for the affected model.</p>")
	return body.String()
}
