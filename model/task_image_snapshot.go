package model

import (
	"slices"
	"strings"

	"github.com/QuantumNous/new-api/dto"
)

func MediaImageTaskSnapshotIsCurrent(task *Task) bool {
	if task == nil || task.ClientProtocol != TaskClientProtocolOpenAIImages {
		return false
	}
	publicModel := strings.TrimSpace(task.Properties.OriginModelName)
	capability, ok := ResolveImageSKUCapability(publicModel)
	if !ok {
		return !IsRegisteredLinkSKU(publicModel)
	}
	if task.PrivateData.NorthboundContractID != capability.ContractID ||
		task.PrivateData.NorthboundContractVersion != capability.Version ||
		task.PrivateData.SKUCapabilityVersion != capability.Version ||
		task.PrivateData.SKUCapabilityHash != capability.ContentHash {
		return false
	}
	implementation, ok := ResolveLinkImplementation(dto.LinkImplementationRef{
		ID: task.PrivateData.LinkImplementationID, Version: task.PrivateData.LinkImplementationVersion,
	})
	return ok && implementation.ContentHash == strings.TrimSpace(task.PrivateData.LinkImplementationHash) &&
		implementation.ContractID == capability.ContractID && slices.Contains(implementation.PublicSKUs, publicModel)
}
