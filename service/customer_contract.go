package service

import (
	"errors"
	"fmt"
	"strings"
	"sync"

	"github.com/QuantumNous/new-api/model"
	hosttypes "github.com/QuantumNous/new-api/types"
	"github.com/shopspring/decimal"
)

var (
	ErrCustomerContractUnavailable = errors.New("customer contract unavailable")
	ErrCustomerContractModelDenied = errors.New("model is not included in the customer contract")
)

type cachedCustomerContract struct {
	Version  int64
	Snapshot *model.CustomerContractSnapshot
}

var customerContractCache sync.Map

// ParseCustomerContractRatio accepts the admin UI's decimal, percentage and
// Chinese-discount notation, then returns the canonical eight-decimal fixed
// point value used by persistence and billing.
func ParseCustomerContractRatio(raw string) (int64, error) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return 0, fmt.Errorf("contract ratio is required")
	}
	divisor := decimal.NewFromInt(1)
	switch {
	case strings.HasSuffix(value, "%"):
		value = strings.TrimSpace(strings.TrimSuffix(value, "%"))
		divisor = decimal.NewFromInt(100)
	case strings.HasSuffix(value, "折"):
		value = strings.TrimSpace(strings.TrimSuffix(value, "折"))
		divisor = decimal.NewFromInt(10)
	}
	parsed, err := decimal.NewFromString(value)
	if err != nil {
		return 0, fmt.Errorf("invalid contract ratio: %w", err)
	}
	parsed = parsed.Div(divisor)
	if !parsed.GreaterThan(decimal.Zero) || parsed.GreaterThan(decimal.NewFromInt(1)) {
		return 0, fmt.Errorf("contract ratio must be greater than zero and no greater than one")
	}
	scaled := parsed.Mul(decimal.NewFromInt(hosttypes.CustomerContractRatioScale))
	if !scaled.Equal(scaled.Truncate(0)) {
		return 0, fmt.Errorf("contract ratio supports at most eight decimal places")
	}
	units := scaled.IntPart()
	if units <= 0 || units > hosttypes.CustomerContractRatioScale {
		return 0, fmt.Errorf("contract ratio is outside the supported range")
	}
	return units, nil
}

func FormatCustomerContractRatio(units int64) (string, error) {
	if units <= 0 || units > hosttypes.CustomerContractRatioScale {
		return "", fmt.Errorf("invalid contract ratio units")
	}
	return decimal.NewFromInt(units).
		Div(decimal.NewFromInt(hosttypes.CustomerContractRatioScale)).
		String(), nil
}

// LoadCustomerContractSnapshot loads one immutable contract definition. The
// expected version comes from the auth-version-fenced UserBase cache. A version
// mismatch is an authorization failure, never a native-mode fallback.
func LoadCustomerContractSnapshot(userId int, expectedVersion int64) (*model.CustomerContractSnapshot, error) {
	if userId <= 0 || expectedVersion <= 0 {
		return nil, fmt.Errorf("%w: invalid user or version", ErrCustomerContractUnavailable)
	}
	if cached, ok := customerContractCache.Load(userId); ok {
		entry := cached.(cachedCustomerContract)
		if entry.Version == expectedVersion {
			return cloneCustomerContractSnapshot(entry.Snapshot), nil
		}
	}
	snapshot, err := model.GetCustomerContractSnapshotWithoutAvailability(userId)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrCustomerContractUnavailable, err)
	}
	if !snapshot.Enabled || snapshot.Version != expectedVersion {
		return nil, fmt.Errorf("%w: expected version %d, got enabled=%t version=%d", ErrCustomerContractUnavailable, expectedVersion, snapshot.Enabled, snapshot.Version)
	}
	stored := cloneCustomerContractSnapshot(snapshot)
	customerContractCache.Store(userId, cachedCustomerContract{Version: expectedVersion, Snapshot: stored})
	return cloneCustomerContractSnapshot(stored), nil
}

func ResolveCustomerContractRule(userId int, expectedVersion int64, publicModel string) (*hosttypes.ContractBillingFact, error) {
	snapshot, err := LoadCustomerContractSnapshot(userId, expectedVersion)
	if err != nil {
		return nil, err
	}
	for _, rule := range snapshot.Rules {
		if rule.PublicModel == publicModel {
			return &hosttypes.ContractBillingFact{
				UserId: userId, ContractVersion: expectedVersion, PublicModel: rule.PublicModel,
				RouteGroup: rule.RouteGroup, RatioUnits: rule.RatioUnits,
			}, nil
		}
	}
	return nil, fmt.Errorf("%w: %s", ErrCustomerContractModelDenied, publicModel)
}

func RefreshCustomerContractAvailability(snapshot *model.CustomerContractSnapshot) error {
	if snapshot == nil {
		return fmt.Errorf("contract snapshot is nil")
	}
	availableByGroup := make(map[string]map[string]struct{})
	for i := range snapshot.Rules {
		group := snapshot.Rules[i].RouteGroup
		availableModels, ok := availableByGroup[group]
		if !ok {
			models, err := model.GetCustomerContractAvailableModelsForGroup(group)
			if err != nil {
				return err
			}
			availableModels = make(map[string]struct{}, len(models))
			for _, modelName := range models {
				availableModels[modelName] = struct{}{}
			}
			availableByGroup[group] = availableModels
		}
		_, snapshot.Rules[i].Available = availableModels[snapshot.Rules[i].PublicModel]
	}
	return nil
}

func ApplyCustomerContractRatio(value decimal.Decimal, fact *hosttypes.ContractBillingFact) (decimal.Decimal, error) {
	if fact == nil {
		return value, nil
	}
	ratio := fact.RatioDecimal()
	if ratio.IsZero() {
		return decimal.Zero, fmt.Errorf("invalid customer contract billing fact")
	}
	return value.Mul(ratio), nil
}

func cloneCustomerContractSnapshot(snapshot *model.CustomerContractSnapshot) *model.CustomerContractSnapshot {
	if snapshot == nil {
		return nil
	}
	clone := *snapshot
	clone.Rules = append([]model.CustomerContractRule(nil), snapshot.Rules...)
	return &clone
}

func ResetCustomerContractCacheForTest() {
	customerContractCache.Range(func(key any, _ any) bool {
		customerContractCache.Delete(key)
		return true
	})
}

func InvalidateCustomerContractCache(userId int) {
	customerContractCache.Delete(userId)
}
