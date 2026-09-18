package controller

import (
	"strconv"

	"github.com/QuantumNous/new-api/clienterrlog"
	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"

	"github.com/gin-gonic/gin"
)

// GetErrorLogs 查询 API 调用错误事件（relay/asset 已鉴权请求的最终 4xx/5xx）。
// 权限由路由层 AdminAuth 控制；表中只保存受控脱敏字段，无 Root 专属诊断内容。
func GetErrorLogs(c *gin.Context) {
	pageInfo := common.GetPageQuery(c)
	module := c.Query("module")
	if !model.ValidErrorEventModule(module) {
		common.ApiErrorMsg(c, "invalid module filter")
		return
	}
	eventType := c.Query("event_type")
	if !clienterrlog.ValidEventType(eventType) {
		common.ApiErrorMsg(c, "invalid event_type filter")
		return
	}
	// 状态过滤必须区分「未指定」与「指定为 0（无 HTTP 状态）」，用可空值承载。
	var status *int
	if rawStatus := c.Query("status"); rawStatus != "" {
		parsed, err := strconv.Atoi(rawStatus)
		if err != nil {
			common.ApiErrorMsg(c, "invalid status filter")
			return
		}
		status = &parsed
	}
	userId, _ := strconv.Atoi(c.Query("user_id"))
	channel, _ := strconv.Atoi(c.Query("channel"))
	startTimestamp, _ := strconv.ParseInt(c.Query("start_timestamp"), 10, 64)
	endTimestamp, _ := strconv.ParseInt(c.Query("end_timestamp"), 10, 64)
	events, total, err := model.GetErrorEvents(model.ErrorEventFilter{
		Module:            module,
		EventType:         eventType,
		TaskId:            c.Query("task_id"),
		Status:            status,
		UserId:            userId,
		Username:          c.Query("username"),
		TokenName:         c.Query("token_name"),
		ModelName:         c.Query("model_name"),
		ChannelId:         channel,
		RequestId:         c.Query("request_id"),
		UpstreamRequestId: c.Query("upstream_request_id"),
		Reason:            c.Query("reason"),
		StartTimestamp:    startTimestamp,
		EndTimestamp:      endTimestamp,
	}, pageInfo.GetStartIdx(), pageInfo.GetPageSize())
	if err != nil {
		common.ApiError(c, err)
		return
	}
	pageInfo.SetTotal(int(total))
	pageInfo.SetItems(events)
	common.ApiSuccess(c, pageInfo)
}
