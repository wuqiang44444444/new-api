package model

import (
	"fmt"
	"reflect"
	"sort"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"gorm.io/gorm/schema"
)

// InitBillingStatementSourceTracking installs runtime callbacks on every writer,
// independently of schema migration and the feature switch. Call after both DBs initialize.
func InitBillingStatementSourceTracking() error {
	if DB == nil || !BillingStatementVersionTopologyOK() {
		return nil
	}
	if DB.Callback().Create().Get("billing_statement:created") != nil {
		return nil
	}
	return registerBillingStatementSourceTracking(DB)
}

// 同库 GORM 持久化边界覆盖 createLog、Task/Batch 直接写入与任务补证变化。
// 钩子在写事务提交前执行；功能关闭仍维护修订，避免停用期间出现不可追踪的空窗。
// 原始 SQL 维护仍须遵守维护协议，不在此解析 SQL 或重做计费。
func registerBillingStatementSourceTracking(db *gorm.DB) error {
	if err := db.Callback().Create().After("gorm:create").Before("gorm:commit_or_rollback_transaction").Register("billing_statement:created", trackBillingStatementCreated); err != nil {
		return err
	}
	if err := db.Callback().Update().Before("gorm:update").Register("billing_statement:before_update", captureBillingStatementUpdates); err != nil {
		return err
	}
	if err := db.Callback().Update().After("gorm:update").Before("gorm:commit_or_rollback_transaction").Register("billing_statement:updated", trackBillingStatementUpdated); err != nil {
		return err
	}
	if err := db.Callback().Delete().Before("gorm:delete").Register("billing_statement:before_delete", captureBillingStatementSources); err != nil {
		return err
	}
	return db.Callback().Delete().After("gorm:delete").Before("gorm:commit_or_rollback_transaction").Register("billing_statement:deleted", trackBillingStatementDeleted)
}

type billingStatementSourceRow struct {
	ID          int64
	UserId      int
	CreatedAt   int64
	Type        int
	TokenId     int
	TokenName   string
	AppID       int
	TaskID      string
	Content     string
	ChannelId   int
	Properties  string
	PrivateData string
	Quota       int
	Other       string
}

func billingStatementTrackedTable(tx *gorm.DB) bool {
	return tx.Error == nil && BillingStatementVersionTopologyOK() &&
		(tx.Statement.Table == "logs" || tx.Statement.Table == "tasks")
}

func trackBillingStatementCreated(tx *gorm.DB) {
	if !billingStatementTrackedTable(tx) || tx.RowsAffected == 0 {
		return
	}
	var rows []billingStatementSourceRow
	read := func(value reflect.Value) {
		row := billingStatementSourceRow{}
		target := reflect.ValueOf(&row).Elem()
		for _, column := range []struct{ db, field string }{
			{"id", "ID"}, {"user_id", "UserId"}, {"created_at", "CreatedAt"}, {"type", "Type"},
			{"other", "Other"}, {"token_id", "TokenId"}, {"token_name", "TokenName"}, {"content", "Content"}, {"app_id", "AppID"}, {"task_id", "TaskID"},
		} {
			var raw interface{}
			if value.Kind() == reflect.Map {
				cell := value.MapIndex(reflect.ValueOf(column.db))
				if !cell.IsValid() && column.db == "id" {
					cell = value.MapIndex(reflect.ValueOf("@id"))
				}
				if cell.IsValid() {
					raw = cell.Interface()
				}
			} else if tx.Statement.Schema != nil {
				if field := tx.Statement.Schema.FieldsByDBName[column.db]; field != nil {
					raw, _ = field.ValueOf(tx.Statement.Context, value)
				}
			}
			if raw != nil {
				source := reflect.ValueOf(raw)
				field := target.FieldByName(column.field)
				if source.Type().ConvertibleTo(field.Type()) {
					field.Set(source.Convert(field.Type()))
				}
			}
		}
		rows = append(rows, row)
	}
	value := reflect.Indirect(reflect.ValueOf(tx.Statement.Dest))
	if value.Kind() == reflect.Slice || value.Kind() == reflect.Array {
		for i := 0; i < value.Len(); i++ {
			read(reflect.Indirect(value.Index(i)))
		}
	} else {
		read(value)
	}
	tx.AddError(bumpBillingStatementSources(tx, rows, false))
}

