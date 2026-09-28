package controller

import (
	"errors"
	"net/http"
	"slices"
	"sort"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/i18n"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
)

type customerContractCatalogSource struct {
	ChannelId         int    `json:"channel_id"`
	ChannelName       string `json:"channel_name"`
	Available         bool   `json:"available"`
	UnavailableReason string `json:"unavailable_reason,omitempty"`
	FromChannelConfig bool   `json:"from_channel_config"`
	FromAbility       bool   `json:"from_ability"`
	// ConfigDiff carries the management-only channel-config vs Ability drift
	// diagnosis. It is independent of Available and never gates anything.
	ConfigDiff string `json:"config_diff,omitempty"`
}

type customerContractCatalogModel struct {
	Model string                                `json:"model"`
	Price *service.CustomerContractPricePreview `json:"price,omitempty"`
	// Sources lists every deduplicated channel source of this model in the
	// group, including unavailable ones with their controlled reason.
	Sources []customerContractCatalogSource `json:"sources"`
}

type customerContractCatalogGroup struct {
	Group             string                         `json:"group"`
	RatioConfigured   bool                           `json:"ratio_configured"`
	NativeGroupRatio  string                         `json:"native_group_ratio"`
	SpecialGroupRatio bool                           `json:"special_group_ratio"`
	Models            []customerContractCatalogModel `json:"models"`
}

type customerContractNoGroupChannel struct {
	ChannelId     int      `json:"channel_id"`
	ChannelName   string   `json:"channel_name"`
	ChannelStatus int      `json:"channel_status"`
	Models        []string `json:"models"`
}

type customerContractCatalogResponse struct {
	Groups          []customerContractCatalogGroup   `json:"groups"`
	NoGroupChannels []customerContractNoGroupChannel `json:"no_group_channels"`
	CustomerContext bool                             `json:"customer_context"`
}

