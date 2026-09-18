package model

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// 客户月账单确认与版本固化（docs/80-dev/2026-09-17-客户月账单确认与版本固化方案.md）。
// 主库持久化正式版本、明细、产物清单、操作审计、来源修订、来源保留与维护控制；
// OSS 保存不可变下载产物。Redis/进程缓存只加速读取，不是权威。
// 修订表放主库：同库部署（LOG_DB == DB）下消费/退款日志与修订递增可共用本地事务。

const (
	// Increment whenever statement interpretation changes. New drafts freeze
	// this value; old confirmed versions retain their original facts.
	BillingStatementParserVersion = 4

	// BillingStatementNamespace 是正式账单对象的独立私有命名空间（方案 12.1）。
	BillingStatementNamespace = "billing/statements"

	// 能力开关 Option 键（布尔字符串，默认 false）。
	BillingStatementVersionEnabledKey = "BillingStatementVersionEnabled"
)

// 草稿/版本状态机（方案 7）。
type BillingStatementVersionStatus string

const (
	BillingStatementVersionQueued     BillingStatementVersionStatus = "queued"
	BillingStatementVersionGenerating BillingStatementVersionStatus = "generating"
	BillingStatementVersionPending    BillingStatementVersionStatus = "pending"
	BillingStatementVersionConfirmed  BillingStatementVersionStatus = "confirmed"
	BillingStatementVersionFailed     BillingStatementVersionStatus = "failed"
	BillingStatementVersionCancelled  BillingStatementVersionStatus = "cancelled"
	BillingStatementVersionInvalid    BillingStatementVersionStatus = "invalid"
	BillingStatementVersionCleaning   BillingStatementVersionStatus = "cleaning"
)

// 来源保留状态（方案 10.6）：区分“从未有记录”“有记录但已清理”“完整保留”。
type BillingStatementRetentionStatus string

const (
	BillingStatementRetentionUnknown BillingStatementRetentionStatus = "unknown" // 功能启用前，未经核实
	BillingStatementRetentionNone    BillingStatementRetentionStatus = "none"    // 该客户月从未有消费/退款记录
	BillingStatementRetentionIntact  BillingStatementRetentionStatus = "intact"  // 记录完整，未被清理
	BillingStatementRetentionPartial BillingStatementRetentionStatus = "partial" // 部分金额记录已清理
)

var (
	ErrBillingStatementVersionDisabled = errors.New("billing statement version confirmation is disabled")
	ErrBillingStatementVersionTopology = errors.New("billing statement version confirmation requires the same-database topology (LOG_DB == DB)")
	ErrBillingStatementVersionConflict = errors.New("billing statement version state conflict")
)

// BillingStatementMonth 月账单主记录：客户＋账期唯一，管理当前确认版与活动草稿指针。
type BillingStatementMonth struct {
	ID               int64  `json:"id" gorm:"primary_key"`
	UserId           int    `json:"user_id" gorm:"uniqueIndex:uidx_bsm_user_period,priority:1"`
	PeriodStart      int64  `json:"period_start" gorm:"bigint;uniqueIndex:uidx_bsm_user_period,priority:2;index:idx_bsm_period_current,priority:1"`
	Timezone         string `json:"timezone" gorm:"type:varchar(32);default:'Asia/Shanghai'"`
	CurrentVersionId *int64 `json:"current_version_id" gorm:"bigint;index:idx_bsm_period_current,priority:2"`
	ActiveDraftId    *int64 `json:"active_draft_id" gorm:"bigint"`
	RowVersion       int64  `json:"row_version" gorm:"bigint;default:1"`
	CreatedAt        int64  `json:"created_at" gorm:"bigint"`
	UpdatedAt        int64  `json:"updated_at" gorm:"bigint"`
}

func (BillingStatementMonth) TableName() string { return "billing_statement_months" }

// BillingStatementVersion 账单版本：草稿与正式版共用一行，正式版本号只在确认成功时分配。
type BillingStatementVersion struct {
	ID                 int64                         `json:"id" gorm:"primary_key"`
	SourceJobId        string                        `json:"-" gorm:"index"`
	Language           string                        `json:"language"`
	DraftPublicId      string                        `json:"draft_public_id" gorm:"type:varchar(64);uniqueIndex"`
	MonthId            int64                         `json:"month_id" gorm:"index"`
	UserId             int                           `json:"user_id" gorm:"index"`
	PeriodStart        int64                         `json:"period_start" gorm:"bigint"`
	PeriodEndExclusive int64                         `json:"period_end_exclusive" gorm:"bigint"`
	Timezone           string                        `json:"timezone" gorm:"type:varchar(32)"`
	VersionNumber      *int                          `json:"version_number" gorm:"index"` // 确认时分配，失败草稿为空
	CorrectsVersionId  *int64                        `json:"corrects_version_id" gorm:"bigint"`
	Status             BillingStatementVersionStatus `json:"status" gorm:"type:varchar(32);index"`
	ParserVersion      int                           `json:"parser_version" gorm:"default:1"`
	// 来源版本向量（生成时冻结）。
	RevScopeCustomerMonth string               `json:"-" gorm:"type:varchar(128)"`
	RevCustomerMonth      int64                `json:"-" gorm:"bigint"`
	RevScopeMaintenance   string               `json:"-" gorm:"type:varchar(128)"`
	RevMaintenance        int64                `json:"-" gorm:"bigint"`
	Dependencies          BillingStatementJSON `json:"-"`
	// 冻结换算/格式参数。
	QuotaPerUnit float64 `json:"quota_per_unit"`
	Currency     string  `json:"currency" gorm:"type:varchar(16)"`
	CurrencyRate float64 `json:"currency_rate"`
	// 冻结汇总及维度投影（JSON，来自 GetBillingCustomerStatement 的白名单结果）。
	SummaryProjection BillingStatementJSON `json:"-"`
	ChannelProjection BillingStatementJSON `json:"-"`
	Integrity         string               `json:"-" gorm:"type:text"` // 完整性校验结果（方案 9）
	// 确认事实。
	ConfirmedBy    int    `json:"confirmed_by" gorm:"default:0"`
	ConfirmedAt    int64  `json:"confirmed_at" gorm:"bigint;default:0"`
	PublicReason   string `json:"public_reason" gorm:"type:text"` // 客户可见更正原因
	InternalNote   string `json:"internal_note" gorm:"type:text"` // 内部备注，不公开
	AcknowledgedQA string `json:"-" gorm:"type:text"`             // 质量知悉记录（JSON）
	CreatedBy      int    `json:"created_by" gorm:"default:0"`
	CreatedAt      int64  `json:"created_at" gorm:"bigint"`
	UpdatedAt      int64  `json:"updated_at" gorm:"bigint"`
}

