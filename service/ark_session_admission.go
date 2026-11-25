package service

import (
	"context"
	"strings"

	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/gin-gonic/gin"
)

func defaultArkSessionBinding(publicModel string, channelID int) *ArkPrefixCachePilot {
	return &ArkPrefixCachePilot{
		Mode: "charge", MeterVersion: setting.ArkSessionPlatformStorageMeter,
		Model: publicModel, ChannelID: channelID, NativeChannel: true,
		Endpoint: ArkPrefixCacheModelVersion, ModelVersion: ArkPrefixCacheModelVersion,
		MaxInputBytes: 8192, MaxActive: 8, MaxHourlyWrites: 8, MaxTTLSeconds: 1800, MaxHeld: 100,
	}
}

// DefaultArkSessionBinding is only reached for an explicit stored Session
// write. Use existing model prices and channel abilities as the feature gate.
func DefaultArkSessionBinding(c *gin.Context, route ArkCacheRequestRoute, userID, tokenID int, group string) *ArkPrefixCachePilot {
	if route.Model == "" || !route.SessionWrite() || model.DB == nil || !setting.ArkDefaultSessionEnabled() {
		return nil
	}
	ctx := c.Request.Context()
	if route.Previous != "" {
		prior, err := model.FindArkPrefixCache(ctx, userID, tokenID, route.Previous)
		if err != nil {
			return nil
		}
		return ArkSessionBindingForClaim(ctx, route.Model, prior)
	}
	if route.Stream {
		return nil
	}
	var contractRoutes map[int]string
	if snapshot := ActiveCustomerContract(c); snapshot != nil {
		contractRoutes = make(map[int]string)
		for _, rule := range snapshot.Rules {
			if rule.PublicModel == route.Model && ratio_setting.ContainsGroupRatio(rule.RouteGroup) {
				contractRoutes[rule.ChannelId] = rule.RouteGroup
			}
		}
		if len(contractRoutes) == 0 {
			return nil
		}
	} else if group == "" || group == "auto" || !ratio_setting.ContainsGroupRatio(group) {
		return nil
	}
	price, err := model.GetModelStorageHourlyExpr(ctx, route.Model)
	if err != nil {
		return nil
	}
	if price == "" {
		key := strings.TrimSpace(c.GetHeader("Idempotency-Key"))
		if key != "" && len(key) <= 191 {
			prior, findErr := model.FindArkPrefixCacheClaim(ctx, userID, tokenID, model.ArkSessionCacheKeyDigest(key))
			if findErr == nil {
				return ArkSessionBindingForClaim(ctx, route.Model, prior)
			}
		}
		return nil
	}
	routes, err := model.NativeArkSessionRoutes(ctx, route.Model, group, contractRoutes)
	if err != nil || len(routes) == 0 {
		return nil
	}
	if pin, pinned, _ := GetChannelConstraints(c).ResolvedPin(); pinned {
		if _, allowed := routes[pin.ChannelId]; !allowed {
			return nil
		}
		return defaultArkSessionBinding(route.Model, pin.ChannelId)
	}
	channel, err := model.GetChannel(group, route.Model, 0, []dto.ChannelFilter{{Kind: dto.FilterContractRoutes, Routes: routes}})
	if err != nil || !model.NativeArkSessionChannel(channel, route.Model) {
		return nil
	}
	return defaultArkSessionBinding(route.Model, channel.Id)
}

func ArkSessionBindingForClaim(ctx context.Context, publicModel string, row model.ArkPrefixCache) *ArkPrefixCachePilot {
	if row.Model != publicModel || row.SessionRound < 1 || row.MeterVersion != setting.ArkSessionPlatformStorageMeter {
		return nil
	}
	binding := defaultArkSessionBinding(publicModel, row.ChannelID)
	binding.Endpoint = row.Endpoint
	// Existing managed resources on dedicated channels retain their origin.
	var channel model.Channel
	if err := model.DB.WithContext(ctx).Select("models").Take(&channel, row.ChannelID).Error; err != nil {
		return nil
	}
	binding.NativeChannel = strings.TrimSpace(channel.Models) != ""
	return binding
}

func ArkSessionChannel(ctx context.Context, pilot *ArkPrefixCachePilot, id int, credential string) (*model.Channel, string) {
	if !pilot.NativeChannel {
		return ArkCacheChannel(ctx, id, credential)
	}
	var channel model.Channel
	if err := model.DB.WithContext(ctx).Take(&channel, id).Error; err != nil ||
		!model.NativeArkSessionChannel(&channel, pilot.Model) || pilot.Endpoint != ArkPrefixCacheModelVersion {
		return nil, ""
	}
	key := strings.TrimSpace(channel.Key)
	if credential != "" && credential != ArkCacheDigest(key) {
		return nil, ""
	}
	return &channel, key
}
