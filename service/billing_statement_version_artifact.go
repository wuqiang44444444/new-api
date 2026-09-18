package service

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/shopspring/decimal"

	"gorm.io/gorm"
)

// 客户月账单版本的产物生成、确认后定稿、版本差异、更正受理与草稿清理
// （docs/80-dev/2026-09-17 方案第 11.1、12、13 节）。
// 复用既有导出制表与对象存储原语；产物写入独立私有命名空间 billing/statements，
// 对象键以草稿公开 ID 为目录，唯一且不覆盖旧版。

const (
	// billingStatementVersionArtifactTTL 是下载签名的有效期（短时 URL，方案 12.6）。
	billingStatementVersionArtifactTTL = 10 * time.Minute

	billingStatementVersionArtifactRoleSummary = "summary_csv"
	billingStatementVersionArtifactRoleDetail  = "detail_csv"
	billingStatementVersionArtifactRoleNote    = "confirmation_note"
)

// stagedBillingStatementVersionFile 是待登记/上传的产物文件。
type stagedBillingStatementVersionFile struct {
	role      string
	fileName  string
	path      string
	sizeBytes int64
	lineCount int64
	sha256    string
}

// GenerateBillingStatementVersionArtifacts 从冻结事实生成基础产物：
// 汇总 CSV 复用既有制表函数（不重新扫描来源），明细 CSV 直接写版本明细白名单。
// 先登记 staged 清单，逐个上传成功后发布；上传失败保留 staged 引用交由清理。
func GenerateBillingStatementVersionArtifacts(ctx context.Context, v *model.BillingStatementVersion) error {
	store, err := currentExportObjectStore()
	if err != nil {
		return err
	}
	statement, err := model.BillingStatementVersionStatement(v)
	if err != nil {
		return err
	}
	workDir := filepath.Join(os.TempDir(), "billing-statement-version-"+v.DraftPublicId)
	defer func() { _ = os.RemoveAll(workDir) }()

	files, err := buildBillingStatementVersionArtifactFiles(ctx, workDir, v, statement)
	if err != nil {
		return err
	}
	if len(files) == 0 {
		return nil
	}
	var totalBytes int64
	for _, file := range files {
		totalBytes += file.sizeBytes
	}
	if totalBytes > customerExportTotalBytes {
		return errCustomerExportBudgetExceeded
	}
	now := common.GetTimestamp()
	artifacts := make([]model.BillingStatementArtifact, 0, len(files))
	for _, f := range files {
		artifacts = append(artifacts, model.BillingStatementArtifact{
			VersionId:      v.ID,
			Role:           f.role,
			Language:       v.Language,
			FormatVersion:  customerExportFieldVersion,
			ObjectKey:      model.BillingStatementNamespace + "/" + v.DraftPublicId + "/" + f.fileName,
			FileName:       f.fileName,
			SizeBytes:      f.sizeBytes,
			LineCount:      f.lineCount,
			Sha256:         f.sha256,
			StoreIdentity:  store.ExportIdentity(),
			RetentionClass: "draft",
			Staged:         true,
			CreatedAt:      now,
		})
	}
	if err := model.DB.WithContext(ctx).Create(&artifacts).Error; err != nil {
		return err
	}
	for i := range artifacts {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := store.ExportPutFile(ctx, artifacts[i].ObjectKey, artifactMimeType(artifacts[i].Role), files[i].path, artifacts[i].SizeBytes); err != nil {
			return fmt.Errorf("upload billing statement artifact %s: %w", artifacts[i].Role, err)
		}
		readable := artifacts[i]
		readable.Staged = false
		if err := verifyBillingStatementArtifact(ctx, store, readable); err != nil {
			return err
		}
		if err := model.DB.WithContext(ctx).Model(&model.BillingStatementArtifact{}).
			Where("id = ? AND staged = ?", artifacts[i].ID, true).
			Updates(map[string]any{"staged": false, "uploaded_at": common.GetTimestamp()}).Error; err != nil {
			return err
		}
	}
	return nil
}

func artifactMimeType(role string) string {
	if role == billingStatementVersionArtifactRoleNote {
		return "text/plain; charset=utf-8"
	}
	return "text/csv; charset=utf-8"
}

