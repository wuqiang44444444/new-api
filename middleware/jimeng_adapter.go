package middleware

import (
	"bytes"
	"io"
	"net/http"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/gin-gonic/gin"
)

func JimengRequestConvert() func(c *gin.Context) {
	return func(c *gin.Context) {
		action := c.Query("Action")
		if action == "" {
			abortJimengVideo(c, http.StatusBadRequest, "Action query parameter is required")
			return
		}
		if action != "CVSync2AsyncSubmitTask" && action != "CVSync2AsyncGetResult" {
			abortJimengVideo(c, http.StatusBadRequest, "Action query parameter is not supported")
			return
		}
		if c.Query("Version") != "2022-08-31" {
			abortJimengVideo(c, http.StatusBadRequest, "Version must be 2022-08-31")
			return
		}

		var originalReq map[string]interface{}
		if err := common.UnmarshalBodyReusable(c, &originalReq); err != nil {
			abortJimengVideo(c, http.StatusBadRequest, "Invalid request body")
			return
		}
		if action == "CVSync2AsyncGetResult" {
			taskId, ok := originalReq["task_id"].(string)
			if !ok || strings.TrimSpace(taskId) == "" {
				abortJimengVideo(c, http.StatusBadRequest, "task_id is required for CVSync2AsyncGetResult")
				return
			}
			c.Request.URL.Path = "/v1/video/generations/" + taskId
			c.Request.Method = http.MethodGet
			c.Set("task_id", taskId)
			c.Set("relay_mode", relayconstant.RelayModeVideoFetchByID)
			c.Next()
			return
		}

		if err := rejectUnknownVideoFields(
			originalReq,
			"req_key", "binary_data_base64", "image_urls", "prompt", "seed", "aspect_ratio", "frames",
		); err != nil {
			abortJimengVideo(c, http.StatusBadRequest, err.Error())
			return
		}
		var official dto.JimengVideoCreateRequest
		if err := decodeTypedVideoRequest(originalReq, &official); err != nil {
			abortJimengVideo(c, http.StatusBadRequest, "Invalid request body")
			return
		}
		if strings.TrimSpace(official.ReqKey) == "" {
			abortJimengVideo(c, http.StatusBadRequest, "req_key is required")
			return
		}
		internal := relaycommon.TaskSubmitReq{
			Model:  official.ReqKey,
			Prompt: videoStringValue(official.Prompt),
			Images: append([]string(nil), official.ImageURLs...),
		}
		relaycommon.SetVideoContractRequest(c, dto.VideoContractRequest{
			ContractID: dto.VideoContractJimeng,
			Jimeng:     &official,
		})
		jsonData, err := common.Marshal(internal)
		if err != nil {
			abortJimengVideo(c, http.StatusInternalServerError, "Failed to marshal request body")
			return
		}

		// Update request body
		c.Request.Body = io.NopCloser(bytes.NewBuffer(jsonData))
		c.Request.ContentLength = int64(len(jsonData))
		c.Set(common.KeyRequestBody, jsonData)

		if len(official.ImageURLs) == 0 && len(official.BinaryDataBase64) == 0 {
			c.Set("action", constant.TaskActionTextGenerate)
		}

		originalPath := c.Request.URL.Path
		c.Request.URL.Path = "/v1/video/generations"
		c.Next()
		c.Request.URL.Path = originalPath
	}
}

func abortJimengVideo(c *gin.Context, status int, message string) {
	code := dto.JimengVideoErrorCode(status)
	c.AbortWithStatusJSON(status, dto.JimengVideoErrorResponse{
		Code:      code,
		Data:      nil,
		Message:   message,
		RequestID: c.GetString(common.RequestIdKey),
		Status:    code,
	})
}
