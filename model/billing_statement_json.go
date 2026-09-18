package model

import (
	"gorm.io/gorm"
	"gorm.io/gorm/schema"
)

// BillingStatementJSON holds bounded frozen projections and dependency vectors.
// MySQL TEXT is limited to 64 KiB; PostgreSQL/SQLite use their native text type.
type BillingStatementJSON string

func (BillingStatementJSON) GormDataType() string { return "string" }
func (BillingStatementJSON) GormDBDataType(db *gorm.DB, _ *schema.Field) string {
	if db.Dialector.Name() == "mysql" {
		return "longtext"
	}
	return "text"
}
