package service

import (
	"slices"
	"strings"

	"github.com/MoeForge/moego/ojson"
	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/gin-gonic/gin"
)

type ArkCacheRequestRoute struct {
	Model, Previous        string
	Enabled, Prefix, Store bool
	HasExpiry, Stream      bool
}

func ReadArkCacheRequestRoute(c *gin.Context) (ArkCacheRequestRoute, bool) {
	const key = "ark_cache_request_route"
	if route, ok := common.GetContextKeyType[ArkCacheRequestRoute](c, constant.ContextKey(key)); ok {
		return route, true
	}
	storage, err := common.GetBodyStorage(c)
	if err != nil {
		return ArkCacheRequestRoute{}, false
	}
	fields, err := common.ReadBodyJSONFields(storage, "model", "previous_response_id", "caching.type", "caching.prefix", "store", "expire_at", "stream")
	if err != nil || len(fields) != 7 {
		return ArkCacheRequestRoute{}, false
	}
	route := ArkCacheRequestRoute{
		Model: fields[0].String(), Previous: fields[1].String(),
		Enabled: fields[2].Type == ojson.String && fields[2].String() == "enabled",
		Prefix:  fields[3].Bool(), Store: fields[4].Type == ojson.True,
		HasExpiry: fields[5].Exists(), Stream: fields[6].Bool(),
	}
	common.SetContextKey(c, constant.ContextKey(key), route)
	return route, true
}

func (r ArkCacheRequestRoute) PaidSession(pilot *ArkPrefixCachePilot) bool {
	return pilot.ChargesSessionStorage() && r.Model == pilot.Model && r.SessionWrite()
}

func (r ArkCacheRequestRoute) SessionWrite() bool {
	return r.Enabled && r.Store && !r.Prefix &&
		(r.Previous == "" && r.HasExpiry || strings.HasPrefix(r.Previous, model.ArkSessionCacheIDPrefix))
}

func (r ArkCacheRequestRoute) ReplayDigest(pilot *ArkPrefixCachePilot, key string) string {
	if key == "" || len(key) > 191 || r.Model != pilot.Model || !r.Enabled {
		return ""
	}
	if r.Prefix && r.Previous == "" && (!pilot.ChargesSessionStorage() || r.Store && r.HasExpiry) {
		return model.ArkPrefixCacheKeyDigest(key)
	}
	if !r.Prefix && (r.Previous == "" || strings.HasPrefix(r.Previous, model.ArkSessionCacheIDPrefix)) &&
		(!pilot.ChargesSessionStorage() || r.PaidSession(pilot)) {
		return model.ArkSessionCacheKeyDigest(key)
	}
	return ""
}

// These markers require the request to finish inside the Ark handler.
const ArkCacheReplayOnlyKey = "ark_cache_replay_only"
const ArkCacheContractExemptKey = "ark_cache_contract_exempt"
const ArkCacheBindingKey = "ark_cache_request_binding"

// PaidArkCacheRoute checks eligibility without changing native routing context.
func PaidArkCacheRoute(c *gin.Context, pilot *ArkPrefixCachePilot, group string) bool {
	if pin, pinned, _ := GetChannelConstraints(c).ResolvedPin(); pinned && pin.ChannelId != pilot.ChannelID {
		return false
	}
	const key = "ark_cache_paid_channel"
	channel, loaded := common.GetContextKeyType[*model.Channel](c, constant.ContextKey(key))
	if !loaded {
		channel, _ = ArkSessionChannel(c.Request.Context(), pilot, pilot.ChannelID, "")
		common.SetContextKey(c, constant.ContextKey(key), channel)
	}
	if channel == nil || channel.Status != common.ChannelStatusEnabled {
		return false
	}
	if snapshot := ActiveCustomerContract(c); snapshot != nil {
		return slices.ContainsFunc(snapshot.Rules, func(rule model.ContractEntityRule) bool {
			return model.ArkCacheContractRouteAllowed(pilot, channel, rule)
		})
	}
	return group != "" && group != "auto" && ratio_setting.ContainsGroupRatio(group) && slices.Contains(channel.GetGroups(), group) &&
		(!pilot.NativeChannel || model.IsChannelEnabledForGroupModel(group, pilot.Model, channel.Id))
}
