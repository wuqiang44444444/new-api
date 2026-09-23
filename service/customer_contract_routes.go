package service

import (
	"errors"
	"fmt"
	"sort"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	hosttypes "github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
)

const contractRequestKey = "customer_contract_request"
const contractFirstGroupKey = "customer_contract_first_group"

var ErrCustomerContractScope = errors.New("requested model or channel is outside the available contract scope")

// The request owns this snapshot, including a disabled result. A retry never
// reinterprets an in-flight request after contract edits or token unbinding.
func CustomerContractForRequest(c *gin.Context, userID int, authVersion int64, contractID int) (*model.ContractEntitySnapshot, error) {
	if value, loaded := c.Get(contractRequestKey); loaded {
		snapshot, _ := value.(*model.ContractEntitySnapshot)
		return snapshot, nil
	}
	var snapshot *model.ContractEntitySnapshot
	if contractID > 0 {
		var err error
		snapshot, err = LoadContractEntityForRequest(userID, authVersion, contractID)
		if err != nil {
			return nil, err
		}
		if snapshot.Enabled {
			if snapshot.Version <= 0 {
				return nil, ErrCustomerContractUnavailable
			}
			if _, err := ContractDiscountsFromSnapshot(snapshot); err != nil {
				return nil, err
			}
		} else {
			snapshot = nil
		}
	}
	c.Set(contractRequestKey, snapshot)
	return snapshot, nil
}

func ActiveCustomerContract(c *gin.Context) *model.ContractEntitySnapshot {
	value, _ := c.Get(contractRequestKey)
	snapshot, _ := value.(*model.ContractEntitySnapshot)
	return snapshot
}

func ContractTokenModelAllowed(c *gin.Context, publicModel string) bool {
	if !common.GetContextKeyBool(c, constant.ContextKeyTokenModelLimitEnabled) {
		return true
	}
	value, _ := common.GetContextKey(c, constant.ContextKeyTokenModelLimit)
	limits, _ := value.(map[string]bool)
	return limits[publicModel] || limits[ratio_setting.FormatMatchingModelName(publicModel)] || limits[ratio_setting.RoutingMatchModelName(publicModel)]
}

// EffectiveContractRules uses the contract as the routing authorization source,
// independent of user/Key groups. Runtime and projections share source validity;
// database failures are errors, never an empty/native result.
func EffectiveContractRules(snapshot *model.ContractEntitySnapshot) ([]model.ContractEntityRule, error) {
	availability, err := effectiveContractRulesDetailed(snapshot)
	if err != nil {
		return nil, err
	}
	rules := make([]model.ContractEntityRule, 0, len(availability))
	for _, rule := range availability {
		if rule.Available {
			rules = append(rules, rule)
		}
	}
	return rules, nil
}

// effectiveContractRulesDetailed keeps every rule with its derived availability
// and controlled unavailability category so rejection diagnostics can report
// the actual blocking branch without re-running the judgment.
func effectiveContractRulesDetailed(snapshot *model.ContractEntitySnapshot) ([]model.ContractEntityRule, error) {
	if _, err := ContractDiscountsFromSnapshot(snapshot); err != nil {
		return nil, err
	}
	availability, err := model.GetContractRouteAvailability(snapshot.Rules)
	if err != nil {
		return nil, fmt.Errorf("%w: route lookup failed", ErrCustomerContractUnavailable)
	}
	return availability, nil
}