type customerContractWriteRequest struct {
	ExpectedVersion *int64                      `json:"expected_version"`
	Enabled         *bool                       `json:"enabled"`
	Name            string                      `json:"name"`
	Reason          string                      `json:"reason"`
	Rules           []customerContractRuleInput `json:"rules"`
	// Optional template provenance for creation; both must be provided
	// together. Updates never change the stored source.
	SourceTemplateId      *int   `json:"source_template_id"`
	SourceTemplateVersion *int64 `json:"source_template_version"`
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
	sourceTemplateId, sourceTemplateVersion := 0, int64(0)
	if request.SourceTemplateId != nil || request.SourceTemplateVersion != nil {
		if request.SourceTemplateId == nil || request.SourceTemplateVersion == nil ||
			*request.SourceTemplateId <= 0 || *request.SourceTemplateVersion <= 0 {
			c.JSON(http.StatusBadRequest, gin.H{
				"success": false,
				"message": "template source requires both source_template_id and source_template_version",
			})
			return
		}
		sourceTemplateId = *request.SourceTemplateId
		sourceTemplateVersion = *request.SourceTemplateVersion
	}
	snapshot, err := model.CreateCustomerContractEntity(model.CreateCustomerContractParams{
		UserId: target.Id, AdminUserId: c.GetInt("id"), Name: request.Name,
		Enabled: enabled, Reason: request.Reason, Rules: rules,
		SourceTemplateId: sourceTemplateId, SourceTemplateVersion: sourceTemplateVersion,
	})
	if err != nil {
		status := http.StatusBadRequest
		if errors.Is(err, model.ErrCustomerContractTemplateVersionConflict) {
			status = http.StatusConflict
		}
		c.JSON(status, gin.H{"success": false, "message": err.Error()})
		return
	}
	view, err := service.BuildContractEntityAdminViews([]model.ContractEntitySnapshot{*snapshot}, target.Group)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	recordManageAuditFor(c, target.Id, "user.contract.create", map[string]interface{}{
		"contract_id": snapshot.Id, "version": snapshot.Version, "enabled": snapshot.Enabled, "rule_count": len(snapshot.Rules),
		"source_template_id": snapshot.SourceTemplateId,
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

// GetCustomerContractCatalog serves the unified management catalog for
// one customer's contract editor: every connected model source with shared
// availability facts, config-diff diagnostics and this user's price
// reference. It replaces the two per-group option endpoints.
func GetCustomerContractCatalog(c *gin.Context) {
	target, ok := authorizedCustomerContractTarget(c)
	if !ok {
		return
	}
	catalog, err := model.GetCustomerContractCatalog()
	if err != nil {
		common.ApiError(c, err)
		return
	}
	response := buildCustomerContractCatalogResponse(catalog, true, func(group string) (float64, bool) {
		return service.ResolveCustomerContractNativeGroupRatio(target.Group, group)
	})
	common.ApiSuccess(c, response)
}

// buildCustomerContractCatalogResponse merges the catalog projection with the
// caller's group-ratio context. Groups without a configured ratio stay
// visible through their sources' invalid diagnostics; they never become
// addable. Price references are per group context: the customer editor
// applies the target user's special group ratio, templates never do.
func buildCustomerContractCatalogResponse(catalog *model.CustomerContractCatalog, customerContext bool, groupRatio func(group string) (float64, bool)) customerContractCatalogResponse {
	type groupEntry struct {
		ratio           float64
		ratioConfigured bool
		dto             *customerContractCatalogGroup
		models          map[string]*customerContractCatalogModel
	}
	entries := make(map[string]*groupEntry)
	ordered := make([]*groupEntry, 0)
	entry := func(group string) *groupEntry {
		if existing := entries[group]; existing != nil {
			return existing
		}
		ratioConfigured := ratio_setting.ContainsGroupRatio(group)
		var ratio float64
		special := false
		ratioText := ""
		if ratioConfigured {
			// Only configured groups resolve a ratio: an unconfigured group
			// keeps an empty ratio and no price reference instead of a fake
			// "1", and never triggers the missing-ratio log path.
			ratio, special = groupRatio(group)
			ratioText = decimal.NewFromFloat(ratio).String()
		}
		e := &groupEntry{
			ratio:           ratio,
			ratioConfigured: ratioConfigured,
			dto: &customerContractCatalogGroup{
				Group:             group,
				RatioConfigured:   ratioConfigured,
				NativeGroupRatio:  ratioText,
				SpecialGroupRatio: special,
				Models:            []customerContractCatalogModel{},
			},
			models: make(map[string]*customerContractCatalogModel),
		}
		entries[group] = e
		ordered = append(ordered, e)
		return e
	}
	// Concrete ratio-configured groups stay listed even without sources so
	// the route group selector keeps offering every configured group.
	groupNames := make([]string, 0)
	for name := range ratio_setting.GetGroupRatioCopy() {
		if strings.EqualFold(name, "auto") {
			continue
		}
		groupNames = append(groupNames, name)
	}
	sort.Strings(groupNames)
	for _, name := range groupNames {
		entry(name)
	}
	for _, source := range catalog.Sources {
		e := entry(source.RouteGroup)
		modelDto := e.models[source.PublicModel]
		if modelDto == nil {
			modelDto = &customerContractCatalogModel{Model: source.PublicModel, Sources: []customerContractCatalogSource{}}
			e.models[source.PublicModel] = modelDto
		}
		modelDto.Sources = append(modelDto.Sources, customerContractCatalogSource{
			ChannelId:         source.ChannelId,
			ChannelName:       source.ChannelName,
			Available:         source.Available,
			UnavailableReason: source.UnavailableCategory,
			FromChannelConfig: source.FromChannelConfig,
			FromAbility:       source.FromAbility,
			ConfigDiff:        source.ConfigDiff,
		})
	}
	slices.SortFunc(ordered, func(a, b *groupEntry) int { return strings.Compare(a.dto.Group, b.dto.Group) })
	groups := make([]customerContractCatalogGroup, 0, len(ordered))
	for _, e := range ordered {
		modelNames := make([]string, 0, len(e.models))
		for name := range e.models {
			modelNames = append(modelNames, name)
		}
		sort.Strings(modelNames)
		var prices map[string]*service.CustomerContractPricePreview
		if e.ratioConfigured {
			// Unconfigured groups keep every model price reference unset:
			// their sources are invalid anyway and a ratio cannot be faked.
			prices = service.BuildCustomerContractCatalogPrices(modelNames, decimal.NewFromFloat(e.ratio))
		} else {
			prices = map[string]*service.CustomerContractPricePreview{}
		}
		for _, name := range modelNames {
			modelDto := e.models[name]
			modelDto.Price = prices[name]
			e.dto.Models = append(e.dto.Models, *modelDto)
		}
		groups = append(groups, *e.dto)
	}
	noGroupChannels := make([]customerContractNoGroupChannel, 0, len(catalog.NoGroupRecords))
	for _, record := range catalog.NoGroupRecords {
		noGroupChannels = append(noGroupChannels, customerContractNoGroupChannel{
			ChannelId: record.ChannelId, ChannelName: record.ChannelName,
			ChannelStatus: record.ChannelStatus, Models: record.Models,
		})
	}
	return customerContractCatalogResponse{
		Groups: groups, NoGroupChannels: noGroupChannels, CustomerContext: customerContext,
	}
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
	views, err := service.BuildContractEntityUserScopeViews(snapshots)
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