// buildBillingStatementVersionArtifactFiles 生成汇总与明细 CSV 临时文件并计算摘要。
func buildBillingStatementVersionArtifactFiles(ctx context.Context, workDir string, v *model.BillingStatementVersion, statement *model.BillingCustomerStatement) ([]stagedBillingStatementVersionFile, error) {
	scope := customerExportScopeColumns{Language: v.Language,
		QuotaPerUnit: v.QuotaPerUnit, Currency: v.Currency, CurrencyRate: v.CurrencyRate,
		JobID: v.DraftPublicId, ExportType: "billing_statement_version",
		GeneratedAt: v.UpdatedAt, PeriodStart: v.PeriodStart, PeriodEnd: v.PeriodEndExclusive - 1,
		Timezone: v.Timezone, CustomerId: v.UserId,
	}
	files := make([]stagedBillingStatementVersionFile, 0, 2)
	exportFiles, paths, _, err := writeCustomerExportSummaryCsv(workDir, scope, v.Language, *statement)
	if err != nil {
		return nil, err
	}
	for i, f := range exportFiles {
		files = append(files, stagedBillingStatementVersionFile{
			role: billingStatementVersionArtifactRoleSummary, fileName: f.FileName,
			path: paths[i], sizeBytes: f.SizeBytes, lineCount: f.LineCount, sha256: f.Sha256,
		})
	}
	detail, err := writeBillingStatementVersionDetailCsv(ctx, v)
	if err != nil {
		return nil, err
	}
	if detail != nil {
		files = append(files, *detail)
	}
	return files, nil
}

// 从冻结的客户事实制表，复用原导出的金额换算、未知值、退款符号与 CSV 转义。
func writeBillingStatementVersionDetailCsv(ctx context.Context, v *model.BillingStatementVersion) (*stagedBillingStatementVersionFile, error) {
	dir := filepath.Join(os.TempDir(), "billing-statement-version-"+v.DraftPublicId)
	writer, err := newCustomerExportCsvWriter(dir, "statement-detail", customerExportTotalBytes)
	if err != nil {
		return nil, err
	}
	success := false
	defer func() {
		if !success {
			writer.Cleanup()
		}
	}()
	writer.useStatementDetails(v.Language)
	if err := writer.ensureShard(); err != nil {
		return nil, err
	}
	scope := customerExportScopeColumns{Language: v.Language, QuotaPerUnit: v.QuotaPerUnit, Currency: v.Currency, CurrencyRate: v.CurrencyRate, JobID: v.DraftPublicId, ExportType: "billing_statement_version", GeneratedAt: v.UpdatedAt, PeriodStart: v.PeriodStart, PeriodEnd: v.PeriodEndExclusive - 1, Timezone: v.Timezone, CustomerId: v.UserId}
	var cursor int64
	integrity := model.BillingStatementIntegrity{}
	for {
		batchCtx, cancel := context.WithTimeout(ctx, customerExportBatchTimeout)
		var lines []model.BillingStatementVersionLine
		err := model.DB.WithContext(batchCtx).Where("version_id = ? AND id > ?", v.ID, cursor).Order("id asc").Limit(500).Find(&lines).Error
		cancel()
		if err != nil {
			return nil, err
		}
		if len(lines) == 0 {
			break
		}
		for _, line := range lines {
			var facts model.CustomerExportRow
			if err := common.UnmarshalJsonStr(line.Facts, &facts); err != nil {
				return nil, err
			}
			if err := writer.AppendRow(facts, scope); err != nil {
				return nil, err
			}
			if writer.totalBytes+writer.bytesInShard > customerExportTotalBytes {
				return nil, errCustomerExportBudgetExceeded
			}
			integrity.Rows++
			if line.LogType == model.LogTypeConsume {
				integrity.Gross += line.Quota
			} else if line.LogType == model.LogTypeRefund {
				integrity.Refund += line.Quota
			}
		}
		cursor = lines[len(lines)-1].ID
	}
	var expected model.BillingStatementIntegrity
	if err := common.UnmarshalJsonStr(v.Integrity, &expected); err != nil {
		return nil, err
	}
	if integrity != expected {
		return nil, fmt.Errorf("statement CSV does not match frozen detail totals")
	}
	files, paths, count, _, err := writer.Finish()
	if err != nil {
		return nil, err
	}
	if len(files) != 1 || count != integrity.Rows {
		return nil, fmt.Errorf("statement CSV is incomplete")
	}
	success = true
	return &stagedBillingStatementVersionFile{role: billingStatementVersionArtifactRoleDetail, fileName: files[0].FileName, path: paths[0], sizeBytes: files[0].SizeBytes, lineCount: count, sha256: files[0].Sha256}, nil
}

