package model

import (
	"fmt"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
)

// migrateUserUsedQuotaColumn only widens the cumulative projection. The caller
// has already checked the wallet columns; their migration remains independent.
func migrateUserUsedQuotaColumn(db *gorm.DB, kind common.DatabaseType) error {
	columns, err := db.Migrator().ColumnTypes(&User{})
	if err != nil {
		return err
	}
	for _, column := range columns {
		if column.Name() != "used_quota" {
			continue
		}
		dataType := strings.ToLower(column.DatabaseTypeName())
		if is64BitIntegerType(kind, dataType) || kind == common.DatabaseTypeSQLite {
			return nil
		}
		if dataType != "int" && dataType != "integer" && dataType != "int4" {
			return fmt.Errorf("users.used_quota has unsupported migration source type %s", dataType)
		}
		if err := db.Migrator().AlterColumn(&User{}, "UsedQuota"); err != nil {
			return fmt.Errorf("cannot widen users.used_quota: %w", err)
		}
		updated, err := db.Migrator().ColumnTypes(&User{})
		if err != nil {
			return err
		}
		for _, actual := range updated {
			if actual.Name() == "used_quota" && is64BitIntegerType(kind, actual.DatabaseTypeName()) {
				return nil
			}
		}
		return fmt.Errorf("users.used_quota is not a 64-bit integer after migration")
	}
	return fmt.Errorf("users.used_quota is missing")
}
