package service

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
)

// 客户月账单版本固化后台生成（docs/80-dev/2026-09-17 方案第 11 节）。
// 复用 SystemTask 调度/租约底座与既有聚合 reader；版本生成有自己的 handler 和短循环，
// 不同任务类型显式共用资源预算，不复制基础调度系统。

const (
	// 资源预算候选参数（方案 11.2，待测起点）。
	billingStatementVersionBatchTimeout = 5 * time.Second
	billingStatementVersionMaxGroups    = 10000
)

// ErrBillingStatementVersionSourceInsufficient 表示来源保留状态阻止生成可确认版本。
var ErrBillingStatementVersionSourceInsufficient = model.ErrBillingStatementSourceIncomplete

// SubmitBillingStatementVersionJob 受理一次版本生成：校验开关/拓扑/已结束月份/来源保留，
// 原子占用活动草稿，快照来源向量，进入后台执行体系。
func SubmitBillingStatementVersionJob(ctx context.Context, adminId int, userId int, periodStart int64, periodEndExcl int64, language ...string) (*model.BillingStatementVersion, error) {
	if !model.BillingStatementVersionEnabled() {
		return nil, model.ErrBillingStatementVersionDisabled
	}
	if !model.BillingStatementVersionTopologyOK() {
		return nil, model.ErrBillingStatementVersionTopology
	}
	// 只允许已结束的自然月（左闭右开 end 必须早于当前月起点）。
	loc, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		loc = time.UTC
	}
	now := time.Now().In(loc)
	currentMonthStart := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, loc).Unix()
	if periodEndExcl > currentMonthStart {
		return nil, errors.New("billing statement version can only be generated for an ended calendar month")
	}
	// 来源保留核验：已知金额来源缺口阻止生成（方案 10.6 规则 3）。
	ret, err := model.GetBillingStatementRetention(ctx, userId, periodStart)
	if err != nil {
		return nil, err
	}
	if ret == nil || (ret.Status != model.BillingStatementRetentionIntact && ret.Status != model.BillingStatementRetentionNone) {
		return nil, fmt.Errorf("%w: customer %d period %d has cleaned source logs", ErrBillingStatementVersionSourceInsufficient, userId, periodStart)
	}
	// 原子占用活动草稿。
	filters, err := billingStatementGenerationFilters(ctx, adminId, userId, periodStart, periodEndExcl, language...)
	if err != nil {
		return nil, err
	}
	_, draft, err := model.AcquireBillingStatementDraft(ctx, userId, periodStart, "Asia/Shanghai", adminId, filters)
	if err != nil {
		return nil, err
	}
	_, _, _ = EnqueueSystemTask(model.SystemTaskTypeCustomerExport, nil)
	return draft, nil
}