// FinalizeBillingStatementVersionArtifacts 确认提交后定稿产物（方案 12.2）：
// 保留类别 draft → confirmed，并生成绑定正式版本号的确认说明文件。
// 说明文件失败只影响该文件下载，不回退确认事实。
func FinalizeBillingStatementVersionArtifacts(ctx context.Context, v *model.BillingStatementVersion) error {
	if v.Status != model.BillingStatementVersionConfirmed || v.VersionNumber == nil {
		return model.ErrBillingStatementVersionConflict
	}
	store, err := currentExportObjectStore()
	if err != nil {
		return err
	}
	content := buildBillingStatementVersionConfirmationNote(v)
	fileName := fmt.Sprintf("confirmation-note-v%d.txt", *v.VersionNumber)
	objectKey := model.BillingStatementNamespace + "/" + v.DraftPublicId + "/" + fileName
	digest := hexSHA256([]byte(content))
	now := common.GetTimestamp()
	artifact := model.BillingStatementArtifact{
		VersionId: v.ID, Role: billingStatementVersionArtifactRoleNote, Language: v.Language,
		FormatVersion: customerExportFieldVersion, ObjectKey: objectKey, FileName: fileName,
		SizeBytes: int64(len(content)), LineCount: 1, Sha256: digest,
		StoreIdentity: store.ExportIdentity(), RetentionClass: "confirmed", Staged: true, CreatedAt: now,
	}
	if err := model.StageBillingStatementConfirmationNote(ctx, &artifact); err != nil {
		return err
	}
	if artifact.StoreIdentity != store.ExportIdentity() {
		return ErrCustomerExportStorageUnavailable
	}
	if artifact.Sha256 != digest || artifact.ObjectKey != objectKey {
		return errors.New("confirmation note manifest differs from frozen version")
	}
	if !artifact.Staged {
		return nil
	}
	notePath, err := writeBillingStatementVersionNoteTemp(v.DraftPublicId, content)
	if err != nil {
		return err
	}
	defer func() { _ = os.Remove(notePath) }()
	if err := store.ExportPutFile(ctx, objectKey, "text/plain; charset=utf-8", notePath, artifact.SizeBytes); err != nil {
		return err
	}
	readable := artifact
	readable.Staged = false
	if err := verifyBillingStatementArtifact(ctx, store, readable); err != nil {
		return err
	}
	if err := model.DB.WithContext(ctx).Model(&model.BillingStatementArtifact{}).
		Where("id = ?", artifact.ID).Updates(map[string]any{"staged": false, "uploaded_at": common.GetTimestamp()}).Error; err != nil {
		return err
	}
	return nil
}

// writeBillingStatementVersionNoteTemp 把确认说明写入临时文件，复用有界文件上传。
func writeBillingStatementVersionNoteTemp(draftPublicId string, content string) (string, error) {
	dir := filepath.Join(os.TempDir(), "billing-statement-version-"+draftPublicId)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	file, err := os.CreateTemp(dir, "note-*")
	if err != nil {
		return "", err
	}
	defer func() { _ = file.Close() }()
	if _, err := file.WriteString(content); err != nil {
		return "", err
	}
	return file.Name(), nil
}

// buildBillingStatementVersionConfirmationNote 生成与快照 ID 绑定的确认说明文本。
func buildBillingStatementVersionConfirmationNote(v *model.BillingStatementVersion) string {
	var b strings.Builder
	fmt.Fprintf(&b, "billing_statement_version=%d\n", *v.VersionNumber)
	fmt.Fprintf(&b, "snapshot_id=%s\n", v.DraftPublicId)
	fmt.Fprintf(&b, "user_id=%d\n", v.UserId)
	fmt.Fprintf(&b, "period_start=%s\n", formatExportTimestamp(v.PeriodStart))
	fmt.Fprintf(&b, "period_end=%s\n", formatExportTimestamp(v.PeriodEndExclusive-1))
	fmt.Fprintf(&b, "confirmed_at=%s\n", formatExportTimestamp(v.ConfirmedAt))
	if v.PublicReason != "" {
		fmt.Fprintf(&b, "public_reason=%s\n", v.PublicReason)
	}
	return b.String()
}

