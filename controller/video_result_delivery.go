package controller

import (
	"net/http"
	"strings"

	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
)

// Link 视频结果托管交付（本地扩展）。范围内任务的公共投影与内容入口统一
// 返回/重定向本站 OSS 预签名地址；准备中与不可用按唯一公共合同表达，不
// 下发上游直链，不回退旧代理。

// 视频结果托管的唯一公共交付合同（§3.2：实现前落实到唯一公共合同）。
const (
	videoResultDeliveryReady       = "ready"
	videoResultDeliveryPreparing   = "preparing"
	videoResultDeliveryUnavailable = "unavailable"
)

// videoResultDeliveryParts 汇总一个任务的主视频与尾帧交付状态。
type videoResultDeliveryParts struct {
	status       string
	videoURL     string
	frameURL     string
	framePresent bool
}

// resolveVideoResultDelivery 汇总范围内任务的交付状态与签名地址。存储禁用
// 或对象缺失按合同失败关闭；签名 URL 只在本次响应内使用，不落日志。
func resolveVideoResultDelivery(c *gin.Context, task *model.Task) videoResultDeliveryParts {
	parts := videoResultDeliveryParts{status: videoResultDeliveryPreparing}
	if !service.VideoResultTransferScoped(task) {
		parts.status = ""
		return parts
	}
	rows, err := model.ListVideoResultObjects(task.TaskID)
	if err != nil {
		parts.status = videoResultDeliveryPreparing
		return parts
	}
	if len(rows) == 0 {
		// 登记缺口补扫窗口之外的成功任务不属于自动迁移范围，如实报告不可用。
		if task.FinishTime < service.VideoResultSweepWindowStart() {
			parts.status = videoResultDeliveryUnavailable
		}
		return parts
	}
	main := model.VideoResultReadyObject(rows, model.TaskVideoResultPartVideo)
	if main == nil {
		mainPresent := false
		for i := range rows {
			if rows[i].Part == model.TaskVideoResultPartVideo {
				mainPresent = true
				if rows[i].Status == model.TaskVideoResultStatusUnavailable {
					parts.status = videoResultDeliveryUnavailable
				}
				break
			}
		}
		if !mainPresent && task.FinishTime < service.VideoResultSweepWindowStart() {
			parts.status = videoResultDeliveryUnavailable
		}
		return parts
	}
	url, _, err := service.PresignVideoResultDownload(c.Request.Context(), main, http.MethodGet)
	if err != nil {
		parts.status = videoResultDeliveryPreparing
		return parts
	}
	parts.status = videoResultDeliveryReady
	parts.videoURL = url
	frame := model.VideoResultReadyObject(rows, model.TaskVideoResultPartLastFrame)
	if frame == nil {
		return parts
	}
	frameURL, _, err := service.PresignVideoResultDownload(c.Request.Context(), frame, http.MethodGet)
	if err != nil {
		return parts
	}
	parts.frameURL = frameURL
	parts.framePresent = true
	return parts
}

// applyVideoResultDelivery 把托管交付状态写入 ModelArk 投影：ready 时填充
// 本站预签名地址，preparing/unavailable 时省略地址并标记交付状态。
func applyVideoResultDelivery(c *gin.Context, task *model.Task, projected *dto.ModelArkVideoTask) {
	if projected == nil || projected.Content == nil {
		return
	}
	parts := resolveVideoResultDelivery(c, task)
	if parts.status == "" {
		return
	}
	projected.Content.DeliveryStatus = parts.status
	if parts.status != videoResultDeliveryReady {
		projected.Content.VideoURL = ""
		projected.Content.LastFrameURL = ""
		return
	}
	projected.Content.VideoURL = parts.videoURL
	if parts.framePresent {
		projected.Content.LastFrameURL = parts.frameURL
	} else {
		projected.Content.LastFrameURL = ""
	}
}

// serveVideoResultDelivery 处理范围内任务的内容请求：就绪时授权后 302 重定
// 向本站预签名地址（网关不再逐次传输字节）；准备中/不可用返回明确交付错误。
// 返回 true 表示已接管响应。
func serveVideoResultDelivery(c *gin.Context, task *model.Task) bool {
	if !service.VideoResultTransferScoped(task) {
		return false
	}
	contentPart := strings.TrimSpace(c.Query("part"))
	if contentPart != "" && contentPart != model.TaskVideoResultPartLastFrame {
		modelArkVideoError(c, http.StatusBadRequest, "invalid_content_part", "Content part is not supported")
		return true
	}
	rows, err := model.ListVideoResultObjects(task.TaskID)
	if err != nil {
		modelArkVideoError(c, http.StatusInternalServerError, "internal_error", "Failed to query video delivery")
		return true
	}
	part := model.TaskVideoResultPartVideo
	if contentPart == model.TaskVideoResultPartLastFrame {
		part = model.TaskVideoResultPartLastFrame
	}
	row := model.VideoResultReadyObject(rows, part)
	if row != nil {
		url, _, signErr := service.PresignVideoResultDownload(c.Request.Context(), row, c.Request.Method)
		if signErr != nil {
			modelArkVideoError(c, http.StatusServiceUnavailable, "video_delivery_unavailable", "Video delivery storage is unavailable")
			return true
		}
		// 客户端直连对象存储下载；签名 URL 不落日志（§3.3）。
		c.Redirect(http.StatusFound, url)
		return true
	}
	partPresent := false
	for i := range rows {
		if rows[i].Part != part {
			continue
		}
		partPresent = true
		if rows[i].Status == model.TaskVideoResultStatusUnavailable {
			modelArkVideoError(c, http.StatusGone, "video_delivery_unavailable", "Video delivery is unavailable")
			return true
		}
		break
	}
	if !partPresent {
		expected := false
		for _, knownPart := range model.VideoTaskResultParts(task) {
			if part == knownPart {
				expected = true
				break
			}
		}
		if !expected {
			modelArkVideoError(c, http.StatusNotFound, "content_not_found", "Requested content is not available")
			return true
		}
		if task.FinishTime < service.VideoResultSweepWindowStart() {
			modelArkVideoError(c, http.StatusGone, "video_delivery_unavailable", "Video delivery is unavailable")
			return true
		}
	}
	modelArkVideoError(c, http.StatusConflict, "video_delivery_preparing", "Video result is being prepared for download")
	return true
}
