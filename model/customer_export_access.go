package model

import (
	"context"

	"github.com/QuantumNous/new-api/common"
)

// AuthorizeCustomerExport reads current authority from the primary database.
// A previous admin session or ownership of an export never grants access to
// another customer's data after the actor has lost that authority.
func AuthorizeCustomerExport(ctx context.Context, actorID, targetID int) error {
	if actorID <= 0 || targetID <= 0 {
		return ErrCustomerExportNotFound
	}
	var actor User
	if err := DB.WithContext(ctx).Select("id", "role", "status").First(&actor, actorID).Error; err != nil {
		return err
	}
	if actor.Status != common.UserStatusEnabled ||
		(actorID != targetID && actor.Role < common.RoleAdminUser) {
		return ErrCustomerExportNotFound
	}
	// An administrator can export a deleted customer's historical statement.
	// Target identity is resolved by the admin create endpoint; log retention
	// and account deletion do not revoke the administrator's historical scope.
	return nil
}

// Resolve the explicit administrator username filter once, then freeze the
// durable customer ID on the job; later renames cannot redirect the export.
func ResolveCustomerExportUserId(ctx context.Context, username string) (int, error) {
	var user User
	err := DB.WithContext(ctx).Unscoped().Select("id").Where("username = ?", username).First(&user).Error
	return user.Id, err
}
