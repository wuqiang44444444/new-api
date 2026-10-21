package controller

import (
	"encoding/base64"
	"net/http"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
)

// Admin/Root list content locations, then explicitly read one protected object.
// Directory reads never decrypt bodies; original access is audited separately.

func GetTaskRequestEvidenceList(c *gin.Context) {
	requestID := strings.TrimSpace(c.Query("request_id"))
	taskID := strings.TrimSpace(c.Query("task_id"))
	upstreamRequestID := strings.TrimSpace(c.Query("upstream_request_id"))
	pageInfo := common.GetPageQuery(c)
	items, total, err := model.QueryTaskRequestEvidence(model.TaskRequestEvidenceQueryParams{
		RequestID:         requestID,
		UpstreamRequestID: upstreamRequestID,
		TaskID:            taskID,
		Kind:              c.Query("kind"),
		StartIdx:          pageInfo.GetStartIdx(),
		Num:               pageInfo.GetPageSize(),
	})
	if err != nil {
		common.ApiErrorMsg(c, "Failed to load request evidence")
		return
	}
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
	model.RecordTaskRequestEvidenceAccess(evidence.Id, int64(c.GetInt("id")), "view", "directory")
	events, err := model.ListTaskRequestEvidenceEvents(evidence.Id)
	if err != nil {
		common.ApiErrorMsg(c, "Failed to query evidence events")
		return
	}
	isRoot := isRootViewer(c)
	eventViews := make([]gin.H, 0, len(events))
	for _, event := range events {
		eventViews = append(eventViews, evidenceEventView(event))
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
	payload, bodyStatus := service.ReadEvidenceEventBody(event, service.IsEvidenceBodyExpired(evidence, common.GetTimestamp()))
	if !respondEvidenceBodyStatus(c, bodyStatus) {
		return
	}
	model.RecordTaskRequestEvidenceAccess(evidence.Id, int64(c.GetInt("id")), "download", "original")
	filename := strconv.FormatInt(event.Id, 10) + ".bin"
	c.Header("Content-Disposition", "attachment; filename=\""+filename+"\"")
	c.Data(http.StatusOK, "application/octet-stream", payload)
}

// GetTaskRequestEvidenceContent returns one selected raw body and captured metadata.
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
	content, bodyStatus := service.ReadEvidenceEventContent(event, service.IsEvidenceBodyExpired(evidence, common.GetTimestamp()))
	if !respondEvidenceBodyStatus(c, bodyStatus) {
		return
	}
	model.RecordTaskRequestEvidenceAccess(evidence.Id, int64(c.GetInt("id")), "view", "original")
	result := gin.H{"meta": content.Meta, "content_type": event.ContentType, "body_status": "available"}
	if service.EvidenceContentIsText(content.Body) {
		result["text"] = string(content.Body)
	} else {
		result["text"] = nil
		result["base64"] = base64.StdEncoding.EncodeToString(content.Body)
	}
	common.ApiSuccess(c, result)
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
		"body_expired":    service.IsEvidenceBodyExpired(evidence, common.GetTimestamp()),
	}
	if includeDiagnostics {
		view["upstream_request_id"] = evidence.UpstreamRequestID
		view["upstream_model"] = evidence.UpstreamModel
	}
	return view
}

// Directory projection never reads an object or exposes captured metadata.
// The event ID is sufficient to attempt a read, even after a failed object write.
func evidenceEventView(event *model.TaskRequestEvidenceEvent) gin.H {
	return gin.H{"id": event.Id, "seq": event.Id, "name": event.Stage, "status_code": event.StatusCode, "method": event.Method, "target": service.EvidenceMaskSignedURLs(event.Target), "content_type": event.ContentType, "created_at": event.CreatedAt}
}