func (BillingStatementVersion) TableName() string { return "billing_statement_versions" }

// BillingStatementVersionLine 版本明细：version_id + sequence 唯一，保存客户字段白名单与精确 quota。
type BillingStatementVersionLine struct {
	ID               int64  `json:"id" gorm:"primary_key"`
	VersionId        int64  `json:"version_id" gorm:"uniqueIndex:uidx_bsvl_version_seq,priority:1"`
	Sequence         int64  `json:"sequence" gorm:"bigint;uniqueIndex:uidx_bsvl_version_seq,priority:2"`
	SourceLogId      int64  `json:"source_log_id" gorm:"bigint"` // 稳定源记录身份
	RequestId        string `json:"request_id" gorm:"type:varchar(64);index"`
	TokenId          int    `json:"token_id" gorm:"index"`
	TokenName        string `json:"token_name" gorm:"type:varchar(128)"`
	ChannelId        int    `json:"channel_id" gorm:"index"`
	CustomerModel    string `json:"customer_model" gorm:"type:varchar(128);index"`
	Group            string `json:"group" gorm:"type:varchar(128)"`
	LogType          int    `json:"log_type"`
	CreatedAt        int64  `json:"created_at" gorm:"bigint;index"`
	BillingMode      string `json:"billing_mode" gorm:"type:varchar(32);index"`
	InputTokens      int64  `json:"input_tokens"`
	OutputTokens     int64  `json:"output_tokens"`
	CacheReadTokens  int64  `json:"cache_read_tokens"`
	CacheWriteTokens int64  `json:"cache_write_tokens"`
	Quota            int64  `json:"quota"`
	// 折扣三态与费用解释质量（parsed 结果的冻结投影，JSON）。
	Facts string `json:"-" gorm:"type:text"`
}

func (BillingStatementVersionLine) TableName() string { return "billing_statement_version_lines" }

// BillingStatementArtifact 版本产物清单：staged 先行登记，上传成功才发布。
type BillingStatementArtifact struct {
	ID             int64  `json:"id" gorm:"primary_key"`
	VersionId      int64  `json:"version_id" gorm:"index"`
	Role           string `json:"role" gorm:"type:varchar(32)"` // summary_csv / detail_csv / correction_note
	Language       string `json:"language" gorm:"type:varchar(16)"`
	FormatVersion  int    `json:"format_version"`
	ObjectKey      string `json:"object_key" gorm:"type:varchar(255);uniqueIndex"`
	FileName       string `json:"file_name" gorm:"type:varchar(255)"`
	SizeBytes      int64  `json:"size_bytes"`
	LineCount      int64  `json:"line_count"`
	Sha256         string `json:"sha256" gorm:"type:varchar(64)"`
	StoreIdentity  string `json:"store_identity" gorm:"type:varchar(64)"`
	RetentionClass string `json:"retention_class" gorm:"type:varchar(32)"` // draft / confirmed
	Staged         bool   `json:"staged"`
	UploadedAt     int64  `json:"uploaded_at" gorm:"bigint;default:0"`
	CreatedAt      int64  `json:"created_at" gorm:"bigint"`
}

func (BillingStatementArtifact) TableName() string { return "billing_statement_artifacts" }

// BillingStatementAudit 账单操作审计：只追加，不更新不删除。
type BillingStatementAudit struct {
	ID             int64  `json:"id" gorm:"primary_key"`
	VersionId      *int64 `json:"version_id" gorm:"bigint"`
	MonthId        *int64 `json:"month_id" gorm:"bigint"`
	Action         string `json:"action" gorm:"type:varchar(32);index"` // generate/invalidate/abandon/confirm/correct/fail
	ActorId        int    `json:"actor_id" gorm:"index"`
	IdempotencyKey string `json:"idempotency_key" gorm:"type:varchar(64);index"`
	Reason         string `json:"reason" gorm:"type:text"`
	Result         string `json:"result" gorm:"type:text"`
	CreatedAt      int64  `json:"created_at" gorm:"bigint;index"`
}

func (BillingStatementAudit) TableName() string { return "billing_statement_audits" }

// BillingStatementRevision 来源修订记录：稳定作用域内的单调修订号，服务方案第 10 节。不是资金账本。
// Scope 编码：
//
//	cm:<userId>:<periodStart>            客户＋自然月的消费/退款日志
//	ev:<userId>:<tokenId|0>              退款证据（预扣关联 + 历史退款候选）作用域
//	task:<userId>                        Task 计费证据（按客户收敛）
//	maintenance                          维护控制（全局，固定主键）
type BillingStatementRevision struct {
	ID         int64  `json:"id" gorm:"primary_key"`
	Scope      string `json:"scope" gorm:"type:varchar(128);uniqueIndex"`
	Revision   int64  `json:"revision" gorm:"bigint;default:0"`
	Generation int64  `json:"generation" gorm:"bigint;default:1"` // 代际：备份恢复/维护后递增，防止旧草稿误通过
	UpdatedAt  int64  `json:"updated_at" gorm:"bigint"`
}

