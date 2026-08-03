package service

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
)

var (
	ErrAPIServiceRuleUnavailable = errors.New("API service rule is unavailable")
	ErrAPIServiceRuleNotAccepted = errors.New("current API service rule has not been accepted")
	ErrAPIServiceRuleMismatch    = errors.New("API service rule version or content hash does not match")
	ErrInvalidAPIServiceRule     = errors.New("invalid API service rule")
)

func CreateAPIServiceRule(req dto.CreateAPIServiceRuleRequest) (*model.APIServiceRule, error) {
	req.Version = strings.TrimSpace(req.Version)
	req.Title = strings.TrimSpace(req.Title)
	req.Content = strings.TrimSpace(req.Content)
	if req.Version == "" || req.Title == "" || req.Content == "" || len(req.Version) > 64 || len(req.Title) > 191 {
		return nil, ErrInvalidAPIServiceRule
	}
	if req.EffectiveAt == 0 {
		req.EffectiveAt = common.GetTimestamp()
	}
	hash := sha256.Sum256([]byte(req.Content))
	rule := &model.APIServiceRule{
		Version: req.Version, Title: req.Title, Content: req.Content,
		ContentSHA256: fmt.Sprintf("%x", hash[:]), Status: model.APIServiceRuleDraft,
		EffectiveAt: req.EffectiveAt,
	}
	if err := model.DB.Create(rule).Error; err != nil {
		return nil, err
	}
	return rule, nil
}

func CurrentAPIServiceRuleAcceptance(userID, appID int) (*model.APIServiceRule, *model.ApplicationAPIRuleAcceptance, error) {
	rule, err := model.GetActiveAPIServiceRule()
	if err != nil || rule == nil {
		if err == nil {
			err = ErrAPIServiceRuleUnavailable
		}
		return rule, nil, err
	}
	acceptance, err := model.GetApplicationAPIRuleAcceptance(userID, appID, rule.ID)
	if err != nil {
		return nil, nil, err
	}
	if acceptance != nil && (acceptance.RuleVersion != rule.Version || !strings.EqualFold(acceptance.ContentSHA256, rule.ContentSHA256)) {
		acceptance = nil
	}
	return rule, acceptance, nil
}

func AcceptCurrentAPIServiceRule(userID, appID int, req dto.AcceptAPIServiceRuleRequest) (*model.ApplicationAPIRuleAcceptance, error) {
	rule, err := model.GetActiveAPIServiceRule()
	if err != nil || rule == nil {
		if err == nil {
			err = ErrAPIServiceRuleUnavailable
		}
		return nil, err
	}
	if strings.TrimSpace(req.Version) != rule.Version || !strings.EqualFold(strings.TrimSpace(req.ContentSHA256), rule.ContentSHA256) {
		return nil, ErrAPIServiceRuleMismatch
	}
	req.ComplianceOwner = strings.TrimSpace(req.ComplianceOwner)
	req.ConsentRecordSystemRef = strings.TrimSpace(req.ConsentRecordSystemRef)
	if req.ComplianceOwner == "" || req.ConsentRecordSystemRef == "" || len(req.ComplianceOwner) > 191 || len(req.ConsentRecordSystemRef) > 300 {
		return nil, ErrInvalidAPIServiceRule
	}
	acceptance := &model.ApplicationAPIRuleAcceptance{
		UserID: userID, AppID: appID, RuleID: rule.ID, RuleVersion: rule.Version,
		ContentSHA256: rule.ContentSHA256, AcceptedAt: common.GetTimestamp(),
		AcceptedBy: fmt.Sprintf("token:%d", appID), AcceptanceMethod: "api_token",
		ComplianceOwner: req.ComplianceOwner, ConsentRecordSystemRef: req.ConsentRecordSystemRef,
	}
	if err := model.SaveApplicationAPIRuleAcceptance(acceptance); err != nil {
		return nil, err
	}
	return acceptance, nil
}

func RequireCurrentAPIServiceRuleAcceptance(userID, appID int) error {
	_, acceptance, err := CurrentAPIServiceRuleAcceptance(userID, appID)
	if err != nil {
		return err
	}
	if acceptance == nil {
		return ErrAPIServiceRuleNotAccepted
	}
	return nil
}
