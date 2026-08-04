package service

import (
	"fmt"

	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relaykit/dto"
)

// resolveTaskAttemptLinkImplementation freezes implementation identity only
// for explicitly registered Link SKUs. Native contracts must stay unmarked.
func resolveTaskAttemptLinkImplementation(publicModel string, ref dto.LinkImplementationRef) (model.LinkImplementation, error) {
	if !model.IsRegisteredLinkSKU(publicModel) {
		return model.LinkImplementation{}, nil
	}
	implementation, registered := model.ResolveLinkImplementation(ref)
	if !registered {
		return model.LinkImplementation{}, fmt.Errorf("channel Link implementation is not registered")
	}
	return implementation, nil
}