func (BillingStatementRevision) TableName() string { return "billing_statement_revisions" }

// BillingStatementRetention 来源保留状态（方案 10.6）：回答“当前来源是否仍足够重建该月”，与修订号互补。
type BillingStatementRetention struct {
	ID          int64                           `json:"id" gorm:"primary_key"`
	UserId      int                             `json:"user_id" gorm:"uniqueIndex:uidx_bsret_user_period,priority:1"`
	PeriodStart int64                           `json:"period_start" gorm:"bigint;uniqueIndex:uidx_bsret_user_period,priority:2"`
	Status      BillingStatementRetentionStatus `json:"status" gorm:"type:varchar(16);index"`
	Detail      BillingStatementJSON            `json:"-"` // 受影响范围说明
	UpdatedAt   int64                           `json:"updated_at" gorm:"bigint"`
}

func (BillingStatementRetention) TableName() string { return "billing_statement_retentions" }

// BillingStatementMaintenance 维护控制记录（方案 10.7）：协调确认发布与关开关/进入维护的先后。
// 固定单行（ID=1）。产品开关以 Option 为配置权威，维护状态表达正在执行的操作。
type BillingStatementMaintenance struct {
	ID         int64  `json:"id" gorm:"primary_key"`
	Enabled    bool   `json:"enabled"`                  // 是否处于维护中（阻止发布）
	Generation int64  `json:"generation" gorm:"bigint"` // 单调代际
	Reason     string `json:"reason" gorm:"type:text"`
	UpdatedAt  int64  `json:"updated_at" gorm:"bigint"`
}

func (BillingStatementMaintenance) TableName() string { return "billing_statement_maintenance" }

const billingStatementMaintenanceRowID = 1

// nowMillis 统一时间戳（秒）。
func nowSeconds() int64 { return time.Now().Unix() }

// migrateBillingStatementVersionDB 对版本固化相关模型执行 AutoMigrate，兼容 SQLite/MySQL/PostgreSQL。
// 同时幂等种子维护控制行（固定主键 1），供确认与维护切换锁定。
func migrateBillingStatementVersionDB() error {
	if err := DB.AutoMigrate(
		&BillingStatementMonth{},
		&BillingStatementVersion{},
		&BillingStatementVersionLine{},
		&BillingStatementArtifact{},
		&BillingStatementAudit{},
		&BillingStatementRevision{},
		&BillingStatementRetention{},
		&BillingStatementMaintenance{},
	); err != nil {
		return err
	}
	if err := ensureBillingStatementMaintenanceRow(context.Background()); err != nil {
		return err
	}
	return nil
}

// ensureBillingStatementMaintenanceRow 幂等创建维护控制行。
func ensureBillingStatementMaintenanceRow(ctx context.Context) error {
	var count int64
	if err := DB.WithContext(ctx).Model(&BillingStatementMaintenance{}).Where("id = ?", billingStatementMaintenanceRowID).Count(&count).Error; err != nil {
		return err
	}
	if count > 0 {
		return nil
	}
	return DB.WithContext(ctx).Create(&BillingStatementMaintenance{
		ID: billingStatementMaintenanceRowID, Enabled: false, Generation: 1, UpdatedAt: nowSeconds(),
	}).Error
}

// --- 能力开关与拓扑 ---

// BillingStatementVersionEnabled 读取运行时能力开关（Option 布尔字符串，默认 false）。
func BillingStatementVersionEnabled() bool {
	common.OptionMapRWMutex.RLock()
	defer common.OptionMapRWMutex.RUnlock()
	raw, ok := common.OptionMap[BillingStatementVersionEnabledKey]
	if !ok {
		return false
	}
	return raw == "true" || raw == "1"
}

// BillingStatementVersionTopologyOK 报告当前是否为同库部署（LOG_DB == DB）。
// 首期仅在该拓扑启用生成/确认；分库保留实时查询与已有版本读取。
func BillingStatementVersionTopologyOK() bool { return LOG_DB == DB }

// --- 来源修订 ---

// IncrementBillingStatementRevisionTx 在给定事务内原子递增指定作用域的修订号（插入或 +1）。
// 必须在业务行变化的同一事务内调用，否则不满足方案 10.2 的“同事务”要求。
func IncrementBillingStatementRevisionTx(tx *gorm.DB, scope string) error {
	return tx.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "scope"}}, DoUpdates: clause.Assignments(map[string]interface{}{"revision": gorm.Expr("? + 1", clause.Column{Table: "billing_statement_revisions", Name: "revision"}), "updated_at": nowSeconds()})}).Create(&BillingStatementRevision{Scope: scope, Revision: 1, Generation: 1, UpdatedAt: nowSeconds()}).Error
}

// --- 维护控制（方案 10.7） ---

// GetBillingStatementMaintenance 读取维护控制行（不存在则惰性创建）。
func GetBillingStatementMaintenance(ctx context.Context) (*BillingStatementMaintenance, error) {
	var m BillingStatementMaintenance
	err := DB.WithContext(ctx).First(&m, billingStatementMaintenanceRowID).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		m = BillingStatementMaintenance{ID: billingStatementMaintenanceRowID, Generation: 1, UpdatedAt: nowSeconds()}
		if cerr := DB.WithContext(ctx).Create(&m).Error; cerr != nil {
			return nil, cerr
		}
		return &m, nil
	}
	return &m, err
}

