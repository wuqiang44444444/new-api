package controller

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
)

// 音视频证据查询（一期）。管理员查看脱敏业务证据（列表/详情/预览），
// Root 通过专用查看接口读取原文、下载完整对象。审计在实际操作入口分别记录：
// 脱敏预览 view/redacted、原文查看 view/original、下载 download/original；
// 列表与详情不返回原文。响应由路由组中间件禁止缓存。

func GetTaskRequestEvidenceList(c *gin.Context) {
	requestID := strings.TrimSpace(c.Query("request_id"))
	taskID := strings.TrimSpace(c.Query("task_id"))
	upstreamRequestID := strings.TrimSpace(c.Query("upstream_request_id"))
	if requestID == "" && taskID == "" && upstreamRequestID == "" {
		common.ApiErrorMsg(c, "request_id or task_id is required")
		return
	}
	pageInfo := common.GetPageQuery(c)
	items, total := model.QueryTaskRequestEvidence(model.TaskRequestEvidenceQueryParams{
		RequestID:         requestID,
		UpstreamRequestID: upstreamRequestID,
		TaskID:            taskID,
		Kind:              c.Query("kind"),
		StartIdx:          pageInfo.GetStartIdx(),
		Num:               pageInfo.GetPageSize(),
	})
	results := make([]gin.H, 0, len(items))
	for _, evidence := range items {
		results = append(results, evidenceIndexView(evidence, isRootViewer(c)))
	}
	pageInfo.SetTotal(int(total))
	pageInfo.SetItems(results)
	common.ApiSuccess(c, pageInfo)
}

func GetTaskRequestEvidenceDetail(c *gin.Context) {
	evidence, ok := loadEvidenceForViewer(c)
	if !ok {
		return
	}
	model.RecordTaskRequestEvidenceAccess(evidence.Id, int64(c.GetInt("id")), "view", "redacted")
	events, err := model.ListTaskRequestEvidenceEvents(evidence.Id)
	if err != nil {
		common.ApiErrorMsg(c, "Failed to query evidence events")
		return
	}
	isRoot := isRootViewer(c)
	previews := service.GetEvidenceEventPreviews(events, evidence.BodyExpired)
	eventViews := make([]gin.H, 0, len(events))
	for _, event := range events {
		eventViews = append(eventViews, evidenceEventView(event, previews[event.Id]))
	}
	common.ApiSuccess(c, gin.H{
		"evidence": evidenceIndexView(evidence, isRoot),
		"events":   eventViews,
	})
}

func GetTaskRequestEvidenceObject(c *gin.Context) {
	evidence, ok := loadEvidenceForViewer(c)
	if !ok {
		return
	}
	eventId, _ := strconv.ParseInt(c.Param("event_id"), 10, 64)
	event, exists, err := model.GetTaskRequestEvidenceEvent(evidence.Id, eventId)
	if err != nil || !exists || event == nil {
		c.JSON(http.StatusNotFound, gin.H{"success": false, "message": "Evidence event not found"})
		return
	}
	payload, bodyStatus := service.ReadEvidenceEventBody(event, evidence.BodyExpired)
	if !respondEvidenceBodyStatus(c, bodyStatus) {
		return
	}
	model.RecordTaskRequestEvidenceAccess(evidence.Id, int64(c.GetInt("id")), "download", "original")
	filename := strconv.FormatInt(event.Id, 10) + ".bin"
	c.Header("Content-Disposition", "attachment; filename=\""+filename+"\"")
	c.Data(http.StatusOK, "application/octet-stream", payload)
}

// GetTaskRequestEvidenceContent 按 Root 显式查看返回事件正文原文（内联文本）。
// 与下载共用读取服务；仅在实际交付原文时记录 view/original 审计。
func GetTaskRequestEvidenceContent(c *gin.Context) {
	evidence, ok := loadEvidenceForViewer(c)
	if !ok {
		return
	}
	eventId, _ := strconv.ParseInt(c.Param("event_id"), 10, 64)
	event, exists, err := model.GetTaskRequestEvidenceEvent(evidence.Id, eventId)
	if err != nil || !exists || event == nil {
		c.JSON(http.StatusNotFound, gin.H{"success": false, "message": "Evidence event not found"})
		return
	}
	payload, bodyStatus := service.ReadEvidenceEventBody(event, evidence.BodyExpired)
	if !respondEvidenceBodyStatus(c, bodyStatus) {
		return
	}
	model.RecordTaskRequestEvidenceAccess(evidence.Id, int64(c.GetInt("id")), "view", "original")
	c.Header("X-Content-Type-Options", "nosniff")
	c.Data(http.StatusOK, "text/plain; charset=utf-8", payload)
}

// respondEvidenceBodyStatus 在正文不可读时按既有语义返回明确错误；
// 返回 true 表示正文可用、调用方继续交付。
func respondEvidenceBodyStatus(c *gin.Context, bodyStatus string) bool {
	if bodyStatus == "available" {
		return true
	}
	status, message := http.StatusGone, "Evidence body is no longer available"
	if bodyStatus == "expired" || bodyStatus == "not_recorded" {
		message = "Evidence body expired"
	}
	if bodyStatus == "storage_unavailable" {
		status, message = http.StatusServiceUnavailable, "Evidence storage is not enabled"
	}
	c.JSON(status, gin.H{"success": false, "message": message, "body_status": bodyStatus})
	return false
}

func loadEvidenceForViewer(c *gin.Context) (*model.TaskRequestEvidence, bool) {
	evidenceId, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	evidence, exists, err := model.GetTaskRequestEvidenceById(evidenceId)
	if err != nil || !exists || evidence == nil {
		c.JSON(http.StatusNotFound, gin.H{"success": false, "message": "Evidence not found"})
		return nil, false
	}
	return evidence, true
}

func isRootViewer(c *gin.Context) bool {
	return c.GetInt("role") >= common.RoleRootUser
}

func evidenceIndexView(evidence *model.TaskRequestEvidence, includeDiagnostics bool) gin.H {
	view := gin.H{
		"id":           evidence.Id,
		"request_id":   evidence.RequestID,
		"user_id":      evidence.UserID,
		"app_id":       evidence.AppID,
		"task_id":      evidence.TaskID,
		"kind":         evidence.Kind,
		"client_model": evidence.ClientModel,

		"channel_id":      evidence.ChannelID,
		"client_protocol": evidence.ClientProtocol,
		"created_at":      evidence.CreatedAt,
		"body_expired":    evidence.BodyExpired,
	}
	if includeDiagnostics {
		view["upstream_request_id"] = evidence.UpstreamRequestID
		view["upstream_model"] = evidence.UpstreamModel
	}
	return view
}

func evidenceEventView(event *model.TaskRequestEvidenceEvent, preview service.EvidenceEventPreview) gin.H {
	return gin.H{
		"id":           event.Id,
		"seq":          event.Seq,
		"stage":        event.Stage,
		"phase":        event.Phase,
		"status_code":  event.StatusCode,
		"method":       event.Method,
		"target":       service.EvidenceAdminPreview(event.Target),
		"content_type": event.ContentType,
		"byte_count":   event.ByteCount,
		"stored_bytes": event.StoredBytes,
		"complete":     event.Complete,
		"redacted":     event.Redacted,
		"preview":      preview.Text,
		"body_status":  preview.BodyStatus,
		"detail":       service.EvidenceAdminPreview(event.Detail),
		"created_at":   event.CreatedAt,
		"has_body":     event.ObjectKey != "",
	}
}