// 明确只改轮询进度等非依赖字段时，不读取证据、不递增修订。
// 结构体更新比较实际依赖字段；普通状态/进度轮询不递增修订。
func captureBillingStatementUpdates(tx *gorm.DB) {
	if !billingStatementTrackedTable(tx) {
		return
	}
	if tx.Statement.Table == "tasks" {
		if fields, ok := tx.Statement.Dest.(map[string]interface{}); ok {
			relevant := false
			for key := range fields {
				name := strings.ToLower(key)
				if tx.Statement.Schema != nil {
					if field := tx.Statement.Schema.LookUpField(key); field != nil {
						name = field.DBName
					}
				}
				switch name {
				case "created_at", "quota", "user_id", "app_id", "task_id", "channel_id", "properties", "private_data":
					relevant = true
				}
			}
			if !relevant {
				return
			}
		}
	}
	captureBillingStatementSources(tx)
}

func captureBillingStatementSources(tx *gorm.DB) {
	if !billingStatementTrackedTable(tx) {
		return
	}
	query := tx.Session(&gorm.Session{NewDB: true}).Table(tx.Statement.Table)
	if where, ok := tx.Statement.Clauses["WHERE"]; ok {
		query = query.Clauses(where.Expression)
	}
	if tx.Statement.Schema != nil {
		_, values := schema.GetIdentityFieldValuesMap(tx.Statement.Context, tx.Statement.ReflectValue, tx.Statement.Schema.PrimaryFields)
		if len(values) > 0 {
			column, vals := schema.ToQueryValues(tx.Statement.Table, tx.Statement.Schema.PrimaryFieldDBNames, values)
			query = query.Where(clause.IN{Column: column, Values: vals})
		}
	}
	columns := "id, user_id, created_at, app_id, task_id, channel_id, properties, private_data, quota"
	if tx.Statement.Table == "logs" {
		columns = "id, user_id, created_at, type, token_id, token_name, content, other"
	}
	var rows []billingStatementSourceRow
	if err := query.Select(columns).Scan(&rows).Error; err != nil {
		tx.AddError(err)
		return
	}
	tx.InstanceSet("billing_statement:sources", rows)
}

func trackBillingStatementUpdated(tx *gorm.DB) {
	trackBillingStatementChanged(tx, false)
}
func trackBillingStatementDeleted(tx *gorm.DB) {
	trackBillingStatementChanged(tx, true)
}
func trackBillingStatementChanged(tx *gorm.DB, deleted bool) {
	if !billingStatementTrackedTable(tx) || tx.RowsAffected == 0 {
		return
	}
	captured, ok := tx.InstanceGet("billing_statement:sources")
	if !ok {
		return
	}
	rows := captured.([]billingStatementSourceRow)
	if !deleted && len(rows) > 0 {
		ids := make([]int64, 0, len(rows))
		for _, row := range rows {
			ids = append(ids, row.ID)
		}
		var current []billingStatementSourceRow
		columns := "id, user_id, created_at, app_id, task_id, channel_id, properties, private_data, quota"
		if tx.Statement.Table == "logs" {
			columns = "id, user_id, created_at, type, token_id, token_name, content, other"
		}
		if err := tx.Session(&gorm.Session{NewDB: true}).Table(tx.Statement.Table).Select(columns).Where("id IN ?", ids).Scan(&current).Error; err != nil {
			tx.AddError(err)
			return
		}
		if tx.Statement.Table == "tasks" {
			before := make(map[int64]billingStatementSourceRow, len(rows))
			for _, row := range rows {
				before[row.ID] = row
			}
			rows = nil
			for _, row := range current {
				old, found := before[row.ID]
				if !found || old != row {
					rows = append(rows, old, row)
				}
			}
		} else {
			rows = append(rows, current...)
		}
	}
	tx.AddError(bumpBillingStatementSources(tx, rows, deleted))
}

