package controller

import (
	"errors"
	"net/http"
	"sort"
	"strconv"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/i18n"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
)

type customerContractGroupOption struct {
	Group             string                                          `json:"group"`
	Models            []string                                        `json:"models"`
	Prices            map[string]service.CustomerContractPricePreview `json:"prices"`
	NativeGroupRatio  string                                          `json:"native_group_ratio"`
	SpecialGroupRatio bool                                            `json:"special_group_ratio"`
}

type customerContractChannelGroupOption struct {
	Group             string                                           `json:"group"`
	Models            []model.CustomerContractEntityGroupModelChannels `json:"models"`
	NativeGroupRatio  string                                           `json:"native_group_ratio"`
	SpecialGroupRatio bool                                             `json:"special_group_ratio"`
}

type customerContractWriteRequest struct {
	ExpectedVersion *int64                      `json:"expected_version"`
	Enabled         *bool                       `json:"enabled"`
	Name            string                      `json:"name"`
	Reason          string                      `json:"reason"`
	Rules           []customerContractRuleInput `json:"rules"`
}

type customerContractRuleInput struct {
	Model      string `json:"model"`
	ChannelId  int    `json:"channel_id"`
	RouteGroup string `json:"route_group"`
	Discount   string `json:"discount"`
}

// parseCustomerContractEntityRules converts the admin drawer's rule inputs
// into normalized model inputs; discount notation is validated here.
func parseCustomerContractEntityRules(inputs []customerContractRuleInput) ([]model.CustomerContractEntityRuleInput, error) {
	rules := make([]model.CustomerContractEntityRuleInput, 0, len(inputs))
	for _, input := range inputs {
		ratioUnits, err := service.ParseCustomerContractRatio(input.Discount)
		if err != nil {
			return nil, err
		}
		rules = append(rules, model.CustomerContractEntityRuleInput{
			PublicModel: input.Model, ChannelId: input.ChannelId, RouteGroup: input.RouteGroup, RatioUnits: ratioUnits,
		})
	}
	return rules, nil
}

func GetCustomerContract(c *gin.Context) {
	target, ok := authorizedCustomerContractTarget(c)
	if !ok {
		return
	}
	snapshots, err := model.ListContractEntitiesForUser(target.Id, true)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	views, err := service.BuildContractEntityAdminViews(snapshots, target.Group)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, gin.H{
		"user_id": target.Id, "username": target.Username, "contracts": views,
	})
}

func PostCustomerContractEntity(c *gin.Context) {
	target, ok := authorizedCustomerContractTarget(c)
	if !ok {
		return
	}
	var request customerContractWriteRequest
	if err := common.DecodeJson(c.Request.Body, &request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "a valid request body is required"})
		return
	}
	rules, err := parseCustomerContractEntityRules(request.Rules)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": err.Error()})
		return
	}
	enabled := false
	if request.Enabled != nil {
		enabled = *request.Enabled
	}
	snapshot, err := model.CreateCustomerContractEntity(model.CreateCustomerContractParams{
		UserId: target.Id, AdminUserId: c.GetInt("id"), Name: request.Name,
		Enabled: enabled, Reason: request.Reason, Rules: rules,
	})
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": err.Error()})
		return
	}
	view, err := service.BuildContractEntityAdminViews([]model.ContractEntitySnapshot{*snapshot}, target.Group)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	recordManageAuditFor(c, target.Id, "user.contract.create", map[string]interface{}{
		"contract_id": snapshot.Id, "version": snapshot.Version, "enabled": snapshot.Enabled, "rule_count": len(snapshot.Rules),
	})
	common.ApiSuccess(c, gin.H{
		"user_id": target.Id, "username": target.Username, "contracts": view,
	})
}

// authorizeCustomerContractEntity verifies the acting admin may manage the
// contract owner, then returns the contract snapshot without availability.
func authorizeCustomerContractEntity(c *gin.Context, contractId int) (*model.ContractEntitySnapshot, bool) {
	if contractId <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "invalid contract id"})
		return nil, false
	}
	snapshot, err := model.GetContractEntitySnapshot(contractId, false)
	if err != nil {
		if errors.Is(err, model.ErrCustomerContractEntityNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"success": false, "message": "contract not found"})
		} else {
			common.ApiError(c, err)
		}
		return nil, false
	}
	owner, err := model.GetUserById(snapshot.UserId, false)
	if err != nil {
		common.ApiError(c, err)
		return nil, false
	}
	if !canManageTargetRole(c.GetInt("role"), owner.Role) {
		common.ApiErrorI18n(c, i18n.MsgUserNoPermissionSameLevel)
		return nil, false
	}
	return snapshot, true
}

