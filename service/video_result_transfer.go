package service

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
)

// Link 视频结果托管转存 worker（本地扩展）。成功任务的结果先持久登记，
// 后台及时领取并按冻结协议与连接读取上游完整结果文件，有界流式上传本站
// 私有 OSS，提交对象登记事实。重试只针对同一任务、同一结果的读取或上传；
// 不重发生成 POST、不换 Provider、不重复扣费，不因转存失败分类错误自动退款。

const (
	videoResultTransferBatchSize = 4
	videoResultTransferClaimTTL  = 30 * time.Minute
	// videoResultTransferMaxAttempts 是单部件累计领取上限；用尽后如实标记
	// 交付不可用，不回退旧代理。
	videoResultTransferMaxAttempts = 8
	// videoResultTransferTimeout 是单部件完整转存（读取+上传）的超时上限。
	videoResultTransferTimeout = 15 * time.Minute
	// videoResultSweepWindow 是登记缺口补扫窗口（默认 24 小时），只做崩溃
	// 恢复；窗口外的存量任务不自动迁移，范围由部署方上线前盘点决定。
	videoResultSweepWindow = 24 * time.Hour
)

// videoResultMaxBytes 是单文件托管大小上限（§4.2 待运营确认的默认值）；
// 用包级变量承载，测试可缩小以验证有界读取。
var videoResultMaxBytes = int64(1) << 30

// videoResultRetryBackoffs 是按累计尝试次数递增的重试退避。
var videoResultRetryBackoffs = []time.Duration{
	30 * time.Second, 2 * time.Minute, 5 * time.Minute,
	15 * time.Minute, 30 * time.Minute, 1 * time.Hour, 2 * time.Hour,
}

// ResolveMiniMaxVideoContentFunc 打破 service -> relay/channel/task/minimax
// 的循环依赖（该包反过来依赖 service）；由 main 注入，未注入视为临时不可用
// 按退避重试。
var ResolveMiniMaxVideoContentFunc func(ctx context.Context, task *model.Task) (string, error)

// VideoResultTransferSummary 汇报一轮转存执行结果，作为 system task result。
type VideoResultTransferSummary struct {
	Attempted   int `json:"attempted"`
	Ready       int `json:"ready"`
	Failed      int `json:"failed"`
	Unavailable int `json:"unavailable"`
	Swept       int `json:"swept"`
}

// VideoResultTransferScoped 报告任务是否属于 Link 视频结果托管范围
// （Seedance Link 与 MiniMax Link 的 ModelArk V3 客户协议任务）。
func VideoResultTransferScoped(task *model.Task) bool {
	if task == nil || task.ClientProtocol != model.TaskClientProtocolModelArkV3 {
		return false
	}
	switch task.Platform {
	case constant.TaskPlatform(strconv.Itoa(constant.ChannelTypeSeedanceLink)),
		constant.TaskPlatform(strconv.Itoa(constant.ChannelTypeMiniMaxLink)):
		return true
	default:
		return false
	}
}

// RegisterVideoResultTransfer 在可信生成成功时登记待转存结果；非范围内
// 任务是 no-op。登记失败不改变成功事实，由登记缺口补扫恢复。
func RegisterVideoResultTransfer(ctx context.Context, task *model.Task) {
	if !VideoResultTransferScoped(task) {
		return
	}
	if task.Status != model.TaskStatusSuccess {
		return
	}
	durable, exists, err := model.GetUniqueByOnlyTaskId(task.TaskID)
	if err != nil {
		logger.LogWarn(ctx, "video result durable task lookup failed")
		return
	}
	if !exists || !VideoResultTransferScoped(durable) || durable.Status != model.TaskStatusSuccess || durable.ClientDeletedAt != 0 {
		return
	}
	if err := model.RegisterVideoResultObjects(durable, model.VideoTaskResultParts(durable)); err != nil {
		logger.LogWarn(ctx, fmt.Sprintf("video result registration failed for task %s: %s", task.TaskID, err.Error()))
	}
}

// runVideoResultSweep 补扫登记缺口：完成窗口内成功且缺少实际结果部件的范围内任务
// 重新登记。窗口之外的存量任务不自动迁移（上线前盘点决定范围）。
func runVideoResultSweep(ctx context.Context) int {
	windowStart := common.GetTimestamp() - int64(videoResultSweepWindow.Seconds())
	platforms := []constant.TaskPlatform{
		constant.TaskPlatform(strconv.Itoa(constant.ChannelTypeSeedanceLink)),
		constant.TaskPlatform(strconv.Itoa(constant.ChannelTypeMiniMaxLink)),
	}
	gaps, err := model.FindVideoResultRegistrationGaps(platforms, windowStart, 200)
	if err != nil {
		logger.LogWarn(ctx, "video result registration sweep query failed: "+err.Error())
		return 0
	}
	registered := 0
	for i := range gaps {
		if err := model.RegisterVideoResultObjects(&gaps[i], model.VideoTaskResultParts(&gaps[i])); err != nil {
			logger.LogWarn(ctx, fmt.Sprintf("video result sweep registration failed for task %s: %s", gaps[i].TaskID, err.Error()))
			continue
		}
		registered++
	}
	return registered
}

