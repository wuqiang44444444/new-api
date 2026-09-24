package model

import (
	"gorm.io/gorm"
	"gorm.io/gorm/schema"
)

// BillingText stores complete calculation evidence without MySQL TEXT's 64 KiB
// limit. PostgreSQL and SQLite use their ordinary unbounded text representation.
type BillingText string

func (BillingText) GormDataType() string { return "string" }
func (BillingText) GormDBDataType(db *gorm.DB, _ *schema.Field) string {
	if db.Dialector.Name() == "mysql" {
		return "longtext"
	}
	return "text"
}
