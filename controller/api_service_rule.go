package controller

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

func GetCurrentAPIServiceRule(c *gin.Context) {
	rule, acceptance, err := service.CurrentAPIServiceRuleAcceptance(c.GetInt("id"), c.GetInt("token_id"))
	if err != nil {
		if errors.Is(err, service.ErrAPIServiceRuleUnavailable) {
			assetAPIError(c, http.StatusServiceUnavailable, "api_service_rule_unavailable", "API service rule is not configured")
		} else {
			assetAPIError(c, http.StatusInternalServerError, "database_error", "failed to load API service rule")
		}
		return
	}
	c.JSON(http.StatusOK, gin.H{"rule": rule, "acceptance": apiServiceRuleAcceptanceResponse(rule, acceptance, c.GetInt("token_id"))})
}

func AcceptCurrentAPIServiceRule(c *gin.Context) {
	var req dto.AcceptAPIServiceRuleRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		assetAPIError(c, http.StatusBadRequest, "invalid_request", "invalid API service rule acceptance")
		return
	}
	acceptance, err := service.AcceptCurrentAPIServiceRule(c.GetInt("id"), c.GetInt("token_id"), req)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrAPIServiceRuleUnavailable):
			assetAPIError(c, http.StatusServiceUnavailable, "api_service_rule_unavailable", "API service rule is not configured")
		case errors.Is(err, service.ErrAPIServiceRuleMismatch):
			assetAPIError(c, http.StatusConflict, "api_service_rule_changed", "API service rule changed; fetch the current version and accept again")
		case errors.Is(err, service.ErrInvalidAPIServiceRule):
			assetAPIError(c, http.StatusBadRequest, "invalid_request", "API service rule acceptance is incomplete")
		default:
			assetAPIError(c, http.StatusInternalServerError, "database_error", "failed to record API service rule acceptance")
		}
		return
	}
	rule, _ := model.GetActiveAPIServiceRule()
	c.JSON(http.StatusOK, apiServiceRuleAcceptanceResponse(rule, acceptance, c.GetInt("token_id")))
}

func CreateAPIServiceRule(c *gin.Context) {
	var req dto.CreateAPIServiceRuleRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		assetAPIError(c, http.StatusBadRequest, "invalid_request", "invalid API service rule")
		return
	}
	rule, err := service.CreateAPIServiceRule(req)
	if err != nil {
		if errors.Is(err, service.ErrInvalidAPIServiceRule) {
			assetAPIError(c, http.StatusBadRequest, "invalid_request", "invalid API service rule")
		} else {
			assetAPIError(c, http.StatusInternalServerError, "database_error", "failed to create API service rule")
		}
		return
	}
	c.JSON(http.StatusCreated, rule)
}

func ListAPIServiceRules(c *gin.Context) {
	rules, err := model.ListAPIServiceRules()
	if err != nil {
		assetAPIError(c, http.StatusInternalServerError, "database_error", "failed to list API service rules")
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": rules})
}

func ListAPIServiceRuleAcceptances(c *gin.Context) {
	appID, _ := strconv.Atoi(c.Query("app_id"))
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 || pageSize > 100 {
		pageSize = 20
	}
	acceptances, total, err := model.ListApplicationAPIRuleAcceptances(appID, (page-1)*pageSize, pageSize)
	if err != nil {
		assetAPIError(c, http.StatusInternalServerError, "database_error", "failed to list API service rule acceptances")
		return
	}
	data := make([]dto.AdminAPIServiceRuleAcceptanceResponse, 0, len(acceptances))
	for i := range acceptances {
		a := &acceptances[i]
		data = append(data, dto.AdminAPIServiceRuleAcceptanceResponse{
			AppID: a.AppID, UserID: a.UserID, RuleVersion: a.RuleVersion, ContentSHA256: a.ContentSHA256,
			AcceptedAt: a.AcceptedAt, AcceptedBy: a.AcceptedBy, AcceptanceMethod: a.AcceptanceMethod,
			ComplianceOwner: a.ComplianceOwner, ConsentRecordSystemRef: a.ConsentRecordSystemRef,
		})
	}
	c.JSON(http.StatusOK, gin.H{"data": data, "total": total, "page": page, "page_size": pageSize})
}

func ActivateAPIServiceRule(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("rule_id"), 10, 64)
	if err != nil || id <= 0 {
		assetAPIError(c, http.StatusBadRequest, "invalid_request", "invalid API service rule id")
		return
	}
	if err := model.ActivateAPIServiceRule(id); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			assetAPIError(c, http.StatusNotFound, "api_service_rule_not_found", "API service rule not found")
		} else if errors.Is(err, model.ErrAPIServiceRuleNotEffective) {
			assetAPIError(c, http.StatusConflict, "api_service_rule_not_effective", "API service rule cannot be activated before its effective time")
		} else {
			assetAPIError(c, http.StatusInternalServerError, "database_error", "failed to activate API service rule")
		}
		return
	}
	c.Status(http.StatusNoContent)
}

func apiServiceRuleAcceptanceResponse(rule *model.APIServiceRule, acceptance *model.ApplicationAPIRuleAcceptance, appID int) dto.APIServiceRuleAcceptanceResponse {
	response := dto.APIServiceRuleAcceptanceResponse{AppID: appID}
	if rule != nil {
		response.Version = rule.Version
		response.ContentSHA = rule.ContentSHA256
	}
	if acceptance != nil {
		response.Accepted = true
		response.AcceptedAt = acceptance.AcceptedAt
	}
	return response
}