// RunVideoResultTransferPass 执行一轮转存：先补扫登记缺口，再领取并逐个
// 转存。单部件失败不影响同批其他部件；进程崩溃后由领取租约过期恢复。
func RunVideoResultTransferPass(ctx context.Context, report func(processed, total int)) VideoResultTransferSummary {
	summary := VideoResultTransferSummary{}
	if ctx == nil {
		ctx = context.Background()
	}
	storeReady := VideoResultDeliveryAvailable(ctx)
	summary.Swept = runVideoResultSweep(ctx)
	rows, err := model.ClaimVideoResultObjects(
		videoResultTransferBatchSize, videoResultRunnerID(), common.GetTimestamp(), int64(videoResultTransferClaimTTL.Seconds()),
	)
	if err != nil {
		logger.LogWarn(ctx, "video result transfer claim failed: "+err.Error())
		return summary
	}
	total := len(rows)
	for i := range rows {
		if ctx.Err() != nil {
			return summary
		}
		if report != nil {
			report(i, total)
		}
		if !storeReady {
			// 存储不可用是可重试状态：退避重试，不下发上游直链，不回退旧代理。
			rescheduleVideoResultRetry(&rows[i], "object storage unavailable")
			summary.Failed++
			continue
		}
		switch transferVideoResultObject(ctx, &rows[i]) {
		case transferReady:
			summary.Ready++
		case transferRetryable:
			summary.Failed++
		case transferUnavailable:
			summary.Unavailable++
		}
		summary.Attempted++
	}
	if report != nil && ctx.Err() == nil {
		report(total, total)
	}
	return summary
}

// videoResultRunnerID 标识本轮领取者，与 claim 租约列配合完成崩溃恢复。
func videoResultRunnerID() string {
	random, err := common.GenerateRandomCharsKey(8)
	if err != nil {
		return common.NodeName
	}
	return common.NodeName + "-" + random
}

// videoResultRetryAt 按累计尝试次数取退避时间；用尽尝试次数后返回 0，由
// 调用方改为标记交付不可用。
func videoResultRetryAt(attempts int) int64 {
	if attempts >= videoResultTransferMaxAttempts {
		return 0
	}
	index := attempts - 1
	if index < 0 {
		index = 0
	}
	if index >= len(videoResultRetryBackoffs) {
		index = len(videoResultRetryBackoffs) - 1
	}
	return common.GetTimestamp() + int64(videoResultRetryBackoffs[index].Seconds())
}

// transferOutcome 区分三类可观察结果。
type transferOutcome int

const (
	transferReady transferOutcome = iota
	transferRetryable
	transferUnavailable
)

// rescheduleVideoResultRetry 退回 pending 等待下次领取；用尽尝试次数时改为
// 如实标记交付不可用。
func rescheduleVideoResultRetry(row *model.TaskVideoResultObject, reason string) {
	next := videoResultRetryAt(row.Attempts)
	if next == 0 {
		commitVideoResultUnavailable(row, reason)
		return
	}
	if _, err := model.RescheduleVideoResultObjectRetry(row.ID, row.ClaimedBy, next, reason, common.GetTimestamp()); err != nil {
		logger.LogWarn(context.Background(), fmt.Sprintf("video result retry reschedule failed for task %s: %s", row.TaskID, err.Error()))
	}
}

func commitVideoResultUnavailable(row *model.TaskVideoResultObject, reason string) transferOutcome {
	if _, err := model.FinishVideoResultObjectUnavailable(row.ID, row.ClaimedBy, reason, common.GetTimestamp()); err != nil {
		logger.LogWarn(context.Background(), fmt.Sprintf("video result unavailable commit failed for task %s: %s", row.TaskID, err.Error()))
	}
	return transferUnavailable
}