// PresignBillingStatementVersionArtifact 为已鉴权的下载签发短时对象地址（方案 12.6）。
// role 为空时默认汇总 CSV；只发布已上传（非 staged）的产物。
func PresignBillingStatementVersionArtifact(ctx context.Context, v *model.BillingStatementVersion, role string) (string, string, int64, error) {
	if role == "" {
		role = billingStatementVersionArtifactRoleSummary
	}
	if role == billingStatementVersionArtifactRoleNote {
		if err := FinalizeBillingStatementVersionArtifacts(ctx, v); err != nil {
			return "", "", 0, err
		}
	}
	store, err := currentExportObjectStore()
	if err != nil {
		return "", "", 0, err
	}
	var artifact model.BillingStatementArtifact
	err = model.DB.WithContext(ctx).
		Where("version_id = ? AND role = ? AND staged = ?", v.ID, role, false).
		Order("id desc").First(&artifact).Error
	if err != nil {
		return "", "", 0, err
	}
	if artifact.StoreIdentity != store.ExportIdentity() {
		return "", "", 0, ErrCustomerExportStorageUnavailable
	}
	exists, err := store.ExportObjectExists(ctx, artifact.ObjectKey)
	if err != nil || !exists {
		common.SysError("billing statement download file is unavailable")
		return "", "", 0, ErrBillingStatementArtifactUnavailable
	}
	fileName := artifact.FileName
	if v.VersionNumber != nil {
		month := time.Unix(v.PeriodStart, 0).In(time.FixedZone("Asia/Shanghai", 8*3600)).Format("2006-01")
		fileName = fmt.Sprintf("%s-v%d-%s", month, *v.VersionNumber, artifact.FileName)
	}
	url, expiresAt, err := store.ExportPresignURL(artifact.ObjectKey, billingStatementVersionArtifactTTL, fileName)
	if err != nil {
		return "", "", 0, err
	}
	return url, fileName, expiresAt, nil
}

// CleanupBillingStatementDraft 清理失效/失败/已取消草稿（方案 12.3）：
// 先原子标记清理中（使其无法被确认），再删对象，最后事务内删事实；对象删除失败保留引用可重试。
func CleanupBillingStatementDraft(ctx context.Context, draftPublicId string, actorId int) error {
	marked, err := model.MarkBillingStatementDraftCleaning(ctx, draftPublicId, actorId)
	if err != nil {
		return err
	}
	if !marked {
		return model.ErrBillingStatementVersionConflict
	}
	keys, err := billingStatementDraftObjectKeys(ctx, draftPublicId)
	if err != nil {
		return err
	}
	if len(keys) > 0 {
		store, err := currentExportObjectStore()
		if err != nil {
			return err
		}
		var mismatched int64
		if err := model.DB.WithContext(ctx).Model(&model.BillingStatementArtifact{}).Where("object_key IN ? AND store_identity <> ?", keys, store.ExportIdentity()).Count(&mismatched).Error; err != nil {
			return err
		}
		if mismatched > 0 {
			return ErrCustomerExportStorageUnavailable
		}
		for _, key := range keys {
			if err := store.ExportDeleteObject(ctx, key); err != nil {
				return fmt.Errorf("delete billing statement artifact before cleanup: %w", err)
			}
		}
	}
	return model.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		_, err := model.DeleteBillingStatementDraftFactsTx(ctx, tx, draftPublicId)
		return err
	})
}

func billingStatementDraftObjectKeys(ctx context.Context, draftPublicId string) ([]string, error) {
	var keys []string
	err := model.DB.WithContext(ctx).Model(&model.BillingStatementArtifact{}).
		Joins("JOIN billing_statement_versions ON billing_statement_versions.id = billing_statement_artifacts.version_id").
		Where("billing_statement_versions.draft_public_id = ?", draftPublicId).
		Pluck("billing_statement_artifacts.object_key", &keys).Error
	return keys, err
}

// --- 版本差异（方案 6.1 更正对比，第 13 节） ---

// BillingStatementAmountDiff 单项金额对比：nil 表示该侧未知。
type BillingStatementAmountDiff struct {
	BaseUSD    *string `json:"base_usd,omitempty"`
	CompareUSD *string `json:"compare_usd,omitempty"`
	DeltaUSD   *string `json:"delta_usd,omitempty"`
	Base       *int64  `json:"base"`
	Compare    *int64  `json:"compare"`
	Delta      *int64  `json:"delta,omitempty"`
}

