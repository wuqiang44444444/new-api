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

type customerContractWriteRequest struct {
	ExpectedVersion *int64                      `json:"expected_version"`
	Enabled         *bool                       `json:"enabled"`
	Reason          string                      `json:"reason"`
	Rules           []customerContractRuleInput `json:"rules"`
}

type customerContractRuleInput struct {
	Model      string `json:"model"`
	RouteGroup string `json:"route_group"`
	Discount   string `json:"discount"`
}

func GetCustomerContract(c *gin.Context) {
	target, ok := authorizedCustomerContractTarget(c)
	if !ok {
		return
	}
	snapshot, err := model.GetCustomerContractSnapshot(target.Id)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	rules, err := service.BuildCustomerContractAdminRules(snapshot, target.Group)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, gin.H{
		"user_id": target.Id, "username": target.Username, "contract_mode": snapshot.Enabled,
		"contract_version": snapshot.Version, "rules": rules,
		"disable_warning": "All existing API keys will immediately return to native NEWAPI model permissions; contract rules will be retained but no longer apply.",
	})
}

func PutCustomerContract(c *gin.Context) {
	target, ok := authorizedCustomerContractTarget(c)
	if !ok {
		return
	}
	var request customerContractWriteRequest
	if err := common.DecodeJson(c.Request.Body, &request); err != nil || request.ExpectedVersion == nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "expected_version and a valid request body are required"})
		return
	}
	current, err := model.GetCustomerContractSnapshot(target.Id)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	enabled := current.Enabled
	if request.Enabled != nil {
		enabled = *request.Enabled
	}
	if current.Version == 0 && len(request.Rules) > 0 {
		enabled = true
	}
	rules := make([]model.CustomerContractRule, 0, len(request.Rules))
	for _, input := range request.Rules {
		ratioUnits, err := service.ParseCustomerContractRatio(input.Discount)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": err.Error()})
			return
		}
		rules = append(rules, model.CustomerContractRule{
			PublicModel: input.Model, RouteGroup: input.RouteGroup, RatioUnits: ratioUnits,
		})
	}
	snapshot, err := model.ReplaceCustomerContract(model.ReplaceCustomerContractParams{
		UserId: target.Id, AdminUserId: c.GetInt("id"), ExpectedVersion: *request.ExpectedVersion,
		Enabled: enabled, Reason: request.Reason, Rules: rules,
	})
	if err != nil {
		status := http.StatusBadRequest
		if errors.Is(err, model.ErrCustomerContractVersionConflict) {
			status = http.StatusConflict
		}
		c.JSON(status, gin.H{"success": false, "message": err.Error()})
		return
	}
	service.InvalidateCustomerContractCache(target.Id)
	view, err := service.BuildCustomerContractAdminRules(snapshot, target.Group)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	recordManageAuditFor(c, target.Id, "user.contract.update", map[string]interface{}{
		"version": snapshot.Version, "enabled": snapshot.Enabled, "rule_count": len(snapshot.Rules),
	})
	common.ApiSuccess(c, gin.H{
		"user_id": target.Id, "username": target.Username, "contract_mode": snapshot.Enabled,
		"contract_version": snapshot.Version, "rules": view,
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

func GetSelfCustomerContract(c *gin.Context) {
	user, err := model.GetUserById(c.GetInt("id"), false)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if !user.ContractMode {
		common.ApiSuccess(c, gin.H{
			"contract_mode": false, "contract_version": user.ContractVersion,
			"models": []service.CustomerContractUserRuleView{},
		})
		return
	}
	snapshot, err := service.LoadCustomerContractSnapshot(user.Id, user.ContractVersion)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "customer contract is temporarily unavailable"})
		return
	}
	rules, err := service.BuildCustomerContractUserRules(snapshot, user.Group)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, gin.H{
		"contract_mode": true, "contract_version": snapshot.Version, "models": rules,
	})
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