func ResolveCustomerContractRequest(c *gin.Context, publicModel string) (*hosttypes.ContractBillingFact, error) {
	authVersion, _ := common.GetContextKeyType[int64](c, constant.ContextKeyAuthVersion)
	contractID, _ := common.GetContextKeyType[int](c, constant.ContextKeyTokenContractId)
	snapshot, err := CustomerContractForRequest(c, common.GetContextKeyInt(c, constant.ContextKeyUserId), authVersion, contractID)
	if err != nil || snapshot == nil {
		if err != nil {
			attachContractRejection(c, contractStageScope, contractReasonUnavailable, publicModel, nil)
		}
		return nil, err
	}
	if !ContractTokenModelAllowed(c, publicModel) {
		attachContractRejection(c, "model_access", "token_model_forbidden", publicModel, nil)
		return nil, ErrCustomerContractScope
	}
	fact, err := resolveContractBillingFact(snapshot, publicModel)
	if err != nil {
		if errors.Is(err, ErrCustomerContractScope) {
			attachContractRejection(c, contractStageScope, contractReasonModelNotListed, publicModel, nil)
		} else {
			attachContractRejection(c, contractStageScope, contractReasonUnavailable, publicModel, nil)
		}
		return nil, err
	}
	common.SetContextKey(c, constant.ContextKeyContractFact, fact)
	return fact, nil
}

func customerContractRoutes(c *gin.Context, publicModel string) (map[int]string, error) {
	snapshot := *ActiveCustomerContract(c)
	snapshot.Rules = nil
	for _, rule := range ActiveCustomerContract(c).Rules {
		if rule.PublicModel == publicModel {
			snapshot.Rules = append(snapshot.Rules, rule)
		}
	}
	rules, err := effectiveContractRulesDetailed(&snapshot)
	if err != nil {
		attachContractRejection(c, contractStageSelection, contractReasonRouteLookupFailed, publicModel, nil)
		return nil, err
	}
	routes := make(map[int]string)
	firstGroup := c.GetString(contractFirstGroupKey)
	for _, rule := range rules {
		if !rule.Available || rule.PublicModel != publicModel || !ContractTokenModelAllowed(c, publicModel) {
			continue
		}
		if firstGroup != "" && !common.GetContextKeyBool(c, constant.ContextKeyTokenCrossGroupRetry) && rule.RouteGroup != firstGroup {
			continue
		}
		routes[rule.ChannelId] = rule.RouteGroup
	}
	if len(routes) == 0 {
		// 候选全部不可用与“有可用候选但被当前约束过滤空”是不同事实，分开记录。
		attach := contractReasonCandidatesUnavailable
		for _, rule := range rules {
			if rule.Available {
				attach = contractReasonNoSelectableChannel
				break
			}
		}
		attachContractRejection(c, contractStageSelection, attach, publicModel, contractCandidatesDetail(&snapshot, rules))
		return nil, ErrCustomerContractScope
	}
	return routes, nil
}

func setCustomerContractGroup(c *gin.Context, group string) {
	common.SetContextKey(c, constant.ContextKeyUsingGroup, group)
	common.SetContextKey(c, constant.ContextKeyAutoGroup, group)
	if c.GetString(contractFirstGroupKey) == "" {
		c.Set(contractFirstGroupKey, group)
	}
}

// ValidateCustomerContractChannel handles native pins, new origin-task calls,
// and deterministic typed routes without giving the contract pin precedence.
func ValidateCustomerContractChannel(c *gin.Context, publicModel string, channelID int) error {
	if ActiveCustomerContract(c) == nil {
		return nil
	}
	routes, err := customerContractRoutes(c, publicModel)
	if err != nil {
		return err
	}
	group, allowed := routes[channelID]
	if !allowed {
		attachContractRejection(c, contractStageSelection, contractReasonChannelNotInScope, publicModel, nil)
		return ErrCustomerContractScope
	}
	setCustomerContractGroup(c, group)
	return nil
}

