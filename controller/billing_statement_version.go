package controller

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"

	"github.com/gin-gonic/gin"
)

// 客户月账单版本固化控制器（docs/80-dev/2026-09-17 方案第 13 节）。
// 管理端动作：生成草稿、查询进度、确认、放弃、历史版本。金额与校验结论只来自服务端。

// PostAdminBillingStatementVersion 生成待确认版本草稿（管理员）。
// 只接受客户与账期；不接受客户端金额或校验结论。
func PostAdminBillingStatementVersion(c *gin.Context) {
	userId, err := strconv.Atoi(c.Query("user_id"))
	if err != nil || userId <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"message": "valid user_id is required"})
		return
	}
	period, ok := parseBillingReconciliationPeriod(c)
	if !ok {
		return
	}
	// 新版本内部统一左闭右开；既有 period.EndTimestamp 为包含式月末，+1 转排他终点。
	endExclusive := period.EndTimestamp + 1
	draft, err := service.SubmitBillingStatementVersionJob(c.Request.Context(), c.GetInt("id"), userId, period.StartTimestamp, endExclusive, c.Query("language"))
	if err != nil {
		respondBillingStatementVersionError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success":         true,
		"draft_public_id": draft.DraftPublicId,
		"status":          draft.Status,
	})
}

// GetAdminBillingStatementVersion 查询草稿/版本状态与进度（管理员）。
func GetAdminBillingStatementVersion(c *gin.Context) {
	draftPublicId := c.Param("draft_public_id")
	if draftPublicId == "" {
		c.JSON(http.StatusBadRequest, gin.H{"message": "draft_public_id is required"})
		return
	}
	v, err := model.GetBillingStatementVersionByDraftPublicId(c.Request.Context(), draftPublicId)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"message": "version not found"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "version": billingStatementVersionView(v, true)})
}

