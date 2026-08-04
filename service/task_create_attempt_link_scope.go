package service

import (
	"fmt"

	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relaykit/dto"
)

// resolveTaskAttemptLinkImplementation freezes implementation identity only
// for explicitly registered Link SKUs. Native contracts must stay unmarked.
func resolveTaskAttemptLinkImplementation(contractSKU string, ref dto.LinkImplementationRef) (model.LinkImplementation, error) {
	if contractSKU == "" {
		return model.LinkImplementation{}, nil
	}
	if !model.IsRegisteredLinkSKU(contractSKU) {
		return model.LinkImplementation{}, fmt.Errorf("published Link contract SKU %q is not registered", contractSKU)
	}
	implementation, registered := model.ResolveLinkImplementation(ref)
	if !registered {
		return model.LinkImplementation{}, fmt.Errorf("channel Link implementation is not registered")
	}
	return implementation, nil
}
