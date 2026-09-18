package model

import "gorm.io/gorm"

// Only describes an index over existing columns. Never AutoMigrate this type:
// Task remains the authority for column types, defaults and lifecycle facts.
type taskUsageCheckIndex struct {
	BillingState     string `gorm:"index:idx_task_usage_poll,priority:1"`
	Status           string `gorm:"index:idx_task_usage_poll,priority:2"`
	UsageReviewAt    int64  `gorm:"index:idx_task_usage_poll,priority:3"`
	UsageCheckNextAt int64  `gorm:"index:idx_task_usage_poll,priority:4"`
	ID               int64  `gorm:"index:idx_task_usage_poll,priority:5"`
}

// migrateTaskUsageCheckIndex keeps the due-usage probe inside a covering index
// even when SQLite has no statistics and most tasks have status SUCCESS.
func migrateTaskUsageCheckIndex(db *gorm.DB) error {
	stmt := &gorm.Statement{DB: db}
	if err := stmt.Parse(&Task{}); err != nil {
		return err
	}
	migrator := db.Table(stmt.Schema.Table).Migrator()
	if migrator.HasIndex(&taskUsageCheckIndex{}, "idx_task_usage_poll") {
		return nil
	}
	return migrator.CreateIndex(&taskUsageCheckIndex{}, "idx_task_usage_poll")
}
