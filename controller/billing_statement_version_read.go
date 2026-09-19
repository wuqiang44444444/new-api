package controller

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"

	"github.com/gin-gonic/gin"
)

// 客户月账单版本读路径绑定与剩余动作（docs/80-dev/2026-09-17 方案第 13 节）。
// 版本选择：请求明确版本 → 校验归属/状态/角色 → 读取冻结视图；
// 未指定版本 → 有当前确认版则读取该版本，否则回落实时查询（由调用方继续）。

// tryRespondBillingStatementVersion 处理账单汇总接口的版本绑定。
// 返回 true 表示已按冻结版本响应（或已写出错误）；false 表示调用方继续既有实时路径。
func tryRespondBillingStatementVersion(c *gin.Context, period billingReconciliationPeriod, userId int, dimension string, admin bool) bool {
	versionRef := strings.TrimSpace(c.Query("version"))
	var version *model.BillingStatementVersion
	ctx := c.Request.Context()
	if versionRef != "" {
		v, err := model.GetBillingStatementVersionByDraftPublicId(ctx, versionRef)
		if err != nil || v.UserId != userId || v.PeriodStart != period.StartTimestamp || !billingStatementVersionVisibleTo(v, userId, admin) {
			c.JSON(http.StatusNotFound, gin.H{"success": false, "message": "billing statement version not found"})
			return true
		}
		version = v
	} else {
		month, err := model.GetBillingStatementMonthByUserPeriod(ctx, userId, period.StartTimestamp)
		if err != nil {
			common.ApiError(c, err)
			return true
		}
		if month == nil || month.CurrentVersionId == nil {
			return false
		}
		v, err := model.GetBillingStatementVersion(ctx, *month.CurrentVersionId)
		if err != nil {
			common.ApiError(c, err)
			return true
		}
		if v.UserId != userId || v.PeriodStart != period.StartTimestamp || v.Status != model.BillingStatementVersionConfirmed {
			common.ApiErrorMsg(c, "invalid current statement version")
			return true
		}
		version = v
	}
	if dimension == "" {
		dimension = "api_key"
	}
	if dimension != "api_key" && (dimension != "channel" || !admin) {
		common.ApiErrorMsg(c, "invalid dimension")
		return true
	}
	var groupID *int
	key := "token_id"
	if admin {
		key = "group_id"
	}
	if raw := c.Query(key); raw != "" {
		value, err := strconv.Atoi(raw)
		if err != nil || value < 0 {
			common.ApiErrorMsg(c, "invalid group id")
			return true
		}
		groupID = &value
	}
	statement, err := model.ReadBillingStatementProjection(version, dimension, groupID, strings.TrimSpace(c.Query("model_name")), strings.TrimSpace(c.Query("billing_mode")))
	if err != nil {
		common.ApiError(c, err)
		return true
	}
	statement.CurrentBalance, err = model.GetBillingStatementCurrentBalance(ctx, userId)
	if err != nil {
		common.ApiError(c, err)
		return true
	}
	filters := gin.H{"user_id": userId, "dimension": "api_key"}
	if admin {
		filters["dimension"] = dimension
	}
	common.ApiSuccess(c, gin.H{
		"period":          period,
		"filters":         filters,
		"result":          statement,
		"generated_at":    common.GetTimestamp(),
		"data_version":    period.EndTimestamp,
		"data_source":     "billing_statement_version",
		"billing_version": billingStatementVersionView(version, admin),
	})
	return true
}

// billingStatementVersionVisibleTo 角色可见性：管理员可见全部状态；
// 客户仅可见自己的已确认版本（草稿/生成中/待确认不可见，方案 6.2）。
func billingStatementVersionVisibleTo(v *model.BillingStatementVersion, userId int, admin bool) bool {
	if admin {
		return true
	}
	return v.UserId == userId && v.Status == model.BillingStatementVersionConfirmed
}

// GetAdminBillingStatementVersionMonthStatus 管理端客户月版本状态（方案 6.1 状态条）。
func GetAdminBillingStatementVersionMonthStatus(c *gin.Context) {
	userId, err := strconv.Atoi(c.Query("user_id"))
	if err != nil || userId <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"message": "valid user_id is required"})
		return
	}
	period, ok := parseBillingReconciliationPeriod(c)
	if !ok {
		return
	}
	respondBillingStatementVersionMonthStatus(c, userId, period, true)
}

// GetSelfBillingStatementVersionMonthStatus 客户端本人月版本状态（方案 6.2）。
func GetSelfBillingStatementVersionMonthStatus(c *gin.Context) {
	period, ok := parseBillingReconciliationPeriod(c)
	if !ok {
		return
	}
	respondBillingStatementVersionMonthStatus(c, c.GetInt("id"), period, false)
}

