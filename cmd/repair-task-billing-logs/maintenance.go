package main

import (
	"context"
	"fmt"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"gorm.io/gorm"
)

// This command uses the existing migrated control tables; it must not migrate
// a production database or turn confirmation back on as a side effect of repair.
func repairMaintenance(db *gorm.DB, action, evidence string, generation int64, actorID int) (*model.BillingStatementMaintenance, error) {
	var actor model.User
	if err := db.Select("id, role, status").First(&actor, actorID).Error; err != nil {
		return nil, fmt.Errorf("an active root operator is required")
	}
	if actorID <= 0 || actor.Role != common.RoleRootUser || actor.Status != common.UserStatusEnabled {
		return nil, fmt.Errorf("an active root operator is required")
	}
	model.DB, model.LOG_DB = db, db
	common.SetDatabaseTypes(common.DatabaseTypeSQLite, common.DatabaseTypeSQLite)
	switch action {
	case "begin":
		return model.BeginBillingStatementMaintenance(context.Background(), evidence, actorID)
	case "end":
		return model.EndBillingStatementMaintenance(context.Background(), generation, evidence, actorID)
	default:
		return nil, fmt.Errorf("maintenance must be begin or end")
	}
}

// Do not print raw driver errors: they can include private query arguments.
func maintenanceFailureMessage(err error) string {
	switch err.Error() {
	case "an active root operator is required", "maintenance must be begin or end", "billing statement maintenance already in progress":
		return "maintenance refused: " + err.Error()
	case "billing statement maintenance not in progress":
		return "maintenance refused: maintenance is inactive or its generation does not match"
	case "billing statement version state conflict":
		return "maintenance refused: verify nonempty evidence, operator and the disabled confirmation switch"
	case "record not found":
		return "maintenance refused: required control or operator record is missing"
	default:
		return "maintenance refused: database operation failed; verify migrated tables and database availability"
	}
}