// 由既有导出执行器调用：共用容量、租约、权限、超时与压力门控。
func runBillingStatementVersionGeneration(ctx context.Context, job *model.CustomerExportJob, pressure *customerExportPressureTracker) error {
	ctx = model.WithBillingStatementRefundReferenceCache(ctx)
	filters, err := job.DecodeFilters()
	if err != nil {
		return err
	}
	v, err := model.GetBillingStatementVersionByDraftPublicId(ctx, filters.StatementDraftId)
	if err != nil {
		return err
	}
	if v.ParserVersion != model.BillingStatementParserVersion {
		return model.ErrBillingStatementVersionConflict
	}
	result := model.DB.WithContext(ctx).Model(&model.BillingStatementVersion{}).Where("id = ? AND source_job_id = ? AND status = ?", v.ID, job.JobID, model.BillingStatementVersionQueued).Updates(map[string]interface{}{"status": model.BillingStatementVersionGenerating, "updated_at": common.GetTimestamp()})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return model.ErrBillingStatementVersionConflict
	}
	if err := model.VerifyBillingStatementRetention(ctx, v.UserId, v.PeriodStart); err != nil {
		return err
	}
	gate := customerExportSummaryGate(pressure, job, nil)
	beforeBatch := func(ctx context.Context) error {
		if err := model.CheckBillingStatementGeneration(ctx, v, job); err != nil {
			return err
		}
		return gate(ctx)
	}
	vec, err := model.SnapshotBillingStatementDependencyVector(ctx, v.UserId, v.PeriodStart, model.BillingStatementReadPolicy{BeforeBatch: beforeBatch, BatchTimeout: billingStatementVersionBatchTimeout})
	if err != nil {
		return err
	}
	lines := make([]model.BillingStatementVersionLine, 0, 500)
	integrity := model.BillingStatementIntegrity{}
	flush := func() error {
		if len(lines) == 0 {
			return nil
		}
		if err := model.AppendBillingStatementLines(ctx, v, job, lines); err != nil {
			return err
		}
		lines = lines[:0]
		return nil
	}
	observe := func(line model.BillingStatementVersionLine, source *model.Log) error {
		var facts model.CustomerExportRow
		if err := common.UnmarshalJsonStr(line.Facts, &facts); err != nil {
			return err
		}
		attachCustomerExportBillingExplanation(source, &facts, v.QuotaPerUnit)
		raw, err := common.Marshal(facts)
		if err != nil {
			return err
		}
		line.Facts = string(raw)
		line.VersionId = v.ID
		line.Sequence = integrity.Rows
		integrity.Rows++
		if line.LogType == model.LogTypeConsume {
			integrity.Gross += line.Quota
		} else if line.LogType == model.LogTypeRefund {
			integrity.Refund += line.Quota
		}
		lines = append(lines, line)
		if len(lines) == cap(lines) {
			return flush()
		}
		return nil
	}
	policy := model.BillingStatementReadPolicy{BatchTimeout: billingStatementVersionBatchTimeout, MaxGroups: billingStatementVersionMaxGroups, BeforeBatch: beforeBatch, ObserveFact: observe}
	statement, err := model.GetBillingCustomerStatement(ctx, v.UserId, v.PeriodStart, v.PeriodEndExclusive-1, "api_key", 0, "", "", policy)
	if err != nil {
		return err
	}
	if err := flush(); err != nil {
		return err
	}
	if statement.Summary.GrossQuota != integrity.Gross || statement.Summary.RefundQuota != integrity.Refund || statement.Summary.NetQuota != integrity.Gross-integrity.Refund {
		return errors.New("statement detail totals differ from summary")
	}
	// 第二次有界扫描仍使用同一解析/聚合器，避免为渠道视图复制金额规则。
	policy.ObserveFact = nil
	channel, err := model.GetBillingCustomerStatement(ctx, v.UserId, v.PeriodStart, v.PeriodEndExclusive-1, "channel", 0, "", "", policy)
	if err != nil {
		return err
	}
	if channel.Summary != statement.Summary {
		return errors.New("statement dimensions differ")
	}
	summaryJSON, err := model.FreezeBillingStatementProjection(statement)
	if err != nil {
		return err
	}
	channelJSON, err := model.FreezeBillingStatementProjection(channel)
	if err != nil {
		return err
	}
	integrityJSON, err := common.Marshal(integrity)
	if err != nil {
		return err
	}
	if err := model.CheckBillingStatementGeneration(ctx, v, job); err != nil {
		return err
	}
	result = model.DB.WithContext(ctx).Model(&model.BillingStatementVersion{}).Where("id = ? AND status = ?", v.ID, model.BillingStatementVersionGenerating).Updates(map[string]interface{}{
		"summary_projection": summaryJSON, "channel_projection": channelJSON, "integrity": string(integrityJSON),
		"rev_scope_customer_month": vec.ScopeCustomerMonth, "rev_customer_month": vec.RevCustomerMonth,
		"dependencies":          vec.Dependencies,
		"rev_scope_maintenance": vec.ScopeMaintenance, "rev_maintenance": vec.RevMaintenance, "updated_at": common.GetTimestamp(),
	})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return model.ErrBillingStatementVersionConflict
	}
	v, err = model.GetBillingStatementVersion(ctx, v.ID)
	if err != nil {
		return err
	}
	if err := GenerateBillingStatementVersionArtifacts(ctx, v); err != nil {
		return err
	}
	if err := model.CheckBillingStatementGeneration(ctx, v, job); err != nil {
		return err
	}
	if err := model.VerifyBillingStatementRetention(ctx, v.UserId, v.PeriodStart); err != nil {
		return err
	}
	changed, err := model.BillingStatementVectorChanged(ctx, vec)
	if err != nil {
		return err
	}
	if changed {
		return model.ErrBillingStatementVersionConflict
	}
	// pending 在 FinishCustomerExportJob 的租约/取消守卫事务中发布。
	return nil
}