// transferVideoResultObject 完成一个部件的读取与上传，并提交对象登记事实。
// 对象位置由平台生成（task + part），失败不产生新对象身份；同一对象键重复
// 上传是幂等覆盖，配合登记事实满足崩溃恢复，不反复生成新对象。
func transferVideoResultObject(ctx context.Context, row *model.TaskVideoResultObject) transferOutcome {
	task, exists, err := model.GetUniqueByOnlyTaskId(row.TaskID)
	if err != nil || !exists || task == nil {
		rescheduleVideoResultRetry(row, "task lookup failed")
		return transferRetryable
	}
	if task.ClientDeletedAt != 0 {
		return commitVideoResultUnavailable(row, "task deleted")
	}
	if task.Status != model.TaskStatusSuccess {
		rescheduleVideoResultRetry(row, "task is not successful yet")
		return transferRetryable
	}
	objectKey, err := videoResultObjectKey(task.TaskID, row.Part)
	if err != nil {
		rescheduleVideoResultRetry(row, "object key derivation failed")
		return transferRetryable
	}
	source, err := resolveVideoResultSource(ctx, task, row.Part)
	if err != nil {
		return commitVideoResultFailure(ctx, row, err)
	}
	defer func() { _ = source.close() }()
	store, err := currentVideoResultObjectStore(ctx)
	if err != nil {
		rescheduleVideoResultRetry(row, "object storage unavailable")
		return transferRetryable
	}
	storeCtx, cancel := context.WithTimeout(ctx, videoResultTransferTimeout)
	defer cancel()
	mimeType := videoResultMimeTypeForPart(row.Part, source.contentType)
	size, err := store.putVideoResultObject(storeCtx, objectKey, mimeType, source.body, videoResultMaxBytes)
	if err != nil {
		return commitVideoResultFailure(ctx, row, err)
	}
	won, err := model.FinishVideoResultObjectReady(row.ID, row.ClaimedBy, store.videoResultLocation(), objectKey, mimeType, size, common.GetTimestamp())
	if err != nil || !won {
		logger.LogWarn(ctx, fmt.Sprintf("video result ready commit failed for task %s part %s", row.TaskID, row.Part))
		return transferRetryable
	}
	return transferReady
}

// commitVideoResultFailure 把一次读取/上传失败归类为可重试或交付不可用。
func commitVideoResultFailure(ctx context.Context, row *model.TaskVideoResultObject, err error) transferOutcome {
	if errors.Is(err, errVideoResultTooLarge) || errors.Is(err, errVideoResultUnavailable) {
		if ctx != nil && errors.Is(err, errVideoResultTooLarge) {
			logger.LogWarn(ctx, fmt.Sprintf("video result exceeds size limit for task %s part %s", row.TaskID, row.Part))
		}
		return commitVideoResultUnavailable(row, videoResultSanitizedReason(err))
	}
	rescheduleVideoResultRetry(row, videoResultSanitizedReason(err))
	return transferRetryable
}

// videoResultSanitizedReason 收敛失败描述：不含上游 URL、凭据或响应正文。
func videoResultSanitizedReason(err error) string {
	reason := "video result transfer failed"
	if err != nil {
		message := err.Error()
		if len(message) > 200 {
			message = message[:200]
		}
		if message != "" {
			reason = reason + ": " + message
		}
	}
	return common.SanitizeTaskDiagnostic(reason)
}

// videoResultTransferHandler 是注册进 system task 框架的定期转存任务：DB
// lease 去重执行 + 行级领取租约双保险，多节点并发领取安全。
type videoResultTransferHandler struct{}

func (videoResultTransferHandler) Type() string { return model.SystemTaskTypeVideoResultTransfer }

func init() {
	RegisterSystemTaskHandler(videoResultTransferHandler{})
}

func (videoResultTransferHandler) Enabled() bool {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if !VideoResultDeliveryAvailable(ctx) {
		return false
	}
	count, err := model.CountVideoResultTransferWork(common.GetTimestamp())
	if err != nil {
		return false
	}
	if count > 0 {
		return true
	}
	gaps, err := model.FindVideoResultRegistrationGaps([]constant.TaskPlatform{
		constant.TaskPlatform(strconv.Itoa(constant.ChannelTypeSeedanceLink)),
		constant.TaskPlatform(strconv.Itoa(constant.ChannelTypeMiniMaxLink)),
	}, VideoResultSweepWindowStart(), 1)
	return err == nil && len(gaps) > 0
}

func (videoResultTransferHandler) Interval() time.Duration { return 15 * time.Second }

func (videoResultTransferHandler) NewPayload() any { return nil }

func (videoResultTransferHandler) Run(ctx context.Context, task *model.SystemTask, runnerID string) {
	summary := RunVideoResultTransferPass(ctx, NewSystemTaskProgressReporter(task, runnerID))
	if err := model.FinishSystemTask(task.TaskID, runnerID, model.SystemTaskStatusSucceeded, summary, ""); err != nil {
		logSystemTaskLockError(ctx, task, err)
	}
}

// VideoResultSweepWindowStart 返回登记补扫窗口起点（unix 秒）。窗口外的
// 成功任务不属于自动托管范围，公开表达按交付不可用处理。
func VideoResultSweepWindowStart() int64 {
	return common.GetTimestamp() - int64(videoResultSweepWindow.Seconds())
}