// lockBillingStatementMaintenanceTx 在事务内锁定维护控制行（固定主键），供确认与开关切换共用。
func lockBillingStatementMaintenanceTx(ctx context.Context, tx *gorm.DB) (*BillingStatementMaintenance, error) {
	if common.UsingMainDatabase(common.DatabaseTypeSQLite) {
		if err := tx.WithContext(ctx).Model(&BillingStatementMaintenance{}).Where("id = ?", billingStatementMaintenanceRowID).Update("updated_at", nowSeconds()).Error; err != nil {
			return nil, err
		}
	}
	var m BillingStatementMaintenance
	if err := lockForUpdate(tx.WithContext(ctx)).First(&m, billingStatementMaintenanceRowID).Error; err != nil {
		return nil, err
	}
	return &m, nil
}

// BeginBillingStatementMaintenance 进入维护：锁定控制行、校验当前非维护中、递增代际并标记维护中。
// 确认与维护遵循一致锁顺序（先控制行），避免死锁。
func BeginBillingStatementMaintenance(ctx context.Context, reason string, actorID int) (*BillingStatementMaintenance, error) {
	if strings.TrimSpace(reason) == "" || actorID <= 0 {
		return nil, ErrBillingStatementVersionConflict
	}
	var out *BillingStatementMaintenance
	err := DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		m, err := lockBillingStatementMaintenanceTx(ctx, tx)
		if err != nil {
			return err
		}
		if m.Enabled {
			return errors.New("billing statement maintenance already in progress")
		}
		m.Enabled = true
		m.Generation++
		m.Reason = reason
		m.UpdatedAt = nowSeconds()
		if err := tx.Where(Option{Key: BillingStatementVersionEnabledKey}).Assign(Option{Value: "false"}).FirstOrCreate(&Option{}).Error; err != nil {
			return err
		}
		if err := tx.Save(m).Error; err != nil {
			return err
		}
		if err := tx.Create(&BillingStatementAudit{Action: "maintenance_begin", ActorId: actorID, Reason: reason, Result: fmt.Sprint(m.Generation), CreatedAt: nowSeconds()}).Error; err != nil {
			return err
		}
		out = m
		return nil
	})
	return out, err
}

// EndBillingStatementMaintenance 退出维护：校验维护中、递增代际并清除维护标记。
func EndBillingStatementMaintenance(ctx context.Context, expectedGeneration int64, evidence string, actorID int) (*BillingStatementMaintenance, error) {
	if strings.TrimSpace(evidence) == "" || actorID <= 0 {
		return nil, ErrBillingStatementVersionConflict
	}
	var out *BillingStatementMaintenance
	err := DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		m, err := lockBillingStatementMaintenanceTx(ctx, tx)
		if err != nil {
			return err
		}
		if !m.Enabled || expectedGeneration <= 0 || m.Generation != expectedGeneration {
			return errors.New("billing statement maintenance not in progress")
		}
		if enabled, err := billingStatementVersionEnabledFromDBTx(ctx, tx); err != nil {
			return err
		} else if enabled {
			return ErrBillingStatementVersionConflict
		}
		m.Enabled = false
		m.Generation++
		m.Reason = ""
		m.UpdatedAt = nowSeconds()
		if err := tx.Save(m).Error; err != nil {
			return err
		}
		if err := tx.Create(&BillingStatementAudit{Action: "maintenance_end", ActorId: actorID, Reason: evidence, Result: fmt.Sprint(m.Generation), CreatedAt: nowSeconds()}).Error; err != nil {
			return err
		}
		out = m
		return nil
	})
	return out, err
}

// --- 草稿占用与确认发布 ---

// AcquireBillingStatementDraft 原子占用某客户月份的活动草稿位置。
// 返回月记录与新建的草稿版本；若已有活动草稿则返回 ErrBillingStatementVersionConflict。
func AcquireBillingStatementDraft(ctx context.Context, userId int, periodStart int64, timezone string, createdBy int, generation ...CustomerExportFilters) (*BillingStatementMonth, *BillingStatementVersion, error) {
	var month *BillingStatementMonth
	var draft *BillingStatementVersion
	err := DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// 锁月记录（唯一客户＋账期），无则创建。
		m := BillingStatementMonth{}
		findErr := lockForUpdate(tx.WithContext(ctx)).
			Where("user_id = ? AND period_start = ?", userId, periodStart).First(&m).Error
		if errors.Is(findErr, gorm.ErrRecordNotFound) {
			m = BillingStatementMonth{UserId: userId, PeriodStart: periodStart, Timezone: timezone, RowVersion: 1, CreatedAt: nowSeconds(), UpdatedAt: nowSeconds()}
			if err := tx.Create(&m).Error; err != nil {
				return err
			}
		} else if findErr != nil {
			return findErr
		} else {
			if m.ActiveDraftId != nil || m.CurrentVersionId != nil {
				return ErrBillingStatementVersionConflict
			}
		}
		now := nowSeconds()
		d := BillingStatementVersion{
			ParserVersion:      BillingStatementParserVersion,
			DraftPublicId:      "bsv_" + common.GetRandomString(24),
			MonthId:            m.ID,
			UserId:             userId,
			PeriodStart:        periodStart,
			PeriodEndExclusive: periodStart,
			Timezone:           timezone,
			Status:             BillingStatementVersionQueued,
			CreatedBy:          createdBy,
			CreatedAt:          now,
			UpdatedAt:          now,
		}
		if len(generation) > 0 {
			if err := queueBillingStatementGenerationTx(tx, &d, generation[0]); err != nil {
				return err
			}
		}
		if err := tx.Create(&d).Error; err != nil {
			return err
		}
		m.ActiveDraftId = &d.ID
		m.RowVersion++
		m.UpdatedAt = now
		if err := tx.Save(&m).Error; err != nil {
			return err
		}
		month = &m
		draft = &d
		return nil
	})
	if err != nil {
		if m, readErr := GetBillingStatementMonthByUserPeriod(ctx, userId, periodStart); readErr == nil && m != nil && m.ActiveDraftId != nil {
			return nil, nil, ErrBillingStatementVersionConflict
		}
		return nil, nil, err
	}
	return month, draft, nil
}

