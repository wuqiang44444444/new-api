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
	if err := errors.Join(legacyErr, defaultErr); err != nil {
		common.SysError(common.LogText("channel affinity cache clear failed: explicit_rules=%t default_users=%t", legacyErr != nil, defaultErr != nil))
		return legacy + defaults, fmt.Errorf("clear channel affinity caches: %w", err)
	}
	return legacy + defaults, nil
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