func respondBillingStatementVersionMonthStatus(c *gin.Context, userId int, period billingReconciliationPeriod, admin bool) {
	ctx := c.Request.Context()
	view := gin.H{
		"switch_enabled":   model.BillingStatementVersionEnabled(),
		"topology_ok":      model.BillingStatementVersionTopologyOK(),
		"current_version":  nil,
		"active_draft":     nil,
		"versions":         []any{},
		"has_active_draft": false,
	}
	month, err := model.GetBillingStatementMonthByUserPeriod(ctx, userId, period.StartTimestamp)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	retention, err := model.GetBillingStatementRetention(ctx, userId, period.StartTimestamp)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	retentionStatus, err := model.BillingStatementEffectiveRetention(ctx, retention)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	view["retention_status"] = retentionStatus
	if month != nil {
		if month.CurrentVersionId != nil {
			v, err := model.GetBillingStatementVersion(ctx, *month.CurrentVersionId)
			if err != nil {
				common.ApiError(c, err)
				return
			}
			view["current_version"] = billingStatementVersionView(v, admin)
		}
		if month.ActiveDraftId != nil {
			v, err := model.GetBillingStatementVersion(ctx, *month.ActiveDraftId)
			if err != nil {
				common.ApiError(c, err)
				return
			}
			if admin {
				view["active_draft"] = billingStatementVersionView(v, true)
			}
			view["has_active_draft"] = true
		}
		versions, err := model.ListBillingStatementVersionsByMonth(ctx, month.ID)
		if err != nil {
			common.ApiError(c, err)
			return
		}
		views := make([]gin.H, 0, len(versions))
		for i := range versions {
			// 客户只看已确认版本；管理员看全部。
			if !admin && versions[i].Status != model.BillingStatementVersionConfirmed {
				continue
			}
			views = append(views, billingStatementVersionView(&versions[i], admin))
		}
		view["versions"] = views
	}
	common.ApiSuccess(c, view)
}

// GetAdminBillingStatementVersionLines 管理端版本明细分页（草稿/正式均可）。
func GetAdminBillingStatementVersionLines(c *gin.Context) {
	respondBillingStatementVersionLines(c, true)
}

// GetSelfBillingStatementVersionLines 客户端版本明细分页（仅已确认版本，方案 6.2）。
func GetSelfBillingStatementVersionLines(c *gin.Context) {
	respondBillingStatementVersionLines(c, false)
}

func respondBillingStatementVersionLines(c *gin.Context, admin bool) {
	ctx := c.Request.Context()
	v, ok := loadBillingStatementVersionForRead(c, admin)
	if !ok {
		return
	}
	if v.Status != model.BillingStatementVersionPending && v.Status != model.BillingStatementVersionConfirmed {
		common.ApiErrorMsg(c, "statement version is not ready")
		return
	}

	page, err := strconv.Atoi(c.DefaultQuery("page", "1"))
	if err != nil || page < 1 {
		common.ApiErrorMsg(c, "invalid page")
		return
	}
	pageSize, err := strconv.Atoi(c.DefaultQuery("page_size", "100"))
	if err != nil || pageSize < 1 || pageSize > 500 {
		common.ApiErrorMsg(c, "invalid page_size")
		return
	}
	filter := model.BillingStatementVersionLineFilter{
		ModelName:   strings.TrimSpace(c.Query("model_name")),
		BillingMode: strings.TrimSpace(c.Query("billing_mode")),
	}
	if raw := strings.TrimSpace(c.Query("token_id")); raw != "" {
		tokenId, err := strconv.Atoi(raw)
		if err != nil || tokenId < 0 {
			common.ApiErrorMsg(c, "invalid token_id")
			return
		}
		filter.TokenId = &tokenId
	}
	if raw := c.Query("channel_id"); raw != "" {
		value, err := strconv.Atoi(raw)
		if !admin || err != nil || value < 0 {
			common.ApiErrorMsg(c, "invalid channel_id")
			return
		}
		filter.ChannelId = &value
	}
	if raw := strings.TrimSpace(c.Query("log_type")); raw != "" {
		logType, err := strconv.Atoi(raw)
		if err != nil {
			common.ApiErrorMsg(c, "invalid log_type")
			return
		}
		filter.LogType = logType
	}
	lines, total, err := model.ListBillingStatementVersionLines(ctx, v.ID, filter, page, pageSize)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	views, err := billingStatementLineViews(lines)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, gin.H{
		"lines": views, "total": total, "page": page, "page_size": pageSize,
		"billing_version": billingStatementVersionView(v, admin),
	})
}