// BillingStatementVersionDiff 两个版本之间的结构化差异，全部来自冻结投影。
type BillingStatementCountDiff struct {
	Base    *string `json:"base"`
	Compare *string `json:"compare"`
	Delta   *string `json:"delta"`
}

type BillingStatementDiscountDiff struct {
	Base    []model.BillingDiscountCombination `json:"base"`
	Compare []model.BillingDiscountCombination `json:"compare"`
}

type BillingStatementVersionDiff struct {
	Usage            map[string]BillingStatementCountDiff  `json:"usage"`
	Discounts        BillingStatementDiscountDiff          `json:"discounts"`
	BaseVersionId    int64                                 `json:"base_version_id"`
	CompareVersionId int64                                 `json:"compare_version_id"`
	Items            map[string]BillingStatementAmountDiff `json:"items"`
	Groups           []BillingStatementGroupDiff           `json:"groups"`
	Models           []BillingStatementModelDiff           `json:"models"`
	Quality          map[string]BillingStatementAmountDiff `json:"quality"`
}

type BillingStatementGroupDiff struct {
	GroupId int64                      `json:"group_id"`
	Name    string                     `json:"name"`
	Net     BillingStatementAmountDiff `json:"net"`
}

type BillingStatementModelDiff struct {
	GroupId     int64                      `json:"group_id"`
	ModelName   string                     `json:"model_name"`
	BillingMode string                     `json:"billing_mode"`
	Net         BillingStatementAmountDiff `json:"net"`
}

