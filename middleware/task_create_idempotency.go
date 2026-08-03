package middleware

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
)

func TaskCreateIdempotency() gin.HandlerFunc {
	return func(c *gin.Context) {
		if c.Request.Method != http.MethodPost {
			c.Next()
			return
		}
		if common.GetContextKeyString(c, constant.ContextKeyTaskClientProtocol) == model.TaskClientProtocolJimeng &&
			c.Query("Action") == "CVSync2AsyncGetResult" {
			c.Next()
			return
		}
		rawKey := strings.TrimSpace(c.GetHeader("Idempotency-Key"))
		if rawKey == "" {
			c.Next()
			return
		}
		if len(rawKey) > 191 {
			abortTaskCreateIdempotency(c, http.StatusBadRequest, "invalid_idempotency_key", "Idempotency-Key is too long")
			return
		}
		protocol := common.GetContextKeyString(c, constant.ContextKeyTaskClientProtocol)
		requestHash, err := taskCreateRequestHash(c, protocol)
		if err != nil {
			abortTaskCreateIdempotency(c, http.StatusBadRequest, "invalid_request", "request body could not be read")
			return
		}
		keyDigest := sha256.Sum256([]byte(rawKey))
		claim, created, err := model.ClaimTaskCreateIdempotency(
			c.GetInt("id"),
			protocol,
			hex.EncodeToString(keyDigest[:]),
			requestHash,
			time.Now().Add(24*time.Hour).Unix(),
		)
		if errors.Is(err, model.ErrTaskCreateIdempotencyConflict) {
			abortTaskCreateIdempotency(c, http.StatusConflict, "idempotency_conflict", "Idempotency-Key was already used with a different request")
			return
		}
		if err != nil {
			abortTaskCreateIdempotency(c, http.StatusInternalServerError, "internal_error", "idempotency state could not be created")
			return
		}
		if !created {
			replayTaskCreateIdempotency(c, claim)
			return
		}
		common.SetContextKey(c, constant.ContextKeyTaskIdempotencyID, int(claim.ID))
		c.Next()
		if common.GetContextKeyBool(c, constant.ContextKeyTaskIdempotencyCompletedNoReplay) {
			if err := model.MarkTaskCreateIdempotencyCompletedNoReplay(claim.ID); err != nil {
				common.SysError("mark task create idempotency completed without replay failed: " + err.Error())
			}
			return
		}
		if common.GetContextKeyBool(c, constant.ContextKeyTaskIdempotencyRelease) ||
			(protocol == model.TaskClientProtocolOpenAIImages &&
				!common.GetContextKeyBool(c, constant.ContextKeyTaskPersistenceEnabled)) {
			_ = model.ReleaseTaskCreateIdempotency(claim.ID)
			return
		}
		status := c.Writer.Status()
		if status >= http.StatusBadRequest && status < http.StatusInternalServerError {
			_ = model.ReleaseTaskCreateIdempotency(claim.ID)
			return
		}
		if status >= http.StatusInternalServerError &&
			!common.GetContextKeyBool(c, constant.ContextKeyTaskUpstreamStarted) {
			_ = model.ReleaseTaskCreateIdempotency(claim.ID)
			return
		}
		if err := model.MarkTaskCreateIdempotencyUnknown(claim.ID); err != nil {
			common.SysError("mark task create idempotency unknown failed: " + err.Error())
		}
	}
}

func replayTaskCreateIdempotency(c *gin.Context, claim *model.TaskCreateIdempotency) {
	if claim != nil && claim.Status == model.TaskCreateIdempotencyCompletedNoReplay {
		abortTaskCreateIdempotency(c, http.StatusConflict, "idempotency_result_unavailable", "the original response cannot be replayed")
		return
	}
	if claim != nil && claim.Status == model.TaskCreateIdempotencyUpstreamSucceeded {
		task, err := model.RecoverTaskCreateIdempotency(claim.ID)
		if err == nil && task != nil {
			replayTaskCreateResponse(c, claim.Protocol, task)
			return
		}
	}
	if claim != nil && claim.Status == model.TaskCreateIdempotencyComplete && claim.TaskID != "" {
		task, exists, err := model.GetTaskForProtocol(c.GetInt("id"), claim.TaskID, claim.Protocol, true)
		if err == nil && exists {
			replayTaskCreateResponse(c, claim.Protocol, task)
			return
		}
	}
	abortTaskCreateIdempotency(c, http.StatusConflict, "idempotency_in_progress", "the original create outcome is pending reconciliation")
}

func replayTaskCreateResponse(c *gin.Context, protocol string, task *model.Task) {
	switch protocol {
	case model.TaskClientProtocolOpenAIVideos:
		c.AbortWithStatusJSON(http.StatusOK, task.ToOpenAIVideo())
	case model.TaskClientProtocolOpenAIImages:
		imageTask := model.ProjectOpenAIImageTask(task)
		if task.Status == model.TaskStatusSuccess && imageTask.Result != nil {
			c.AbortWithStatusJSON(http.StatusOK, imageTask.Result)
			return
		}
		if task.Status.IsActive() {
			c.Header("Location", "/v1/images/tasks/"+task.TaskID)
			c.Header("Retry-After", "2")
			c.Header("X-Task-ID", task.TaskID)
			c.AbortWithStatusJSON(http.StatusAccepted, imageTask)
			return
		}
		c.AbortWithStatusJSON(http.StatusOK, imageTask)
	case model.TaskClientProtocolModelArkV3:
		c.AbortWithStatusJSON(http.StatusOK, gin.H{"id": task.TaskID})
	case model.TaskClientProtocolKlingV1:
		c.AbortWithStatusJSON(http.StatusOK, gin.H{
			"code": 0, "message": "SUCCEED", "request_id": c.GetString(common.RequestIdKey),
			"data": gin.H{"task_id": task.TaskID, "task_status": "submitted"},
		})
	case model.TaskClientProtocolJimeng:
		c.AbortWithStatusJSON(http.StatusOK, gin.H{
			"code": 10000, "message": "Success", "request_id": c.GetString(common.RequestIdKey),
			"status": 10000,
			"data":   gin.H{"task_id": task.TaskID},
		})
	default:
		abortTaskCreateIdempotency(c, http.StatusConflict, "idempotency_unavailable", "the original response cannot be replayed")
	}
}

func abortTaskCreateIdempotency(c *gin.Context, status int, code, message string) {
	protocol := common.GetContextKeyString(c, constant.ContextKeyTaskClientProtocol)
	if protocol == model.TaskClientProtocolModelArkV3 {
		c.AbortWithStatusJSON(status, gin.H{"error": gin.H{
			"code":       code,
			"message":    message,
			"request_id": c.GetString(common.RequestIdKey),
		}})
		return
	}
	if protocol == model.TaskClientProtocolKlingV1 {
		abortKlingVideo(c, status, message)
		return
	}
	if protocol == model.TaskClientProtocolJimeng {
		abortJimengVideo(c, status, message)
		return
	}
	c.AbortWithStatusJSON(status, gin.H{"error": gin.H{
		"message": message,
		"type":    "idempotency_error",
		"code":    code,
	}})
}