// billingStatementDependencyVector 记录版本冻结的来源版本向量。
type billingStatementDependencyVector struct {
	Dependencies       BillingStatementJSON
	ScopeCustomerMonth string
	RevCustomerMonth   int64
	ScopeMaintenance   string
	RevMaintenance     int64
}

// snapshotBillingStatementDependencyVector 生成时读取并冻结各来源作用域的当前修订号。
// 同时覆盖“未找到”的查询作用域（方案 10.2）：证据依赖作用域须按客户＋Key 预登记。
func snapshotBillingStatementDependencyVector(ctx context.Context, userId int, periodStart int64, policies ...BillingStatementReadPolicy) (*billingStatementDependencyVector, error) {
	vec := &billingStatementDependencyVector{
		ScopeCustomerMonth: fmt.Sprintf("cm:%d:%d", userId, periodStart),
		ScopeMaintenance:   "maintenance",
	}
	read := func(scope string) (int64, error) {
		if err := DB.WithContext(ctx).Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "scope"}}, DoNothing: true}).Create(&BillingStatementRevision{Scope: scope, Generation: 1}).Error; err != nil {
			return 0, err
		}
		var r BillingStatementRevision
		err := DB.WithContext(ctx).Select("revision").Where("scope = ?", scope).First(&r).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return 0, nil
		}
		return r.Revision, err
	}
	var err error
	if vec.RevCustomerMonth, err = read(vec.ScopeCustomerMonth); err != nil {
		return nil, err
	}
	scopes, err := billingStatementEvidenceScopes(ctx, userId, periodStart, policies...)
	if err != nil {
		return nil, err
	}
	dependencies := map[string]int64{}
	if len(scopes) > 0 {
		timeout := 5 * time.Second
		if len(policies) > 0 && policies[0].BatchTimeout > 0 {
			timeout = policies[0].BatchTimeout
		}
		registrationCtx, cancel := context.WithTimeout(ctx, timeout)
		err = DB.WithContext(registrationCtx).Transaction(func(tx *gorm.DB) error {
			if err := lockBillingSourceUser(tx, userId); err != nil {
				return err
			}
			revisions := make([]BillingStatementRevision, 0, len(scopes))
			for _, scope := range scopes {
				revisions = append(revisions, BillingStatementRevision{Scope: scope, Generation: 1})
			}
			if err := tx.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "scope"}}, DoNothing: true}).CreateInBatches(&revisions, 100).Error; err != nil {
				return err
			}
			for start := 0; start < len(scopes); start += 500 {
				var registered []BillingStatementRevision
				batch := scopes[start:min(start+500, len(scopes))]
				if err := tx.Select("scope, revision").Where("scope IN ?", batch).Find(&registered).Error; err != nil {
					return err
				}
				if len(registered) != len(batch) {
					return ErrBillingStatementVersionConflict
				}
				for _, revision := range registered {
					dependencies[revision.Scope] = revision.Revision
				}
			}
			return nil
		})
		cancel()
		if err != nil {
			return nil, err
		}
	}
	raw, err := common.Marshal(dependencies)
	if err != nil {
		return nil, err
	}
	vec.Dependencies = BillingStatementJSON(raw)
	// 维护代际权威是 maintenance 控制行；revision 的 maintenance scope 仅作冗余校验。
	if vec.RevMaintenance, err = read(vec.ScopeMaintenance); err != nil {
		return nil, err
	}
	m, err := GetBillingStatementMaintenance(ctx)
	if err != nil {
		return nil, err
	}
	if m.Enabled {
		return nil, ErrBillingStatementVersionDisabled
	}
	vec.RevMaintenance = m.Generation
	return vec, nil
}