func PutCustomerContractEntity(c *gin.Context) {
	contractId, err := strconv.Atoi(c.Param("contract_id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "invalid contract id"})
		return
	}
	snapshotBefore, ok := authorizeCustomerContractEntity(c, contractId)
	if !ok {
		return
	}
	var request customerContractWriteRequest
	if err := common.DecodeJson(c.Request.Body, &request); err != nil || request.ExpectedVersion == nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "expected_version and a valid request body are required"})
		return
	}
	rules, err := parseCustomerContractEntityRules(request.Rules)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": err.Error()})
		return
	}
	snapshot, err := model.ReplaceCustomerContractEntity(model.ReplaceCustomerContractEntityParams{
		ContractId: contractId, AdminUserId: c.GetInt("id"), ExpectedVersion: *request.ExpectedVersion,
		Name: request.Name, Enabled: request.Enabled, Reason: request.Reason, Rules: rules,
	})
	if err != nil {
		status := http.StatusBadRequest
		if errors.Is(err, model.ErrCustomerContractVersionConflict) {
			status = http.StatusConflict
		}
		c.JSON(status, gin.H{"success": false, "message": err.Error()})
		return
	}
	service.InvalidateContractEntityCache(contractId)
	target, err := model.GetUserById(snapshotBefore.UserId, false)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	view, err := service.BuildContractEntityAdminViews([]model.ContractEntitySnapshot{*snapshot}, target.Group)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	recordManageAuditFor(c, snapshot.UserId, "user.contract.update", map[string]interface{}{
		"contract_id": snapshot.Id, "version": snapshot.Version, "enabled": snapshot.Enabled, "rule_count": len(snapshot.Rules),
	})
	common.ApiSuccess(c, gin.H{
		"user_id": snapshot.UserId, "contracts": view,
	})
}

func GetCustomerContractAudits(c *gin.Context) {
	target, ok := authorizedCustomerContractTarget(c)
	if !ok {
		return
	}
	page := common.GetPageQuery(c)
	audits, total, err := model.GetCustomerContractAudits(target.Id, page.GetStartIdx(), page.GetPageSize())
	if err != nil {
		common.ApiError(c, err)
		return
	}
	page.SetTotal(int(total))
	page.SetItems(audits)
	common.ApiSuccess(c, page)
}

func GetCustomerContractEntityAudits(c *gin.Context) {
	contractId, err := strconv.Atoi(c.Param("contract_id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "invalid contract id"})
		return
	}
	snapshot, ok := authorizeCustomerContractEntity(c, contractId)
	if !ok {
		return
	}
	page := common.GetPageQuery(c)
	audits, total, err := model.GetContractEntityAudits(snapshot.Id, page.GetStartIdx(), page.GetPageSize())
	if err != nil {
		common.ApiError(c, err)
		return
	}
	page.SetTotal(int(total))
	page.SetItems(audits)
	common.ApiSuccess(c, page)
}

func GetCustomerContractOptions(c *gin.Context) {
	target, ok := authorizedCustomerContractTarget(c)
	if !ok {
		return
	}
	groupRatios := ratio_setting.GetGroupRatioCopy()
	groupNames := make([]string, 0, len(groupRatios))
	for group := range groupRatios {
		if group != "auto" {
			groupNames = append(groupNames, group)
		}
	}
	sort.Strings(groupNames)
	options := make([]customerContractGroupOption, 0, len(groupNames))
	for _, group := range groupNames {
		models, err := model.GetCustomerContractAvailableModelsForGroup(group)
		if err != nil {
			common.ApiError(c, err)
			return
		}
		sort.Strings(models)
		nativeRatio, special := service.ResolveCustomerContractNativeGroupRatio(target.Group, group)
		prices := make(map[string]service.CustomerContractPricePreview, len(models))
		for _, modelName := range models {
			prices[modelName] = service.BuildCustomerContractPricePreview(modelName, decimal.NewFromFloat(nativeRatio))
		}
		options = append(options, customerContractGroupOption{
			Group: group, Models: models, Prices: prices,
			NativeGroupRatio: decimal.NewFromFloat(nativeRatio).String(), SpecialGroupRatio: special,
		})
	}
	common.ApiSuccess(c, options)
}

func GetCustomerContractChannelOptions(c *gin.Context) {
	target, ok := authorizedCustomerContractTarget(c)
	if !ok {
		return
	}
	groupRatios := ratio_setting.GetGroupRatioCopy()
	groupNames := make([]string, 0, len(groupRatios))
	for group := range groupRatios {
		if group != "auto" {
			groupNames = append(groupNames, group)
		}
	}
	sort.Strings(groupNames)
	options := make([]customerContractChannelGroupOption, 0, len(groupNames))
	for _, group := range groupNames {
		models, err := model.GetCustomerContractEntityChannelOptions(group)
		if err != nil {
			common.ApiError(c, err)
			return
		}
		nativeRatio, special := service.ResolveCustomerContractNativeGroupRatio(target.Group, group)
		options = append(options, customerContractChannelGroupOption{
			Group: group, Models: models,
			NativeGroupRatio: decimal.NewFromFloat(nativeRatio).String(), SpecialGroupRatio: special,
		})
	}
	common.ApiSuccess(c, options)
}

func GetSelfCustomerContract(c *gin.Context) {
	user, err := model.GetUserById(c.GetInt("id"), false)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	snapshots, err := model.ListContractEntitiesForUser(user.Id, false)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "customer contract is temporarily unavailable"})
		return
	}
	for i := range snapshots {
		if err := model.RefreshContractEntityAvailability(&snapshots[i]); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "customer contract is temporarily unavailable"})
			return
		}
	}
	views, err := service.BuildContractEntityUserViews(snapshots, user.Group)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, gin.H{"contracts": views})
}

func authorizedCustomerContractTarget(c *gin.Context) (*model.User, bool) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		common.ApiError(c, err)
		return nil, false
	}
	user, err := model.GetUserById(id, false)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"success": false, "message": "user not found"})
		} else {
			common.ApiError(c, err)
		}
		return nil, false
	}
	if !canManageTargetRole(c.GetInt("role"), user.Role) {
		common.ApiErrorI18n(c, i18n.MsgUserNoPermissionSameLevel)
		return nil, false
	}
	return user, true
}
