package model

import (
	"reflect"

	"gorm.io/gorm"
	"gorm.io/gorm/schema"
)

// GORM v1.25's matching-default comparison overwrites a previously detected
// VARCHAR size change. Omit only an already equivalent default comparison for
// Channel.Group; AlterColumn still reads the original model and keeps its
// default. This is schema reconciliation, not the all-table import workflow.
func channelGroupSchemaComparison(field *schema.Field, column gorm.ColumnType) (*schema.Field, gorm.ColumnType) {
	if field.Schema.ModelType != reflect.TypeFor[Channel]() || field.DBName != "group" || !field.HasDefaultValue {
		return field, column
	}
	storedDefault, ok := column.DefaultValue()
	if !ok || storedDefault != field.DefaultValue {
		return field, column
	}
	comparison := *field
	comparison.HasDefaultValue = false
	comparison.DefaultValue = ""
	comparison.DefaultValueInterface = nil
	return &comparison, columnWithoutDefault{column}
}