// ConfirmBillingStatementVersion 原子确认发布（方案 10.3/10.7）。
// 在同一事务内：锁维护控制行 → 校验开关/维护状态/代际 → 锁月记录 → 校验草稿状态/来源向量/完整性 →
// 分配正式版本号 → 切换当前版指针 → 追加审计。返回是否本次提交（幂等重试返回同一结果）。
func ConfirmBillingStatementVersion(ctx context.Context, draftPublicId string, expectedBaseVersionId *int64, idempotencyKey string, acknowledgedQA string, publicReason string, confirmedBy int) (*BillingStatementVersion, bool, error) {
	var result *BillingStatementVersion
	var committed bool
	var parserChanged bool
	err := DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// 1. 锁维护控制行，读取开关与维护状态（最终权威，不依赖进程缓存）。
		maint, err := lockBillingStatementMaintenanceTx(ctx, tx)
		if err != nil {
			return err
		}
		// 2. 读草稿。
		var v BillingStatementVersion
		if err := lockForUpdate(tx.WithContext(ctx)).Where("draft_public_id = ?", draftPublicId).First(&v).Error; err != nil {
			return err
		}
		// 3. 幂等：已确认则返回既有结果，不重复发布。
		if v.Status == BillingStatementVersionConfirmed {
			result = &v
			committed = false
			return nil
		}
		if maint.Enabled {
			return ErrBillingStatementVersionDisabled
		}
		// 开关最终权威是主库 option 行（与维护控制行同事务读取），不依赖进程缓存。
		enabled, err := billingStatementVersionEnabledFromDBTx(ctx, tx)
		if err != nil {
			return err
		}
		if !enabled {
			return ErrBillingStatementVersionDisabled
		}
		// 4. 状态、完整性与来源校验。
		if !BillingStatementVersionTopologyOK() {
			return ErrBillingStatementVersionTopology
		}
		if v.Status != BillingStatementVersionPending {
			return fmt.Errorf("draft status %q not confirmable", v.Status)
		}
		if v.ParserVersion != BillingStatementParserVersion {
			if err := tx.Model(&v).Updates(map[string]any{"status": BillingStatementVersionInvalid, "updated_at": nowSeconds()}).Error; err != nil {
				return err
			}
			if err := tx.Model(&BillingStatementMonth{}).Where("id = ? AND active_draft_id = ?", v.MonthId, v.ID).Update("active_draft_id", nil).Error; err != nil {
				return err
			}
			if err := tx.Create(&BillingStatementAudit{VersionId: &v.ID, MonthId: &v.MonthId, Action: "invalidate", ActorId: confirmedBy, Reason: "parser version changed", Result: "invalid", CreatedAt: nowSeconds()}).Error; err != nil {
				return err
			}
			parserChanged = true
			return nil // Commit invalidation, then report a conflict to the caller.
		}
		if v.CorrectsVersionId != nil && (expectedBaseVersionId == nil || *expectedBaseVersionId != *v.CorrectsVersionId) {
			return ErrBillingStatementVersionConflict
		}
		if expectedBaseVersionId != nil {
			if v.CorrectsVersionId == nil || *v.CorrectsVersionId != *expectedBaseVersionId {
				return ErrBillingStatementVersionConflict
			}
		}
		if err := lockBillingSourceUser(tx, v.UserId); err != nil {
			return fmt.Errorf("%w: %v", ErrBillingStatementSourceIncomplete, err)
		}
		if err := assertBillingStatementSourceUnchangedTx(ctx, tx, &v, maint.Generation); err != nil {
			return err
		}
		if err := verifyBillingStatementConfirmationTx(ctx, tx, &v, acknowledgedQA); err != nil {
			return err
		}
		// 5. 锁月记录并校验活动草稿指针。
		var m BillingStatementMonth
		if err := lockForUpdate(tx.WithContext(ctx)).First(&m, v.MonthId).Error; err != nil {
			return err
		}
		if m.ActiveDraftId == nil || *m.ActiveDraftId != v.ID {
			return ErrBillingStatementVersionConflict
		}
		// 6. 分配正式版本号（该客户月内最大 + 1）。
		var maxNum int
		if err := tx.WithContext(ctx).Model(&BillingStatementVersion{}).
			Where("month_id = ? AND version_number IS NOT NULL", m.ID).
			Select("COALESCE(MAX(version_number),0)").Scan(&maxNum).Error; err != nil {
			return err
		}
		now := nowSeconds()
		vn := maxNum + 1
		v.VersionNumber = &vn
		v.Status = BillingStatementVersionConfirmed
		v.ConfirmedBy = confirmedBy
		v.ConfirmedAt = now
		if publicReason != "" {
			v.PublicReason = publicReason
		}
		if v.CorrectsVersionId != nil && v.PublicReason == "" {
			return errors.New("correction reason is required")
		}
		v.AcknowledgedQA = acknowledgedQA
		v.UpdatedAt = now
		if err := tx.Save(&v).Error; err != nil {
			return err
		}
		if err := tx.Model(&BillingStatementArtifact{}).Where("version_id = ?", v.ID).Update("retention_class", "confirmed").Error; err != nil {
			return err
		}
		// 7. 切换当前版指针，释放活动草稿。
		m.CurrentVersionId = &v.ID
		m.ActiveDraftId = nil
		m.RowVersion++
		m.UpdatedAt = now
		if err := tx.Save(&m).Error; err != nil {
			return err
		}
		// 8. 追加审计。
		audit := BillingStatementAudit{
			VersionId: &v.ID, MonthId: &m.ID, Action: "confirm",
			ActorId: confirmedBy, IdempotencyKey: idempotencyKey,
			Reason: publicReason, Result: "confirmed",
			CreatedAt: now,
		}
		if err := tx.Create(&audit).Error; err != nil {
			return err
		}
		result = &v
		committed = true
		return nil
	})
	if err != nil {
		return nil, false, err
	}
	if parserChanged {
		return nil, false, ErrBillingStatementVersionConflict
	}
	return result, committed, nil
}

// assertBillingStatementSourceUnchangedTx 在确认事务内复验来源版本向量与维护代际（方案 9/10.2）。
func assertBillingStatementSourceUnchangedTx(ctx context.Context, tx *gorm.DB, v *BillingStatementVersion, maintGeneration int64) error {
	check := func(scope string, frozen int64) error {
		var r BillingStatementRevision
		err := lockForUpdate(tx.WithContext(ctx)).Select("revision,generation").Where("scope = ?", scope).First(&r).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			if frozen == 0 {
				return nil
			}
			return ErrBillingStatementVersionConflict
		}
		if err != nil {
			return err
		}
		if r.Revision != frozen {
			return ErrBillingStatementVersionConflict
		}
		return nil
	}
	if err := check(v.RevScopeCustomerMonth, v.RevCustomerMonth); err != nil {
		return err
	}
	if err := verifyBillingStatementEvidenceRevisions(lockForUpdate(tx.WithContext(ctx)), v); err != nil {
		return err
	}
	// 维护代际：生成后进入维护会使 maintenance 控制行代际递增，旧草稿必须失效。
	// 代际权威是锁定的 maintenance 控制行（与确认共用同一锁）。revision 的 maintenance
	// scope 不作为代际来源，避免与冗余修订号混淆。
	if maintGeneration != v.RevMaintenance {
		return ErrBillingStatementVersionConflict
	}
	return nil
}

