package controller

import (
	"errors"
	"net/http"
	"sort"
	"strconv"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"
)

type customerContractTemplateWriteRequest struct {
	ExpectedVersion *int64                      `json:"expected_version"`
	Enabled         *bool                       `json:"enabled"`
	Name            string                      `json:"name"`
	Reason          string                      `json:"reason"`
	Rules           []customerContractRuleInput `json:"rules"`
}

// GetCustomerContractTemplates returns one shared template list page with
// derived model/rule/stale counts.
func GetCustomerContractTemplates(c *gin.Context) {
	keyword := c.Query("keyword")
	var enabled *bool
	switch c.Query("enabled") {
	case "true":
		value := true
		enabled = &value
	case "false":
		value := false
		enabled = &value
	}
	page := common.GetPageQuery(c)
	items, total, err := model.GetCustomerContractTemplateList(keyword, enabled, page.GetStartIdx(), page.GetPageSize())
	if err != nil {
		common.ApiError(c, err)
		return
	}
	page.SetTotal(int(total))
	page.SetItems(items)
	common.ApiSuccess(c, page)
}

// GetCustomerContractTemplateOptions serves the no-customer-context admin
// source options for template editing: every concrete group with its models,
// qualified channels and a plain group-ratio price reference. It never
// applies a customer's or the administrator's special group ratio.
func GetCustomerContractTemplateOptions(c *gin.Context) {
	groupRatios := ratio_setting.GetGroupRatioCopy()
	groupNames := make([]string, 0, len(groupRatios))
	for group := range groupRatios {
		if group != "auto" {
			groupNames = append(groupNames, group)
		}
	}
	sort.Strings(groupNames)
	options := make([]customerContractGroupOption, 0, len(groupNames))
	channelGroups := make([]customerContractChannelGroupOption, 0, len(groupNames))
	for _, group := range groupNames {
		// Plain native group ratio only: templates have no customer context,
		// so the reference price is never a target user's deal price.
		nativeRatio := ratio_setting.GetGroupRatio(group)
		models, err := model.GetCustomerContractAvailableModelsForGroup(group)
		if err != nil {
			common.ApiError(c, err)
			return
		}
		sort.Strings(models)
		prices := make(map[string]service.CustomerContractPricePreview, len(models))
		for _, modelName := range models {
			prices[modelName] = service.BuildCustomerContractPricePreview(modelName, decimal.NewFromFloat(nativeRatio))
		}
		options = append(options, customerContractGroupOption{
			Group: group, Models: models, Prices: prices,
			NativeGroupRatio: decimal.NewFromFloat(nativeRatio).String(), SpecialGroupRatio: false,
		})
		channelModels, err := model.GetCustomerContractEntityChannelOptions(group)
		if err != nil {
			common.ApiError(c, err)
			return
		}
		channelGroups = append(channelGroups, customerContractChannelGroupOption{
			Group: group, Models: channelModels,
			NativeGroupRatio: decimal.NewFromFloat(nativeRatio).String(), SpecialGroupRatio: false,
		})
	}
	common.ApiSuccess(c, gin.H{
		"options":          options,
		"channels":         channelGroups,
		"customer_context": false,
	})
}

func GetCustomerContractTemplate(c *gin.Context) {
	templateId, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "invalid template id"})
		return
	}
	snapshot, err := model.GetContractTemplateSnapshot(templateId, true)
	if err != nil {
		if errors.Is(err, model.ErrCustomerContractTemplateNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"success": false, "message": "template not found"})
		} else {
			common.ApiError(c, err)
		}
		return
	}
	common.ApiSuccess(c, snapshot)
}

func PostCustomerContractTemplate(c *gin.Context) {
	var request customerContractTemplateWriteRequest
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
	snapshot, err := model.CreateCustomerContractTemplate(model.CreateCustomerContractTemplateParams{
		AdminUserId: c.GetInt("id"), Name: request.Name,
		Enabled: enabled, Reason: request.Reason, Rules: rules,
	})
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": err.Error()})
		return
	}
	recordManageAuditFor(c, 0, "contract_template.create", map[string]interface{}{
		"template_id": snapshot.Id, "version": snapshot.Version, "enabled": snapshot.Enabled, "rule_count": len(snapshot.Rules),
	})
	common.ApiSuccess(c, snapshot)
}

func PutCustomerContractTemplate(c *gin.Context) {
	templateId, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "invalid template id"})
		return
	}
	var request customerContractTemplateWriteRequest
	if err := common.DecodeJson(c.Request.Body, &request); err != nil || request.ExpectedVersion == nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "expected_version and a valid request body are required"})
		return
	}
	rules, err := parseCustomerContractEntityRules(request.Rules)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": err.Error()})
		return
	}
	snapshot, err := model.ReplaceCustomerContractTemplate(model.ReplaceCustomerContractTemplateParams{
		TemplateId: templateId, AdminUserId: c.GetInt("id"), ExpectedVersion: *request.ExpectedVersion,
		Name: request.Name, Enabled: request.Enabled, Reason: request.Reason, Rules: rules,
	})
	if err != nil {
		status := http.StatusBadRequest
		if errors.Is(err, model.ErrCustomerContractTemplateVersionConflict) {
			status = http.StatusConflict
		}
		if errors.Is(err, model.ErrCustomerContractTemplateNotFound) {
			status = http.StatusNotFound
		}
		c.JSON(status, gin.H{"success": false, "message": err.Error()})
		return
	}
	recordManageAuditFor(c, 0, "contract_template.update", map[string]interface{}{
		"template_id": snapshot.Id, "version": snapshot.Version, "enabled": snapshot.Enabled, "rule_count": len(snapshot.Rules),
	})
	common.ApiSuccess(c, snapshot)
}

func GetCustomerContractTemplateAudits(c *gin.Context) {
	templateId, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "invalid template id"})
		return
	}
	if _, err := model.GetContractTemplateSnapshot(templateId, false); err != nil {
		if errors.Is(err, model.ErrCustomerContractTemplateNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"success": false, "message": "template not found"})
		} else {
			common.ApiError(c, err)
		}
		return
	}
	page := common.GetPageQuery(c)
	audits, total, err := model.GetContractTemplateAudits(templateId, page.GetStartIdx(), page.GetPageSize())
	if err != nil {
		common.ApiError(c, err)
		return
	}
	page.SetTotal(int(total))
	page.SetItems(audits)
	common.ApiSuccess(c, page)
}