func bumpBillingStatementSources(tx *gorm.DB, rows []billingStatementSourceRow, deleted bool) error {
	scopes := map[string]struct{}{}
	type customerMonth struct {
		user  int
		month int64
	}
	affected := map[customerMonth]struct{}{}
	for _, row := range rows {
		if tx.Statement.Table == "tasks" {
			scopes[fmt.Sprintf("ev:%d:0", row.UserId)] = struct{}{}
			scopes[billingStatementTaskScope(row.UserId, row.AppID, row.TaskID)] = struct{}{}
			scopes[fmt.Sprintf("review-task-month:%d:%d", row.UserId, naturalMonthStartAt(row.CreatedAt))] = struct{}{}
			continue
		}
		if (row.Type != LogTypeConsume && row.Type != LogTypeRefund) || isNativeChannelTestLog(row.Type, row.TokenId, row.TokenName, row.Content) {
			continue
		}
		scopes[fmt.Sprintf("ev:%d:0", row.UserId)] = struct{}{}
		addBillingSourceReviewLogScope(scopes, row)
		month := naturalMonthStartAt(row.CreatedAt)
		scopes[fmt.Sprintf("cm:%d:%d", row.UserId, month)] = struct{}{}
		scopes[fmt.Sprintf("log:%d", row.ID)] = struct{}{}
		if row.Type == LogTypeRefund {
			scopes[fmt.Sprintf("refund:%d:%d", row.UserId, row.TokenId)] = struct{}{}
		}
		if deleted {
			affected[customerMonth{row.UserId, month}] = struct{}{}
		}
	}
	ordered := make([]string, 0, len(scopes))
	for scope := range scopes {
		ordered = append(ordered, scope)
	}
	sort.Strings(ordered)
	clean := tx.Session(&gorm.Session{NewDB: true})
	if len(ordered) == 0 {
		return nil
	}
	if err := clean.SavePoint("billing_statement_revision").Error; err != nil {
		return err
	}
	// Registration locks precede evidence scopes; only registered precise scopes
	// are updated, avoiding one permanent revision row per ordinary consume log.
	sort.Slice(ordered, func(i, j int) bool {
		a, b := strings.HasPrefix(ordered[i], "ev:"), strings.HasPrefix(ordered[j], "ev:")
		if a != b {
			return a
		}
		return ordered[i] < ordered[j]
	})
	for _, scope := range ordered {
		var err error
		if strings.HasPrefix(scope, "cm:") || strings.HasPrefix(scope, "ev:") {
			err = IncrementBillingStatementRevisionTx(clean, scope)
		} else {
			err = clean.Model(&BillingStatementRevision{}).Where("scope = ?", scope).Updates(map[string]interface{}{"revision": gorm.Expr("revision + 1"), "updated_at": nowSeconds()}).Error
		}
		if err != nil {
			if rollbackErr := clean.RollbackTo("billing_statement_revision").Error; rollbackErr != nil {
				return rollbackErr
			}
			// A partial marker alone can be overwritten by a scan that finished
			// before this failure. Reuse the existing publication fence: advancing
			// its generation serializes with verification and invalidates old scans
			// even if the customer was already partial or the failure repeats.
			if _, fenceErr := lockBillingStatementMaintenanceTx(tx.Statement.Context, clean.Model(&BillingStatementMaintenance{})); fenceErr != nil {
				return fenceErr
			}
			if fenceErr := clean.Model(&BillingStatementMaintenance{}).Where("id = ?", billingStatementMaintenanceRowID).
				UpdateColumn("generation", gorm.Expr("generation + 1")).Error; fenceErr != nil {
				return fenceErr
			}
			for _, row := range rows {
				// Cross-period evidence can affect older months. The failure fence is intentionally conservative.
				if fenceErr := clean.Model(&BillingStatementRetention{}).Where("user_id = ?", row.UserId).
					Updates(map[string]interface{}{"status": BillingStatementRetentionPartial, "detail": "source revision unavailable; verification required", "updated_at": nowSeconds()}).Error; fenceErr != nil {
					return fenceErr
				}
				// A first export can register reference revisions before any month
				// has a retention row. Both Task and Log writes need a persistent
				// fence, including when the affected export belongs to another month.
				if fenceErr := upsertBillingStatementRetentionTx(clean, row.UserId, naturalMonthStartAt(row.CreatedAt), BillingStatementRetentionPartial, "source revision unavailable; verification required", nowSeconds()); fenceErr != nil {
					return fenceErr
				}
			}
			common.SysError("billing statement source revision unavailable; source retained and confirmation fenced")
			return nil
		}
	}
	for key := range affected {
		if err := upsertBillingStatementRetentionTx(clean, key.user, key.month, BillingStatementRetentionPartial, "source logs cleaned", common.GetTimestamp()); err != nil {
			return err
		}
	}
	return nil
}
