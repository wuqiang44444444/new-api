package model

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
)

const (
	legacyFunCloudVideoProtocol  = "funcloud_seedance_v2"
	legacyFunCloudAssetProtocol  = "funcloud_material_v2"
	legacyFunCloudVideoProfile   = "third_party_funcloud_seedance_v2"
	funCloudProtocolMigrationKey = "migration.funcloud_protocol_names_v1"
)

type funCloudProtocolMigrationTarget struct {
	table  string
	column string
}

var funCloudProtocolMigrationTargets = []funCloudProtocolMigrationTarget{
	{table: "channels", column: "settings"},
	{table: "tasks", column: "private_data"},
	{table: "task_create_attempts", column: "upstream_protocol"},
	{table: "task_create_attempts", column: "upstream_profile"},
	{table: "task_create_attempts", column: "adapter_version"},
	{table: "task_create_attempts", column: "frozen_connection_snapshot"},
	{table: "task_create_attempts", column: "billing_snapshot"},
	{table: "task_create_attempts", column: "recovery_snapshot"},
	{table: "task_create_idempotencies", column: "recovery_snapshot"},
	{table: "provider_cost_exposures", column: "upstream_profile"},
}

// migrateFunCloudProtocolNames rewrites the retired versioned identifiers in
// durable channel, task, attempt, and provider-cost facts. The runtime
// intentionally has no aliases or fallback decoders for the old names.
func migrateFunCloudProtocolNames() error {
	replacer := strings.NewReplacer(
		legacyFunCloudVideoProtocol, "funcloud_seedance",
		legacyFunCloudAssetProtocol, "funcloud_material",
		legacyFunCloudVideoProfile, "third_party_funcloud_seedance",
	)
	legacyValues := []string{
		legacyFunCloudVideoProtocol,
		legacyFunCloudAssetProtocol,
		legacyFunCloudVideoProfile,
	}

	updatedRows := int64(0)
	migrationApplied := false
	err := DB.Transaction(func(tx *gorm.DB) error {
		var marker Option
		err := tx.Where(&Option{Key: funCloudProtocolMigrationKey}).First(&marker).Error
		if err == nil && marker.Value == "done" {
			return nil
		}
		if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		migrationApplied = true
		if err := ensureFunCloudProtocolMigrationSchema(tx); err != nil {
			return err
		}
		if err := ensureNoActiveFunCloudProtocolFacts(tx, legacyValues); err != nil {
			return err
		}

		for _, target := range funCloudProtocolMigrationTargets {
			condition, args := funCloudLegacySearchCondition(tx.Dialector.Name(), []string{target.column}, legacyValues)

			rows, err := tx.Table(target.table).
				Select("id", target.column).
				Where(condition, args...).
				Rows()
			if err != nil {
				return err
			}
			type update struct {
				id    int64
				value string
			}
			updates := make([]update, 0)
			for rows.Next() {
				var id int64
				var value sql.NullString
				if err := rows.Scan(&id, &value); err != nil {
					_ = rows.Close()
					return err
				}
				if !value.Valid {
					continue
				}
				migrated := replacer.Replace(value.String)
				if migrated != value.String {
					updates = append(updates, update{id: id, value: migrated})
				}
			}
			if err := rows.Close(); err != nil {
				return err
			}
			for _, update := range updates {
				result := tx.Table(target.table).
					Where("id = ?", update.id).
					UpdateColumn(target.column, update.value)
				if result.Error != nil {
					return result.Error
				}
				if result.RowsAffected != 1 {
					return fmt.Errorf("FunCloud protocol migration expected one update for %s.%s id=%d, got %d", target.table, target.column, update.id, result.RowsAffected)
				}
				updatedRows++
			}
		}
		remaining, err := countFunCloudLegacyProtocolFacts(tx, legacyValues)
		if err != nil {
			return err
		}
		if remaining != 0 {
			return fmt.Errorf("FunCloud protocol migration verification failed: %d legacy values remain", remaining)
		}
		return tx.Save(&Option{Key: funCloudProtocolMigrationKey, Value: "done"}).Error
	})
	if err != nil {
		return err
	}
	if migrationApplied {
		common.SysLog(fmt.Sprintf("FunCloud protocol migration completed: updated_values=%d", updatedRows))
	}
	return nil
}

