package middleware

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"

	"github.com/gin-gonic/gin"
)

// 图片创建幂等（§3.7）：可选 Idempotency-Key，仅在显式异步模式下支持；
// 键域 = user + app(token) + 操作 + 客户键，编码进 key_hash 摘要前缀，
// 复用既有 TaskCreateIdempotency 表与五态定义，不改视频键语义。

func ImageCreateIdempotency() gin.HandlerFunc {
	return func(c *gin.Context) {
		if c.Request.Method != http.MethodPost {
			c.Next()
			return
		}
		rawKey := strings.TrimSpace(c.GetHeader("Idempotency-Key"))
		if rawKey == "" {
			c.Next()
			return
		}
		if !service.PreferRespondAsync(c) {
			abortImageIdempotency(c, http.StatusBadRequest, "invalid_idempotency_key", "Idempotency-Key requires Prefer: respond-async for image requests")
			return
		}
		if len(rawKey) > 191 {
			abortImageIdempotency(c, http.StatusBadRequest, "invalid_idempotency_key", "Idempotency-Key is too long")
			return
		}
		appID := c.GetInt("token_id")
		operation := service.ImageCreateOperationFromPath(c)
		namespacedKey := strings.Join([]string{
			"image_v1", strconv.Itoa(appID), string(operation), rawKey,
		}, "|")
		keyDigest := sha256.Sum256([]byte(namespacedKey))
		requestHash, err := imageCreateRequestHash(c)
		if err != nil {
			abortImageIdempotency(c, http.StatusBadRequest, "invalid_request", "request body could not be read")
			return
		}
		claim, created, err := model.ClaimTaskCreateIdempotency(
			c.GetInt("id"),
			model.TaskClientProtocolImageOpenAIV1,
			hex.EncodeToString(keyDigest[:]),
			requestHash,
			time.Now().Add(24*time.Hour).Unix(),
		)
		if errors.Is(err, model.ErrTaskCreateIdempotencyConflict) {
			abortImageIdempotency(c, http.StatusConflict, "idempotency_conflict", "Idempotency-Key was already used with a different request")
			return
		}
		if err != nil {
			abortImageIdempotency(c, http.StatusInternalServerError, "internal_error", "idempotency state could not be created")
			return
		}
		if !created {
			replayImageCreateIdempotency(c, claim, appID)
			return
		}
		common.SetContextKey(c, constant.ContextKeyTaskIdempotencyID, int(claim.ID))
		c.Next()
		// 任何失败响应都尝试释放 claim：ReleaseTaskCreateIdempotency 只删
		// creating 态，已由受理事务完成的 claim 不受影响。若不释放，早失败
		// （如存储不可用）会让同 key 重试永远命中 in_progress。
		if c.Writer.Status() >= http.StatusBadRequest {
			_ = model.ReleaseTaskCreateIdempotency(claim.ID)
		}
	}
}

// imageCreateRequestHash 计算图片创建的逻辑请求摘要：JSON 走通用规范化；
// multipart 忽略 boundary 等传输噪声，按排序字段值 + 文件内容摘要计算
// （方案 §3.7 请求等价，评审 S7）。
func imageCreateRequestHash(c *gin.Context) (string, error) {
	contentType := c.GetHeader("Content-Type")
	if !strings.HasPrefix(contentType, "multipart/form-data") {
		return taskCreateRequestHash(c, model.TaskClientProtocolImageOpenAIV1)
	}
	form, err := common.ParseMultipartFormReusable(c)
	if err != nil {
		return "", err
	}
	digest := sha256.New()
	_, _ = io.WriteString(digest, c.Request.Method+"\n"+c.Request.URL.Path+"\n"+model.TaskClientProtocolImageOpenAIV1+"\n")
	keys := make([]string, 0, len(form.Value))
	for key := range form.Value {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		values := form.Value[key]
		sort.Strings(values)
		for _, value := range values {
			_, _ = io.WriteString(digest, key+"="+value+"\n")
		}
	}
	fileKeys := make([]string, 0, len(form.File))
	for key := range form.File {
		fileKeys = append(fileKeys, key)
	}
	sort.Strings(fileKeys)
	for _, key := range fileKeys {
		headers := form.File[key]
		// 保持客户端提交顺序：图片顺序具有语义（§3.7）。
		for _, header := range headers {
			file, openErr := header.Open()
			if openErr != nil {
				return "", openErr
			}
			fileDigest := sha256.New()
			if _, copyErr := io.Copy(fileDigest, io.LimitReader(file, imageIdempotencyMaxFileBytes)); copyErr != nil {
				_ = file.Close()
				return "", copyErr
			}
			_ = file.Close()
			_, _ = io.WriteString(digest, key+"="+header.Filename+":")
			_, _ = digest.Write(fileDigest.Sum(nil))
			_, _ = io.WriteString(digest, "\n")
		}
	}
	return hex.EncodeToString(digest.Sum(nil)), nil
}

const imageIdempotencyMaxFileBytes = 64 << 20

func replayImageCreateIdempotency(c *gin.Context, claim *model.TaskCreateIdempotency, appID int) {
	if claim == nil || claim.Status != model.TaskCreateIdempotencyComplete || claim.TaskID == "" {
		abortImageIdempotency(c, http.StatusConflict, "idempotency_in_progress", "the original create outcome is pending reconciliation")
		return
	}
	task, exists, err := model.GetByTaskIDForApp(c.GetInt("id"), appID, claim.TaskID)
	if err != nil || !exists || task == nil || !model.IsImageTask(task) {
		abortImageIdempotency(c, http.StatusConflict, "idempotency_in_progress", "the original create outcome is pending reconciliation")
		return
	}
	if data := task.PrivateData.ImageTask; data != nil && !data.FundsHeld {
		// 受理未完成（预扣失败/标记失败，或预扣仍在进行）：重放不可签发
		// 202，避免并发窗口内出现第二个"已受理"假象（评审 S7）。
		if task.Status == model.TaskStatusFailure {
			abortImageIdempotency(c, http.StatusConflict, "idempotency_result_unavailable", "the original acceptance did not complete")
			return
		}
		abortImageIdempotency(c, http.StatusConflict, "idempotency_in_progress", "the original acceptance is still completing")
		return
	}
	createdAt := task.CreatedAt
	if createdAt == 0 {
		createdAt = task.SubmitTime
	}
	queryPath := "/v1/tasks/" + task.TaskID
	c.Header("Location", queryPath)
	c.AbortWithStatusJSON(http.StatusAccepted, gin.H{
		"created":   createdAt,
		"id":        task.TaskID,
		"object":    "image_task",
		"status":    "queued",
		"query_url": queryPath,
	})
}

func abortImageIdempotency(c *gin.Context, status int, code, message string) {
	c.AbortWithStatusJSON(status, gin.H{
		"error": gin.H{
			"message": message,
			"type":    "idempotency_error",
			"code":    code,
		},
	})
}