// AbandonBillingStatementDraft 放弃草稿（管理员）：仅活动草稿可放弃，释放月记录的活动草稿指针。
func AbandonBillingStatementDraft(ctx context.Context, draftPublicId string, actorId int, reason string) error {
	var sourceJob string
	var owner int
	err := DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var v BillingStatementVersion
		if err := lockForUpdate(tx.WithContext(ctx)).Where("draft_public_id = ?", draftPublicId).First(&v).Error; err != nil {
			return err
		}
		if v.Status != BillingStatementVersionQueued && v.Status != BillingStatementVersionGenerating && v.Status != BillingStatementVersionPending {
			return fmt.Errorf("draft status %q not abandonable", v.Status)
		}
		var m BillingStatementMonth
		if err := lockForUpdate(tx.WithContext(ctx)).First(&m, v.MonthId).Error; err != nil {
			return err
		}
		sourceJob, owner = v.SourceJobId, v.CreatedBy
		now := nowSeconds()
		v.Status = BillingStatementVersionCancelled
		v.UpdatedAt = now
		if err := tx.Save(&v).Error; err != nil {
			return err
		}
		if m.ActiveDraftId != nil && *m.ActiveDraftId == v.ID {
			m.ActiveDraftId = nil
			m.RowVersion++
			m.UpdatedAt = now
			if err := tx.Save(&m).Error; err != nil {
				return err
			}
		}
		return tx.Create(&BillingStatementAudit{
			VersionId: &v.ID, MonthId: &m.ID, Action: "abandon",
			ActorId: actorId, Reason: reason, Result: "cancelled", CreatedAt: now,
		}).Error
	})
	if err != nil {
		return err
	}
	if sourceJob != "" {
		_, err = CancelCustomerExportJob(sourceJob, owner)
		if errors.Is(err, ErrCustomerExportNotCancellable) {
			return nil
		}
	}
	return err
}

// MarkBillingStatementDraftInvalid 标记草稿失效（来源变化或维护代际变化）。
func MarkBillingStatementDraftInvalid(ctx context.Context, draftPublicId string, reason string) error {
	now := nowSeconds()
	res := DB.WithContext(ctx).Model(&BillingStatementVersion{}).
		Where("draft_public_id = ? AND status IN ?", draftPublicId,
			[]BillingStatementVersionStatus{BillingStatementVersionQueued, BillingStatementVersionGenerating, BillingStatementVersionPending}).
		Updates(map[string]any{"status": BillingStatementVersionInvalid, "updated_at": now})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return nil
	}
	var v BillingStatementVersion
	if err := DB.WithContext(ctx).Select("id,month_id").Where("draft_public_id = ?", draftPublicId).First(&v).Error; err != nil {
		return err
	}
	return DB.WithContext(ctx).Create(&BillingStatementAudit{
		VersionId: &v.ID, MonthId: &v.MonthId, Action: "invalidate",
		Reason: reason, Result: "invalid", CreatedAt: now,
	}).Error
}

// MarkBillingStatementDraftFailed 标记生成失败。
func MarkBillingStatementDraftFailed(ctx context.Context, draftPublicId string, errCode string, errMsg string) error {
	now := nowSeconds()
	res := DB.WithContext(ctx).Model(&BillingStatementVersion{}).
		Where("draft_public_id = ? AND status IN ?", draftPublicId,
			[]BillingStatementVersionStatus{BillingStatementVersionQueued, BillingStatementVersionGenerating}).
		Updates(map[string]any{"status": BillingStatementVersionFailed, "updated_at": now})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return nil
	}
	var v BillingStatementVersion
	if err := DB.WithContext(ctx).Select("id,month_id").Where("draft_public_id = ?", draftPublicId).First(&v).Error; err != nil {
		return err
	}
	runes := []rune(errMsg)
	if len(runes) > 500 {
		errMsg = string(runes[:500])
	}
	return DB.WithContext(ctx).Create(&BillingStatementAudit{
		VersionId: &v.ID, MonthId: &v.MonthId, Action: "fail",
		Reason: errCode, Result: errMsg, CreatedAt: now,
	}).Error
}

// CheckBillingStatementSourceChanged 生成过程中复验来源是否已变化（方案 9/11.1 步骤 5）。
// 返回 true 表示应使草稿失效。同时校验维护代际。
func CheckBillingStatementSourceChanged(ctx context.Context, v *BillingStatementVersion) (bool, error) {
	changed, err := billingStatementVectorChanged(ctx, v)
	if err != nil {
		return false, err
	}
	return changed, nil
}

func billingStatementVectorChanged(ctx context.Context, v *BillingStatementVersion) (bool, error) {
	check := func(scope string, frozen int64) (bool, error) {
		var r BillingStatementRevision
		err := DB.WithContext(ctx).Select("revision").Where("scope = ?", scope).First(&r).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return frozen != 0, nil
		}
		if err != nil {
			return false, err
		}
		return r.Revision != frozen, nil
	}
	for _, c := range []struct {
		scope  string
		frozen int64
	}{
		{v.RevScopeCustomerMonth, v.RevCustomerMonth},
	} {
		changed, err := check(c.scope, c.frozen)
		if err != nil || changed {
			return changed, err
		}
	}
	if err := verifyBillingStatementEvidenceRevisions(DB.WithContext(ctx), v); err != nil {
		if errors.Is(err, ErrBillingStatementVersionConflict) {
			return true, nil
		}
		return false, err
	}
	// 维护代际权威是 maintenance 控制行，不走 revision 查询。
	m, err := GetBillingStatementMaintenance(ctx)
	if err != nil {
		return false, err
	}
	return m.Generation != v.RevMaintenance, nil
}