// loadBillingStatementVersionForRead 按路径参数加载版本并校验归属/角色；失败时已写出响应。
func loadBillingStatementVersionForRead(c *gin.Context, admin bool) (*model.BillingStatementVersion, bool) {
	draftPublicId := c.Param("draft_public_id")
	if draftPublicId == "" {
		c.JSON(http.StatusBadRequest, gin.H{"message": "draft_public_id is required"})
		return nil, false
	}
	userId := c.GetInt("id")
	v, err := model.GetBillingStatementVersionByDraftPublicId(c.Request.Context(), draftPublicId)
	if err != nil || !billingStatementVersionVisibleTo(v, userId, admin) {
		c.JSON(http.StatusNotFound, gin.H{"success": false, "message": "billing statement version not found"})
		return nil, false
	}
	if err := model.AuthorizeCustomerExport(c.Request.Context(), userId, v.UserId); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"success": false, "message": "billing statement version not found"})
		return nil, false
	}
	return v, true
}

// GetAdminBillingStatementVersionDownload 管理端产物下载（草稿预览标记由前端展示，方案 6.3）。
func GetAdminBillingStatementVersionDownload(c *gin.Context) {
	respondBillingStatementVersionDownload(c, true)
}

// GetSelfBillingStatementVersionDownload 客户端已确认版本产物下载（短时 URL，方案 12.6）。
func GetSelfBillingStatementVersionDownload(c *gin.Context) {
	respondBillingStatementVersionDownload(c, false)
}

func respondBillingStatementVersionDownload(c *gin.Context, admin bool) {
	v, ok := loadBillingStatementVersionForRead(c, admin)
	if !ok {
		return
	}
	if v.Status != model.BillingStatementVersionPending && v.Status != model.BillingStatementVersionConfirmed {
		common.ApiErrorMsg(c, "statement version is not ready")
		return
	}

	role := strings.TrimSpace(c.Query("role"))
	url, fileName, expiresAt, err := service.PresignBillingStatementVersionArtifact(c.Request.Context(), v, role)
	if err != nil {
		if errors.Is(err, service.ErrCustomerExportStorageUnavailable) {
			c.JSON(http.StatusConflict, gin.H{"success": false, "message": "billing statement artifact storage is unavailable"})
			return
		}
		c.JSON(http.StatusNotFound, gin.H{"success": false, "message": "billing statement artifact not found or not ready"})
		return
	}
	common.ApiSuccess(c, gin.H{"url": url, "file_name": fileName, "expires_at": expiresAt})
}

// GetAdminBillingStatementVersionDiff 版本差异（方案 6.1 更正对比）。
// compare 缺省为活动草稿；base 缺省为 compare 的更正基准或当前确认版。
func GetAdminBillingStatementVersionDiff(c *gin.Context) {
	ctx := c.Request.Context()
	userId, err := strconv.Atoi(c.Query("user_id"))
	if err != nil || userId <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"message": "valid user_id is required"})
		return
	}
	period, ok := parseBillingReconciliationPeriod(c)
	if !ok {
		return
	}
	month, err := model.GetBillingStatementMonthByUserPeriod(ctx, userId, period.StartTimestamp)
	if err != nil || month == nil {
		c.JSON(http.StatusNotFound, gin.H{"success": false, "message": "billing statement month not found"})
		return
	}
	var compare *model.BillingStatementVersion
	if raw := strings.TrimSpace(c.Query("compare_id")); raw != "" {
		id, err := strconv.ParseInt(raw, 10, 64)
		if err != nil {
			common.ApiErrorMsg(c, "invalid compare_id")
			return
		}
		compare, err = model.GetBillingStatementVersion(ctx, id)
		if err != nil || compare.MonthId != month.ID {
			c.JSON(http.StatusNotFound, gin.H{"success": false, "message": "compare version not found"})
			return
		}
	} else if month.ActiveDraftId != nil {
		compare, err = model.GetBillingStatementVersion(ctx, *month.ActiveDraftId)
		if err != nil {
			common.ApiError(c, err)
			return
		}
	} else {
		c.JSON(http.StatusNotFound, gin.H{"success": false, "message": "no active draft to compare"})
		return
	}
	var base *model.BillingStatementVersion
	if raw := strings.TrimSpace(c.Query("base_id")); raw != "" {
		id, err := strconv.ParseInt(raw, 10, 64)
		if err != nil {
			common.ApiErrorMsg(c, "invalid base_id")
			return
		}
		base, err = model.GetBillingStatementVersion(ctx, id)
	} else if compare.CorrectsVersionId != nil {
		base, err = model.GetBillingStatementVersion(ctx, *compare.CorrectsVersionId)
	} else if month.CurrentVersionId != nil {
		base, err = model.GetBillingStatementVersion(ctx, *month.CurrentVersionId)
	}
	if err != nil || base == nil || base.MonthId != month.ID {
		c.JSON(http.StatusNotFound, gin.H{"success": false, "message": "base version not found"})
		return
	}
	diff, err := service.ComputeBillingStatementVersionDiff(base, compare)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, gin.H{
		"diff":    diff,
		"base":    billingStatementVersionView(base, true),
		"compare": billingStatementVersionView(compare, true),
	})
}

