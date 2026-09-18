package model

import "gorm.io/gorm"

// Index metadata only: Log owns column definitions and defaults. Do not pass
// this type to AutoMigrate, which would change the existing log schema.
type billingStatementLogIndex struct {
	TokenID   int   `gorm:"index:idx_logs_user_type_token_cursor,priority:3"`
	ID        int64 `gorm:"index:idx_logs_user_type_token_cursor,priority:4"`
	UserID    int   `gorm:"index:idx_logs_user_type_created_at,priority:1;index:idx_logs_user_type_token_cursor,priority:1"`
	Type      int   `gorm:"index:idx_logs_user_type_created_at,priority:2;index:idx_logs_user_type_token_cursor,priority:2"`
	CreatedAt int64 `gorm:"index:idx_logs_user_type_created_at,priority:3"`
}

// migrateBillingStatementLogIndex bounds refund evidence reads by customer,
// type and period, including empty periods. Call only for relational log DBs.
func migrateBillingStatementLogIndex(db *gorm.DB) error {
	stmt := &gorm.Statement{DB: db}
	if err := stmt.Parse(&Log{}); err != nil {
		return err
	}
	migrator := db.Table(stmt.Schema.Table).Migrator()
	for _, name := range []string{"idx_logs_user_type_created_at", "idx_logs_user_type_token_cursor"} {
		if !migrator.HasIndex(&billingStatementLogIndex{}, name) {
			if err := migrator.CreateIndex(&billingStatementLogIndex{}, name); err != nil {
				return err
			}
		}
	}
	return nil
}
