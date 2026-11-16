package service

import (
	"errors"
	"fmt"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/pkg/cachex"
)

func ClearChannelAffinityCacheAll() (int, error) {
	legacy, legacyErr := clearAffinityCache(getChannelAffinityCache())
	defaults, defaultErr := clearAffinityCache(defaultAffinityCache())
	users, userErr := clearAffinityCache(userAffinityCache())
	if err := errors.Join(legacyErr, defaultErr, userErr); err != nil {
		common.SysError(common.LogText("channel affinity cache clear failed: session=%t api_key=%t user=%t", legacyErr != nil, defaultErr != nil, userErr != nil))
		return legacy + defaults + users, fmt.Errorf("clear channel affinity caches: %w", err)
	}
	return legacy + defaults + users, nil
}

func clearAffinityCache(cache *cachex.HybridCache[int]) (int, error) {
	keys, err := cache.Keys()
	if err != nil {
		return 0, err
	}
	removed, err := cache.DeleteMany(keys)
	deleted := 0
	for _, ok := range removed {
		if ok {
			deleted++
		}
	}
	return deleted, err
}

func affinityEntryCount(cache *cachex.HybridCache[int]) *int {
	keys, err := cache.Keys()
	if err != nil {
		return nil
	}
	return common.GetPointer(len(keys))
}

func ClearChannelAffinityLayer(layer string) (int, error) {
	cache := channelAffinityCacheForLayer(layer)
	if cache == nil {
		return 0, common.NewMessage("Invalid parameters")
	}
	return clearAffinityCache(cache)
}

func channelAffinityCacheForLayer(layer string) *cachex.HybridCache[int] {
	switch layer {
	case "session":
		return getChannelAffinityCache()
	case "api_key":
		return defaultAffinityCache()
	case "user":
		return userAffinityCache()
	default:
		return nil
	}
}
