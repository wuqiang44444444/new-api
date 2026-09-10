package controller

import (
	"errors"
	"net/http"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
)

// GetCustomerContractMigrationPreview lists legacy user-level contracts with
// their channel candidates so administrators can decide ambiguous mappings
// before switching. Read-only.
func GetCustomerContractMigrationPreview(c *gin.Context) {
	previews, err := model.PreviewLegacyCustomerContractMigration()
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, previews)
}

type customerContractMigrationRequest struct {
	UserId           int            `json:"user_id"`
	ContractName     string         `json:"contract_name"`
	Reason           string         `json:"reason"`
	ChannelOverrides map[string]int `json:"channel_overrides"`
}

// PostCustomerContractMigration converts one user's legacy contract into a
// contract entity and binds the user's existing keys. Ambiguous channel
// mappings must be decided explicitly; the migration never guesses.
func PostCustomerContractMigration(c *gin.Context) {
	var request customerContractMigrationRequest
	if err := common.DecodeJson(c.Request.Body, &request); err != nil || request.UserId <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "user_id and a valid request body are required"})
		return
	}
	target, err := model.GetUserById(request.UserId, false)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if !canManageTargetRole(c.GetInt("role"), target.Role) {
		c.JSON(http.StatusForbidden, gin.H{"success": false, "message": "no permission to migrate this user's contract"})
		return
	}
	snapshot, err := model.MigrateLegacyCustomerContractForUser(model.MigrateCustomerContractParams{
		UserId: request.UserId, AdminUserId: c.GetInt("id"), ContractName: request.ContractName,
		Reason: request.Reason, ChannelOverrides: request.ChannelOverrides,
	})
	if err != nil {
		status := http.StatusBadRequest
		if errors.Is(err, model.ErrCustomerContractMigrationAlreadyDone) ||
			errors.Is(err, model.ErrCustomerContractMigrationNothingToDo) {
			status = http.StatusConflict
		}
		c.JSON(status, gin.H{"success": false, "message": err.Error()})
		return
	}
	views, err := service.BuildContractEntityAdminViews([]model.ContractEntitySnapshot{*snapshot}, target.Group)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	view := gin.H{"user_id": target.Id, "username": target.Username, "contracts": views}
	recordManageAuditFor(c, target.Id, "user.contract.migrate", map[string]interface{}{
		"contract_id": snapshot.Id, "version": snapshot.Version, "rule_count": len(snapshot.Rules),
	})
	common.ApiSuccess(c, view)
}