// ComputeBillingStatementVersionDiff 比较基准版与更正草稿的冻结投影（不读原始来源）。
func ComputeBillingStatementVersionDiff(base *model.BillingStatementVersion, compare *model.BillingStatementVersion) (*BillingStatementVersionDiff, error) {
	baseStatement, err := model.BillingStatementVersionStatement(base)
	if err != nil {
		return nil, err
	}
	compareStatement, err := model.BillingStatementVersionStatement(compare)
	if err != nil {
		return nil, err
	}
	diff := &BillingStatementVersionDiff{
		BaseVersionId:    base.ID,
		CompareVersionId: compare.ID,
		Items:            map[string]BillingStatementAmountDiff{},
		Quality:          map[string]BillingStatementAmountDiff{},
	}
	int64Ptr := func(v int64) *int64 { return &v }
	amountDiff := func(baseVal *int64, compareVal *int64) BillingStatementAmountDiff {
		out := BillingStatementAmountDiff{Base: baseVal, Compare: compareVal}
		if baseVal != nil && compareVal != nil {
			delta := *compareVal - *baseVal
			out.Delta = &delta
		}
		return out
	}
	diff.Items["gross_quota"] = amountDiff(int64Ptr(baseStatement.Summary.GrossQuota), int64Ptr(compareStatement.Summary.GrossQuota))
	diff.Items["refund_quota"] = amountDiff(int64Ptr(baseStatement.Summary.RefundQuota), int64Ptr(compareStatement.Summary.RefundQuota))
	diff.Items["net_quota"] = amountDiff(int64Ptr(baseStatement.Summary.NetQuota), int64Ptr(compareStatement.Summary.NetQuota))
	diff.Items["requests"] = amountDiff(int64Ptr(baseStatement.Summary.Requests), int64Ptr(compareStatement.Summary.Requests))
	diff.Items["original_quota"] = amountDiff(baseStatement.OriginalQuota, compareStatement.OriginalQuota)
	diff.Items["discount_quota"] = amountDiff(baseStatement.DiscountQuota, compareStatement.DiscountQuota)

	diff.Usage = map[string]BillingStatementCountDiff{}
	leftUsage, rightUsage := baseStatement.Summary, compareStatement.Summary
	for _, field := range []struct {
		key         string
		left, right int64
	}{
		{"input_tokens", leftUsage.InputTokens, rightUsage.InputTokens},
		{"output_tokens", leftUsage.OutputTokens, rightUsage.OutputTokens},
		{"cache_read_tokens", leftUsage.CacheReadTokens, rightUsage.CacheReadTokens},
		{"cache_write_tokens", leftUsage.CacheWriteTokens, rightUsage.CacheWriteTokens},
		{"billable_calls", leftUsage.BillableCalls, rightUsage.BillableCalls},
		{"refunded_calls", leftUsage.RefundedCalls, rightUsage.RefundedCalls},
	} {
		left, right, delta := strconv.FormatInt(field.left, 10), strconv.FormatInt(field.right, 10), strconv.FormatInt(field.right-field.left, 10)
		item := BillingStatementCountDiff{Base: &left, Compare: &right, Delta: &delta}
		for _, side := range []struct {
			quality *model.BillingReconciliationDataQuality
			value   **string
		}{{baseStatement.DataQuality, &item.Base}, {compareStatement.DataQuality, &item.Compare}} {
			if side.quality != nil && ((field.key == "input_tokens" && side.quality.InputTokensUnavailableRequests > 0) || (field.key == "cache_write_tokens" && side.quality.CacheWriteUnavailableRequests > 0)) {
				*side.value = nil
				item.Delta = nil
			}
		}
		diff.Usage[field.key] = item
	}
	diff.Discounts = BillingStatementDiscountDiff{Base: baseStatement.DiscountCombinations, Compare: compareStatement.DiscountCombinations}

	baseGroups := map[int64]model.BillingReconciliationGroupSummary{}
	for _, g := range baseStatement.Groups {
		baseGroups[g.Id] = g
	}
	seenGroups := map[int64]bool{}
	for _, g := range compareStatement.Groups {
		seenGroups[g.Id] = true
		var baseNet int64
		name := g.Name
		if bg, ok := baseGroups[g.Id]; ok {
			baseNet = bg.Usage.NetQuota
			if name == "" {
				name = bg.Name
			}
		}
		diff.Groups = append(diff.Groups, BillingStatementGroupDiff{
			GroupId: g.Id, Name: name,
			Net: amountDiff(int64Ptr(baseNet), int64Ptr(g.Usage.NetQuota)),
		})
	}
	for _, g := range baseStatement.Groups {
		if !seenGroups[g.Id] {
			diff.Groups = append(diff.Groups, BillingStatementGroupDiff{
				GroupId: g.Id, Name: g.Name,
				Net: amountDiff(int64Ptr(g.Usage.NetQuota), int64Ptr(0)),
			})
		}
	}

	type modelKey struct {
		groupId     int64
		modelName   string
		billingMode string
	}
	baseModels := map[modelKey]int64{}
	for _, g := range baseStatement.Groups {
		for _, m := range g.Models {
			baseModels[modelKey{g.Id, m.ModelName, m.BillingMode}] = m.Usage.NetQuota
		}
	}
	seenModels := map[modelKey]bool{}
	for _, g := range compareStatement.Groups {
		for _, m := range g.Models {
			key := modelKey{g.Id, m.ModelName, m.BillingMode}
			seenModels[key] = true
			diff.Models = append(diff.Models, BillingStatementModelDiff{
				GroupId: g.Id, ModelName: m.ModelName, BillingMode: m.BillingMode,
				Net: amountDiff(int64Ptr(baseModels[key]), int64Ptr(m.Usage.NetQuota)),
			})
		}
	}
	for _, g := range baseStatement.Groups {
		for _, m := range g.Models {
			key := modelKey{g.Id, m.ModelName, m.BillingMode}
			if !seenModels[key] {
				diff.Models = append(diff.Models, BillingStatementModelDiff{
					GroupId: g.Id, ModelName: m.ModelName, BillingMode: m.BillingMode,
					Net: amountDiff(int64Ptr(m.Usage.NetQuota), int64Ptr(0)),
				})
			}
		}
	}

	qualityKey := func(q *model.BillingReconciliationDataQuality) map[string]int64 {
		out := map[string]int64{}
		if q == nil {
			return out
		}
		out["input_tokens_unavailable_requests"] = q.InputTokensUnavailableRequests
		out["unknown_billing_mode_requests"] = q.UnknownBillingModeRequests
		out["unavailable_requests"] = q.UnavailableRequests
		out["cache_write_unavailable_requests"] = q.CacheWriteUnavailableRequests
		out["missing_historical_price_rows"] = q.MissingHistoricalPriceRows
		out["provider_model_fallback_rows"] = q.ProviderModelFallbackRows
		return out
	}
	baseQuality := qualityKey(baseStatement.DataQuality)
	compareQuality := qualityKey(compareStatement.DataQuality)
	for key, compareVal := range compareQuality {
		baseVal := baseQuality[key]
		diff.Quality[key] = amountDiff(int64Ptr(baseVal), int64Ptr(compareVal))
	}
	for key, baseVal := range baseQuality {
		if _, ok := compareQuality[key]; !ok {
			diff.Quality[key] = amountDiff(int64Ptr(baseVal), int64Ptr(0))
		}
	}
	if base.QuotaPerUnit <= 0 || compare.QuotaPerUnit <= 0 {
		return nil, errors.New("invalid frozen quota conversion")
	}
	convert := func(item BillingStatementAmountDiff) BillingStatementAmountDiff {
		var left, right decimal.Decimal
		if item.Base != nil {
			left = decimal.NewFromInt(*item.Base).Div(decimal.NewFromFloat(base.QuotaPerUnit))
			value := left.StringFixed(8)
			item.BaseUSD = &value
		}
		if item.Compare != nil {
			right = decimal.NewFromInt(*item.Compare).Div(decimal.NewFromFloat(compare.QuotaPerUnit))
			value := right.StringFixed(8)
			item.CompareUSD = &value
		}
		if item.Base != nil && item.Compare != nil {
			value := right.Sub(left).StringFixed(8)
			item.DeltaUSD = &value
		}
		return item
	}
	for key, item := range diff.Items {
		if key != "requests" {
			diff.Items[key] = convert(item)
		}
	}
	for i := range diff.Groups {
		diff.Groups[i].Net = convert(diff.Groups[i].Net)
	}
	for i := range diff.Models {
		diff.Models[i].Net = convert(diff.Models[i].Net)
	}
	return diff, nil
}