// SelectCustomerContractChannel only composes candidates; native selection
// owns priority and weight. Pins and initial affinity precede random selection.
func SelectCustomerContractChannel(param *RetryParam, initial bool) (*model.Channel, string, error) {
	c := param.Ctx
	routes, err := customerContractRoutes(c, param.ModelName)
	if err != nil {
		return nil, "", err
	}
	constraints := GetChannelConstraints(c)
	filters := append(append([]dto.ChannelFilter(nil), constraints.Filters...), dto.ChannelFilter{Kind: dto.FilterContractRoutes, Routes: routes})
	var selected *model.Channel
	if pin, pinned, _ := constraints.ResolvedPin(); pinned {
		if _, allowed := routes[pin.ChannelId]; !allowed {
			attachContractRejection(c, contractStageSelection, contractReasonChannelNotInScope, param.ModelName, nil)
			return nil, "", ErrCustomerContractScope
		}
		selected, err = model.CacheGetChannel(pin.ChannelId)
		if err != nil {
			return nil, "", err
		}
		if ok, _ := model.ChannelSatisfiesFilters(selected, param.ModelName, filters); !ok {
			attachContractRejection(c, contractStageSelection, contractReasonChannelNotInScope, param.ModelName, nil)
			return nil, "", ErrCustomerContractScope
		}
	} else if initial {
		if id, found := GetPreferredChannelByAffinity(c, param.ModelName, param.TokenGroup); found {
			candidate, loadErr := model.CacheGetChannel(id)
			if loadErr == nil {
				if ok, _ := model.ChannelSatisfiesFilters(candidate, param.ModelName, filters); ok {
					selected = candidate
					MarkChannelAffinityUsed(c, routes[id], id)
				}
			}
			if selected == nil && !ShouldKeepChannelAffinityOnChannelDisabled() {
				ClearCurrentChannelAffinityCache(c)
			}
		}
	}
	if selected == nil {
		selected, err = model.GetRandomSatisfiedChannel(param.TokenGroup, param.ModelName, param.GetRetry(), filters)
	}
	if err != nil {
		return nil, "", err
	}
	if selected == nil {
		attachContractRejection(c, contractStageSelection, contractReasonNoSelectableChannel, param.ModelName, nil)
		return nil, "", ErrCustomerContractScope
	}
	group := routes[selected.Id]
	setCustomerContractGroup(c, group)
	return selected, group, nil
}

// Typed selection stays deterministic and uses the entry's own channel type.
func CustomerContractTypedChannel(c *gin.Context, publicModel string, channelType int, pinnedID int) (*model.Channel, string, error) {
	routes, err := customerContractRoutes(c, publicModel)
	if err != nil {
		return nil, "", err
	}
	ids := make([]int, 0, len(routes))
	for id := range routes {
		ids = append(ids, id)
	}
	sort.Ints(ids)
	pinnedInRoutes := false
	for _, id := range ids {
		if pinnedID != 0 && pinnedID != id {
			continue
		}
		if pinnedID != 0 {
			pinnedInRoutes = true
		}
		channel, err := model.GetChannelById(id, true)
		if err != nil {
			return nil, "", err
		}
		if channel.Type != channelType {
			continue
		}
		setCustomerContractGroup(c, routes[id])
		return channel, routes[id], nil
	}
	// 固定渠道在合同路由内但类型不匹配，与渠道根本不在合同范围内是不同事实。
	if pinnedID == 0 || pinnedInRoutes {
		attachContractRejection(c, contractStageSelection, contractReasonChannelMismatch, publicModel, nil)
	} else {
		attachContractRejection(c, contractStageSelection, contractReasonChannelNotInScope, publicModel, nil)
	}
	return nil, "", ErrCustomerContractScope
}

// ResolveCustomerContractLockedChannel authorizes a new origin-task operation.
// A provisional distributor choice is not a failed attempt; the locked channel
// establishes the first actual group for this new request.
func ResolveCustomerContractLockedChannel(c *gin.Context, publicModel string, channelID int) (*hosttypes.ContractBillingFact, error) {
	fact, err := ResolveCustomerContractRequest(c, publicModel)
	if err != nil || fact == nil {
		return fact, err
	}
	c.Set(contractFirstGroupKey, "")
	if err := ValidateCustomerContractChannel(c, publicModel, channelID); err != nil {
		return nil, err
	}
	return fact, nil
}