// --- 读取（供 service/controller 投影；最终绑定在阶段 5 页面接入） ---

// GetBillingStatementMonthByUserPeriod 读取客户月记录。
func GetBillingStatementMonthByUserPeriod(ctx context.Context, userId int, periodStart int64) (*BillingStatementMonth, error) {
	var m BillingStatementMonth
	err := DB.WithContext(ctx).Where("user_id = ? AND period_start = ?", userId, periodStart).First(&m).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	return &m, err
}

// GetBillingStatementVersion 读取单个版本。
func GetBillingStatementVersion(ctx context.Context, id int64) (*BillingStatementVersion, error) {
	var v BillingStatementVersion
	err := DB.WithContext(ctx).First(&v, id).Error
	if err != nil {
		return nil, err
	}
	return &v, nil
}

// GetBillingStatementVersionByDraftPublicId 按草稿公开 ID 读取。
func GetBillingStatementVersionByDraftPublicId(ctx context.Context, draftPublicId string) (*BillingStatementVersion, error) {
	var v BillingStatementVersion
	err := DB.WithContext(ctx).Where("draft_public_id = ?", draftPublicId).First(&v).Error
	if err != nil {
		return nil, err
	}
	return &v, nil
}

// ListBillingStatementVersionsByMonth 列出该客户月的全部版本（历史版本查看）。
func ListBillingStatementVersionsByMonth(ctx context.Context, monthId int64) ([]BillingStatementVersion, error) {
	var vs []BillingStatementVersion
	err := DB.WithContext(ctx).Where("month_id = ?", monthId).Order("version_number ASC").Find(&vs).Error
	return vs, err
}

// --- 来源写入口修订接线（方案 10.5，最小入侵） ---

var billingStatementLocation = func() *time.Location {
	location, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		return time.FixedZone("Asia/Shanghai", 8*60*60)
	}
	return location
}()

// naturalMonthStartAt 返回给定秒级时间所在的 Asia/Shanghai 自然月起点。
func naturalMonthStartAt(ts int64) int64 {
	loc := billingStatementLocation
	tm := time.Unix(ts, 0).In(loc)
	return time.Date(tm.Year(), tm.Month(), 1, 0, 0, 0, 0, loc).Unix()
}

// 跟踪由同库持久化钩子承担，不依赖功能开关。
func ShouldTrackBillingStatementRevision() bool {
	return BillingStatementVersionTopologyOK() && DB.Callback().Create().Get("billing_statement:created") != nil
}

// 精确固定本批 ID；清理范围保持原生行为，修订与保留标记由同一事务钩子写入。
func DeleteBillingStatementSettlementLogsTx(ctx context.Context, tx *gorm.DB, targetTimestamp int64, limit int) (int64, error) {
	var ids []int64
	if err := tx.WithContext(ctx).Model(&Log{}).Where("created_at < ?", targetTimestamp).Order("id asc").Limit(limit).Pluck("id", &ids).Error; err != nil {
		return 0, err
	}
	if len(ids) == 0 {
		return 0, nil
	}
	result := tx.WithContext(ctx).Where("id IN ?", ids).Delete(&Log{})
	return result.RowsAffected, result.Error
}

// upsertBillingStatementRetentionTx 在事务内写入客户月来源保留状态（幂等 upsert）。
func upsertBillingStatementRetentionTx(tx *gorm.DB, userId int, periodStart int64, status BillingStatementRetentionStatus, detail string, now int64) error {
	return tx.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "user_id"}, {Name: "period_start"}}, DoUpdates: clause.Assignments(map[string]interface{}{"status": status, "detail": detail, "updated_at": now})}).Create(&BillingStatementRetention{UserId: userId, PeriodStart: periodStart, Status: status, Detail: BillingStatementJSON(detail), UpdatedAt: now}).Error
}

// GetBillingStatementRetention 读取客户月来源保留状态（不存在返回 nil）。
func GetBillingStatementRetention(ctx context.Context, userId int, periodStart int64) (*BillingStatementRetention, error) {
	var r BillingStatementRetention
	err := DB.WithContext(ctx).Where("user_id = ? AND period_start = ?", userId, periodStart).First(&r).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	return &r, err
}

// --- 导出包装（供 service 生成编排使用） ---

// BillingStatementDependencyVector 是来源版本向量的导出类型。
type BillingStatementDependencyVector = billingStatementDependencyVector

// SnapshotBillingStatementDependencyVector 导出：生成时冻结来源版本向量。
func SnapshotBillingStatementDependencyVector(ctx context.Context, userId int, periodStart int64, policies ...BillingStatementReadPolicy) (*BillingStatementDependencyVector, error) {
	return snapshotBillingStatementDependencyVector(ctx, userId, periodStart, policies...)
}

// BillingStatementVectorChanged 导出：复验来源向量是否变化。
func BillingStatementVectorChanged(ctx context.Context, vec *BillingStatementDependencyVector) (bool, error) {
	probe := &BillingStatementVersion{
		RevScopeCustomerMonth: vec.ScopeCustomerMonth, RevCustomerMonth: vec.RevCustomerMonth,
		Dependencies:        vec.Dependencies,
		RevScopeMaintenance: vec.ScopeMaintenance, RevMaintenance: vec.RevMaintenance,
	}
	return billingStatementVectorChanged(ctx, probe)
}