// SubmitBillingStatementVersionCorrectionJob 受理更正：以当前确认版为基准生成更正草稿
// （方案 6.1/7），客户可见原因与内部备注随草稿冻结，后台生成流程与普通草稿一致。
func SubmitBillingStatementVersionCorrectionJob(ctx context.Context, adminId int, userId int, periodStart int64, periodEndExcl int64, baseVersionId int64, publicReason string, internalNote string, language ...string) (*model.BillingStatementVersion, error) {
	if !model.BillingStatementVersionEnabled() {
		return nil, model.ErrBillingStatementVersionDisabled
	}
	if !model.BillingStatementVersionTopologyOK() {
		return nil, model.ErrBillingStatementVersionTopology
	}
	filters, err := billingStatementGenerationFilters(ctx, adminId, userId, periodStart, periodEndExcl, language...)
	if err != nil {
		return nil, err
	}
	if err := model.VerifyBillingStatementRetention(ctx, userId, periodStart); err != nil {
		return nil, err
	}
	_, draft, err := model.AcquireBillingStatementCorrectionDraft(ctx, userId, periodStart, "Asia/Shanghai", baseVersionId, publicReason, internalNote, adminId, filters)
	if err != nil {
		return nil, err
	}
	_, _, _ = EnqueueSystemTask(model.SystemTaskTypeCustomerExport, nil)
	return draft, nil
}

// 使用既有范围/换算验证，生成任务无需引入第二套资源配置。
func billingStatementGenerationFilters(ctx context.Context, adminId, userId int, start, end int64, language ...string) (model.CustomerExportFilters, error) {
	if _, err := currentExportObjectStore(); err != nil {
		return model.CustomerExportFilters{}, err
	}
	if err := model.AuthorizeCustomerExport(ctx, adminId, userId); err != nil {
		return model.CustomerExportFilters{}, err
	}
	lang := "en"
	if len(language) > 0 {
		lang = language[0]
	}
	filters, err := normalizeCustomerExportFilters(model.CustomerExportJobTypeStatementSummary, CustomerExportRequest{StartTimestamp: start, EndTimestamp: end, Language: lang})
	if err != nil {
		return filters, err
	}
	// 账单是货币金额；积分展示模式不把 USD 金额误标成 tokens。
	if filters.Currency == operation_setting.QuotaDisplayTypeTokens {
		filters.Currency = "USD"
		filters.CurrencyRate = 1
	}
	if filters.Currency == operation_setting.QuotaDisplayTypeCustom {
		filters.Currency = operation_setting.GetCurrencySymbol()
	}
	return filters, nil
}
