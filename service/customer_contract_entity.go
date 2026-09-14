package service

import (
	"errors"
	"fmt"
	"sync"

	"github.com/QuantumNous/new-api/model"
	hosttypes "github.com/QuantumNous/new-api/types"
)

type cachedContractEntity struct {
	authVersion int64
	snapshot    *model.ContractEntitySnapshot
}

// customerContractEntityCache caches loaded contract entities by contract id.
// Entries are fenced by the owner's authentication version: every contract
// write bumps that version, so a stale entry can never be reused.
var customerContractEntityCache sync.Map

// LoadContractEntityForRequest loads one contract entity for request-side
// resolution. A mismatched or missing authorization version is an
// authorization failure, never a native-mode fallback.
func LoadContractEntityForRequest(userId int, authVersion int64, contractId int) (*model.ContractEntitySnapshot, error) {
	if userId <= 0 || contractId <= 0 || authVersion <= 0 {
		return nil, fmt.Errorf("%w: invalid user or contract", ErrCustomerContractUnavailable)
	}
	if authVersion > 0 {
		if cached, ok := customerContractEntityCache.Load(contractId); ok {
			entry := cached.(cachedContractEntity)
			if entry.authVersion == authVersion && entry.snapshot.UserId == userId {
				return cloneContractEntitySnapshot(entry.snapshot), nil
			}
		}
	}
	snapshot, err := model.GetContractEntitySnapshot(contractId, false)
	if err != nil {
		if errors.Is(err, model.ErrCustomerContractEntityNotFound) {
			return nil, fmt.Errorf("%w: contract %d does not exist", ErrCustomerContractUnavailable, contractId)
		}
		return nil, fmt.Errorf("%w: %v", ErrCustomerContractUnavailable, err)
	}
	if snapshot.UserId != userId {
		return nil, fmt.Errorf("%w: contract owner mismatch", ErrCustomerContractUnavailable)
	}
	stored := cloneContractEntitySnapshot(snapshot)
	customerContractEntityCache.Store(contractId, cachedContractEntity{authVersion: authVersion, snapshot: stored})
	return cloneContractEntitySnapshot(stored), nil
}

func cloneContractEntitySnapshot(snapshot *model.ContractEntitySnapshot) *model.ContractEntitySnapshot {
	if snapshot == nil {
		return nil
	}
	clone := *snapshot
	clone.Rules = append([]model.ContractEntityRule(nil), snapshot.Rules...)
	return &clone
}

// ResolveContractEntityRule freezes the contract discount fact for one public
// model of a key-bound contract. The three outcomes are: discount fact (the
// enabled contract lists the model), nil fact (disabled contract, or the
// contract simply does not list the model — native handling applies), or an
// error (binding, read, version or discount-consistency anomaly: fail closed,
// never a silent native fallback). All rules of one public model must agree on
// the discount; any disagreement is a hard failure.
func ResolveContractEntityRule(userId int, authVersion int64, contractId int, publicModel string) (*hosttypes.ContractBillingFact, error) {
	snapshot, err := LoadContractEntityForRequest(userId, authVersion, contractId)
	if err != nil {
		return nil, err
	}
	if !snapshot.Enabled {
		return nil, nil
	}
	discounts, err := ContractDiscountsFromSnapshot(snapshot)
	if err != nil {
		return nil, err
	}
	discount, found := discounts[publicModel]
	if !found {
		return nil, nil
	}
	return &hosttypes.ContractBillingFact{
		UserId: userId, ContractId: snapshot.Id, ContractVersion: snapshot.Version,
		PublicModel: publicModel, RatioUnits: discount,
	}, nil
}

// InvalidateContractEntityCache drops one contract's cached snapshot after a
// committed write.
func InvalidateContractEntityCache(contractId int) {
	customerContractEntityCache.Delete(contractId)
}

func ResetContractEntityCacheForTest() {
	customerContractEntityCache.Range(func(key any, _ any) bool {
		customerContractEntityCache.Delete(key)
		return true
	})
}