// PostAdminBillingStatementVersionConfirm 确认发布（管理员）。
// 输入只接受草稿标识、基准确认版、幂等键、知悉内容、公开原因；金额与校验结论来自服务端。
func PostAdminBillingStatementVersionConfirm(c *gin.Context) {
	draftPublicId := c.Param("draft_public_id")
	var req struct {
		BaseVersionId  *int64 `json:"base_version_id"`
		IdempotencyKey string `json:"idempotency_key"`
		AcknowledgedQA string `json:"acknowledged_quality"`
		PublicReason   string `json:"public_reason"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": "invalid request body"})
		return
	}
	v, committed, err := service.ConfirmBillingStatementVersion(
		c.Request.Context(), draftPublicId, req.BaseVersionId, req.IdempotencyKey, req.AcknowledgedQA, req.PublicReason, c.GetInt("id"),
	)
	if err != nil {
		respondBillingStatementVersionError(c, err)
		return
	}
	if committed {
		// 确认后定稿产物：保留类别切换 + 确认说明文件；失败只影响文件下载，不回退确认。
		if err := service.FinalizeBillingStatementVersionArtifacts(c.Request.Context(), v); err != nil {
			common.SysError("billing statement confirmation note pending; download will retry")
		}
	}
	c.JSON(http.StatusOK, gin.H{
		"success":   true,
		"committed": committed,
		"version":   billingStatementVersionView(v, true),
	})
}

// PostAdminBillingStatementVersionAbandon 放弃草稿（管理员）。
func PostAdminBillingStatementVersionAbandon(c *gin.Context) {
	draftPublicId := c.Param("draft_public_id")
	var req struct {
		Reason string `json:"reason"`
	}
	_ = c.ShouldBindJSON(&req)
	if err := model.AbandonBillingStatementDraft(c.Request.Context(), draftPublicId, c.GetInt("id"), req.Reason); err != nil {
		respondBillingStatementVersionError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true})
}

// GetAdminBillingStatementVersionHistory 列出该客户月的历史版本（管理员）。
func GetAdminBillingStatementVersionHistory(c *gin.Context) {
	userId, err := strconv.Atoi(c.Query("user_id"))
	if err != nil || userId <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"message": "valid user_id is required"})
		return
	}
	period, ok := parseBillingReconciliationPeriod(c)
	if !ok {
		return
	}
	m, err := model.GetBillingStatementMonthByUserPeriod(c.Request.Context(), userId, period.StartTimestamp)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if m == nil {
		c.JSON(http.StatusOK, gin.H{"success": true, "versions": []any{}})
		return
	}
	versions, err := model.ListBillingStatementVersionsByMonth(c.Request.Context(), m.ID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": err.Error()})
		return
	}
	views := make([]gin.H, 0, len(versions))
	for i := range versions {
		views = append(views, billingStatementVersionView(&versions[i], true))
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "versions": views, "current_version_id": m.CurrentVersionId})
}

// billingStatementVersionView 投影版本；管理员可见内部备注，客户视图（阶段 5 页面接入）不返回。
func billingStatementVersionView(v *model.BillingStatementVersion, admin bool) gin.H {
	view := gin.H{
		"id":                   v.ID,
		"draft_public_id":      v.DraftPublicId,
		"status":               v.Status,
		"version_number":       v.VersionNumber,
		"user_id":              v.UserId,
		"period_start":         v.PeriodStart,
		"period_end_exclusive": v.PeriodEndExclusive,
		"confirmed_at":         v.ConfirmedAt,
		"created_at":           v.CreatedAt,
		"updated_at":           v.UpdatedAt,
		"public_reason":        v.PublicReason,
		"corrects_version_id":  v.CorrectsVersionId,
	}
	view["quota_per_unit"] = v.QuotaPerUnit
	view["currency"] = v.Currency
	view["currency_rate"] = v.CurrencyRate
	if admin && v.Status == model.BillingStatementVersionPending {
		if statement, err := model.BillingStatementVersionStatement(v); err == nil {
			view["data_quality"] = statement.DataQuality
		}
	}
	if admin {
		view["internal_note"] = v.InternalNote
	}
	return view
}

// respondBillingStatementVersionError 把版本固化错误映射为稳定响应。
func respondBillingStatementVersionError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, service.ErrBillingStatementVersionForbidden):
		c.JSON(http.StatusForbidden, gin.H{"message": service.ErrBillingStatementVersionForbidden.Error()})
	case errors.Is(err, model.ErrCustomerExportQueueBusy), errors.Is(err, model.ErrCustomerExportUserBusy):
		c.JSON(http.StatusTooManyRequests, gin.H{"message": "export queue is full; try again later"})
	case errors.Is(err, model.ErrBillingStatementVersionDisabled):
		c.JSON(http.StatusForbidden, gin.H{"message": "billing statement version confirmation is disabled"})
	case errors.Is(err, model.ErrBillingStatementVersionTopology):
		c.JSON(http.StatusConflict, gin.H{"message": "billing statement version confirmation requires the same-database topology"})
	case errors.Is(err, model.ErrBillingStatementVersionConflict):
		c.JSON(http.StatusConflict, gin.H{"message": "billing statement version state conflict or source changed"})
	case errors.Is(err, service.ErrBillingStatementVersionSourceInsufficient), errors.Is(err, model.ErrBillingStatementSourceIncomplete):
		c.JSON(http.StatusConflict, gin.H{"message": "source retention insufficient to generate a confirmable version"})
	case errors.Is(err, service.ErrBillingStatementArtifactUnavailable):
		c.JSON(http.StatusServiceUnavailable, gin.H{"message": service.ErrBillingStatementArtifactUnavailable.Error()})
	default:
		c.JSON(http.StatusInternalServerError, gin.H{"message": "unable to process billing statement version"})
	}
}

// PostAdminBillingStatementSourceVerification 仅 Root 登记已核验来源的依据；不从 count/sum 自动推断完整。
func PostAdminBillingStatementSourceVerification(c *gin.Context) {
	if c.GetInt("role") < common.RoleRootUser {
		c.JSON(http.StatusForbidden, gin.H{"success": false})
		return
	}
	userID, err := strconv.Atoi(c.Query("user_id"))
	if err != nil || userID <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"message": "valid user_id is required"})
		return
	}
	period, ok := parseBillingReconciliationPeriod(c)
	if !ok {
		return
	}
	var req model.BillingSourceVerification
	if c.ShouldBindJSON(&req) != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": "invalid request body"})
		return
	}
	if err := model.RecordBillingSourceVerification(c.Request.Context(), userID, period.StartTimestamp, period.EndTimestamp, req, c.GetInt("id")); err != nil {
		respondBillingStatementVersionError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true})
}