func ensureFunCloudProtocolMigrationSchema(tx *gorm.DB) error {
	required := append([]funCloudProtocolMigrationTarget{}, funCloudProtocolMigrationTargets...)
	required = append(required,
		funCloudProtocolMigrationTarget{table: "tasks", column: "status"},
		funCloudProtocolMigrationTarget{table: "task_create_attempts", column: "status"},
		funCloudProtocolMigrationTarget{table: "task_create_idempotencies", column: "status"},
	)
	for _, target := range required {
		if !tx.Migrator().HasTable(target.table) {
			return fmt.Errorf("FunCloud protocol migration requires table %s", target.table)
		}
		if !tx.Migrator().HasColumn(target.table, target.column) {
			return fmt.Errorf("FunCloud protocol migration requires column %s.%s", target.table, target.column)
		}
	}
	return nil
}

func ensureNoActiveFunCloudProtocolFacts(tx *gorm.DB, legacyValues []string) error {
	dialect := tx.Dialector.Name()
	taskCondition, taskArgs := funCloudLegacySearchCondition(dialect, []string{"private_data"}, legacyValues)
	var taskCount int64
	if err := tx.Table("tasks").Where(taskCondition, taskArgs...).
		Where("status IS NULL OR status NOT IN ?", TerminalTaskStatuses()).Count(&taskCount).Error; err != nil {
		return err
	}

	attemptColumns := []string{
		"upstream_protocol", "upstream_profile", "adapter_version", "frozen_connection_snapshot",
		"billing_snapshot", "recovery_snapshot",
	}
	attemptCondition, attemptArgs := funCloudLegacySearchCondition(dialect, attemptColumns, legacyValues)
	var attemptCount int64
	if err := tx.Table("task_create_attempts").Where(attemptCondition, attemptArgs...).
		Where("status IS NULL OR status NOT IN ?", []TaskCreateAttemptStatus{TaskCreateAttemptComplete, TaskCreateAttemptRejected}).
		Count(&attemptCount).Error; err != nil {
		return err
	}

	idempotencyCondition, idempotencyArgs := funCloudLegacySearchCondition(dialect, []string{"recovery_snapshot"}, legacyValues)
	var idempotencyCount int64
	if err := tx.Table("task_create_idempotencies").Where(idempotencyCondition, idempotencyArgs...).
		Where("status IS NULL OR status NOT IN ?", []string{TaskCreateIdempotencyComplete, TaskCreateIdempotencyCompletedNoReplay}).
		Count(&idempotencyCount).Error; err != nil {
		return err
	}

	if taskCount+attemptCount+idempotencyCount != 0 {
		return fmt.Errorf(
			"FunCloud protocol rename blocked by active durable facts: tasks=%d attempts=%d idempotencies=%d; roll back to the previous binary, stop new FunCloud submissions, drain these facts to terminal states, then retry the upgrade",
			taskCount,
			attemptCount,
			idempotencyCount,
		)
	}
	return nil
}

func countFunCloudLegacyProtocolFacts(tx *gorm.DB, legacyValues []string) (int64, error) {
	var total int64
	for _, target := range funCloudProtocolMigrationTargets {
		condition, args := funCloudLegacySearchCondition(tx.Dialector.Name(), []string{target.column}, legacyValues)
		var count int64
		if err := tx.Table(target.table).Where(condition, args...).Count(&count).Error; err != nil {
			return 0, err
		}
		total += count
	}
	return total, nil
}

func funCloudLegacySearchCondition(dialect string, columns, legacyValues []string) (string, []any) {
	conditions := make([]string, 0, len(columns)*len(legacyValues))
	args := make([]any, 0, len(columns)*len(legacyValues))
	for _, column := range columns {
		searchColumn := column
		switch dialect {
		case "mysql":
			searchColumn = "CAST(" + column + " AS CHAR)"
		case "postgres":
			searchColumn = "CAST(" + column + " AS TEXT)"
		}
		for _, legacyValue := range legacyValues {
			likePattern := "%" + strings.NewReplacer(
				"!", "!!",
				"%", "!%",
				"_", "!_",
			).Replace(legacyValue) + "%"
			conditions = append(conditions, searchColumn+" LIKE ? ESCAPE '!'")
			args = append(args, likePattern)
		}
	}
	return "(" + strings.Join(conditions, " OR ") + ")", args
}
