package model

import "gorm.io/gorm"

// Index metadata only. Task owns the columns and their types; never pass this
// descriptor to AutoMigrate or make provider task identities globally unique.
type billingStatementTaskIndex struct {
	TaskID string `gorm:"index:idx_tasks_task_user_app,priority:1"`
	UserID int    `gorm:"index:idx_tasks_task_user_app,priority:2"`
	AppID  int    `gorm:"index:idx_tasks_task_user_app,priority:3"`
}

// migrateBillingStatementTaskIndex lets refund evidence reads locate a batch
// of task IDs within its owner/application scope without scanning that scope.
// Leading with task_id also covers the existing task-ID lookups, allowing us
// to replace the old single-column index instead of taxing every task write
// with an additional index. This changes no query or uniqueness constraint.
func migrateBillingStatementTaskIndex(db *gorm.DB) error {
	stmt := &gorm.Statement{DB: db}
	if err := stmt.Parse(&Task{}); err != nil {
		return err
	}
	migrator := db.Table(stmt.Schema.Table).Migrator()
	const name = "idx_tasks_task_user_app"
	if !migrator.HasIndex(&billingStatementTaskIndex{}, name) {
		if err := migrator.CreateIndex(&billingStatementTaskIndex{}, name); err != nil {
			return err
		}
	}
	columns := []string{"task_id", "user_id", "app_id"}
	if err := retireRedundantIndex(db, &Task{}, "idx_tasks_task_id", []string{"task_id"}, name, columns, false); err != nil {
		return err
	}
	// Retire the earlier additive design too if an installation already has it.
	return retireRedundantIndex(db, &Task{}, "idx_tasks_user_app_task", []string{"user_id", "app_id", "task_id"}, name, columns, false)
}
