package helper

import (
	"fmt"
	"strings"

	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
)

// validatePublishedLinkExecution closes the distributor-to-adapter gap: the
// selected channel must still resolve the frozen customer publication to the
// exact provider model produced by the ordinary model_mapping pipeline.
func validatePublishedLinkExecution(info *relaycommon.RelayInfo) error {
	if info == nil || strings.TrimSpace(info.PublishedLinkContractSKU) == "" {
		return nil
	}
	channel, err := model.GetChannelById(info.ChannelId, true)
	if err != nil {
		return fmt.Errorf("load published Link execution channel: %w", err)
	}
	execution, err := model.ResolveChannelLinkExecution(channel, info.OriginModelName, model.LinkRouteFamily(info.LinkRouteFamily))
	if err != nil {
		return err
	}
	if execution.LinkSKU != info.PublishedLinkContractSKU {
		return fmt.Errorf("selected channel resolves Link SKU %q instead of published SKU %q", execution.LinkSKU, info.PublishedLinkContractSKU)
	}
	if execution.ProviderModel != strings.TrimSpace(info.UpstreamModelName) {
		return fmt.Errorf("selected channel resolves provider model %q instead of mapped model %q", execution.ProviderModel, info.UpstreamModelName)
	}
	return nil
}