// PostAdminBillingStatementVersionCorrection 发起更正（方案 6.1）：客户可见原因必填，
// 可选内部备注；基准版缺省为该客户月当前确认版。
func PostAdminBillingStatementVersionCorrection(c *gin.Context) {
	userId, err := strconv.Atoi(c.Query("user_id"))
	if err != nil || userId <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"message": "valid user_id is required"})
		return
	}
	period, ok := parseBillingReconciliationPeriod(c)
	if !ok {
		return
	}
	var req struct {
		BaseVersionId *int64 `json:"base_version_id"`
		PublicReason  string `json:"public_reason"`
		InternalNote  string `json:"internal_note"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": "invalid request body"})
		return
	}
	req.PublicReason = strings.TrimSpace(req.PublicReason)
	if req.PublicReason == "" || len([]rune(req.PublicReason)) > 500 {
		c.JSON(http.StatusBadRequest, gin.H{"message": "public_reason is required (max 500 chars)"})
		return
	}
	ctx := c.Request.Context()
	baseVersionId := int64(0)
	if req.BaseVersionId != nil {
		baseVersionId = *req.BaseVersionId
	} else {
		month, err := model.GetBillingStatementMonthByUserPeriod(ctx, userId, period.StartTimestamp)
		if err != nil || month == nil || month.CurrentVersionId == nil {
			c.JSON(http.StatusConflict, gin.H{"success": false, "message": "no confirmed version to correct"})
			return
		}
		baseVersionId = *month.CurrentVersionId
	}
	// 新版本内部统一左闭右开；既有 period.EndTimestamp 为包含式月末，+1 转排他终点。
	draft, err := service.SubmitBillingStatementVersionCorrectionJob(
		ctx, c.GetInt("id"), userId, period.StartTimestamp, period.EndTimestamp+1,
		baseVersionId, req.PublicReason, strings.TrimSpace(req.InternalNote), c.Query("language"),
	)
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

// PostAdminBillingStatementVersionCleanup 清理失效/失败/已取消草稿（方案 12.3）。
func PostAdminBillingStatementVersionCleanup(c *gin.Context) {
	draftPublicId := c.Param("draft_public_id")
	if err := service.CleanupBillingStatementDraft(c.Request.Context(), draftPublicId, c.GetInt("id")); err != nil {
		respondBillingStatementVersionError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true})
}

// 明细输出明确白名单；精确整数以字符串输出，不泄露 Channel 或内部来源身份。
func billingStatementLineViews(lines []model.BillingStatementVersionLine) ([]gin.H, error) {
	views := make([]gin.H, 0, len(lines))
	for _, line := range lines {
		var facts model.CustomerExportRow
		if err := common.UnmarshalJsonStr(line.Facts, &facts); err != nil {
			return nil, err
		}
		quota := line.Quota
		if line.LogType == model.LogTypeRefund {
			quota = -quota
		}
		views = append(views, gin.H{
			"id": strconv.FormatInt(line.ID, 10), "sequence": line.Sequence,
			"request_id": line.RequestId, "token_id": line.TokenId, "token_name": line.TokenName,
			"customer_model": line.CustomerModel, "group": line.Group, "log_type": line.LogType,
			"created_at": line.CreatedAt, "billing_mode": line.BillingMode,
			"input_tokens": strconv.FormatInt(line.InputTokens, 10), "output_tokens": strconv.FormatInt(line.OutputTokens, 10),
			"cache_read_tokens": strconv.FormatInt(line.CacheReadTokens, 10), "cache_write_tokens": strconv.FormatInt(line.CacheWriteTokens, 10),
			"quota": strconv.FormatInt(quota, 10), "facts": gin.H{"input_tokens_unavailable": facts.InputTokensUnavailable, "group_name": facts.GroupName, "group_ratio": facts.GroupRatio, "contract_name": facts.ContractName, "contract_ratio": facts.ContractRatio, "contract_applicable": facts.ContractApplicable, "final_ratio": facts.FinalRatio, "billing_line_items": facts.BillingLineItems, "explanation_status": facts.ExplanationStatus},
		})
	}
	return views, nil
}
