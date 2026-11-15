package service

import (
	"cmp"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/i18n"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relaykit/dto"
)

type ChannelProbeMailFailure struct {
	ChannelID, Status int
	Model, Reason     string
	At                time.Time
}

func LatestChannelProbeFailures(previous, recent []ChannelProbeMailFailure) []ChannelProbeMailFailure {
	all := append(previous, recent...)
	slices.SortFunc(all, func(a, b ChannelProbeMailFailure) int {
		if order := b.At.Compare(a.At); order != 0 {
			return order
		}
		return cmp.Compare(b.ChannelID, a.ChannelID)
	})
	return all[:min(len(all), model.MailErrorLimit)]
}

func NotifyChannelProbeRun(at time.Time, tested, passed, failed, unsupported int, samples []ChannelProbeMailFailure) {
	lang := RootUserLanguage()
	subject := i18n.Translate(lang, i18n.MsgChannelTestCompletedSubject)
	var body strings.Builder
	writeOperationsMailRows(&body, [][2]string{
		{"完成时间（北京时间） / Finished at (Beijing time)", at.In(errorReportLocation()).Format("2006-01-02 15:04:05")},
		{"已检查 / Checked", strconv.Itoa(tested)},
		{"通过 / Passed", strconv.Itoa(passed)},
		{"失败 / Failed", strconv.Itoa(failed)},
		{"不支持 / Unsupported", strconv.Itoa(unsupported)},
	})
	if len(samples) > 0 {
		body.WriteString("<h3>本次最近失败（最多 5 条） / Latest failures in this run (up to 5)</h3>")
		for _, sample := range samples {
			label := fmt.Sprintf("%s · #%d", sample.At.In(errorReportLocation()).Format("01-02 15:04:05"), sample.ChannelID)
			modelName := safeMailText(sample.Model, 72)
			if sample.Status >= 400 {
				modelName = strings.TrimSpace(modelName + fmt.Sprintf(" HTTP %d", sample.Status))
			}
			reason := "本次渠道测试失败 / Channel test failed in this run"
			switch sample.Reason {
			case ChannelTestReasonResponseTimeExceeded:
				reason = "超过响应时间阈值 / Response exceeded the time threshold"
			case "test_config_error", "test_azure_batch_connection_failed", "test_request_build_failed":
				reason = "渠道配置或请求构建失败 / Channel configuration or request build failed"
			case "test_upstream_unreachable":
				reason = "无法连接上游 / Could not reach upstream"
			case "test_upstream_rejected":
				reason = "上游拒绝测试请求 / Upstream rejected the probe"
			case "asset_probe_failed":
				reason = "素材探针失败 / Asset probe failed"
			}
			writeMailEvidenceCard(&body, label, reason, modelName)
		}
		body.WriteString("<p>请优先核对失败渠道及模型的配置。<br>Review the configuration of the failed channels and models first.</p>")
	}
	if link := consoleMailLink("/channels"); link != "" {
		writeReportMailLink(&body, link, "打开渠道管理 / Open channel management")
	}
	notifyRootUser(model.MailChannelTest, dto.NotifyTypeChannelTest, subject, i18n.Translate(lang, i18n.MsgChannelTestCompletedContent), body.String())
}
