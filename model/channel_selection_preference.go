package model

import (
	"slices"

	"github.com/QuantumNous/new-api/common"
)

// ChannelSelectionPreference is an optional preference within the layer the
// ordinary selector would use. SelectedPriority reports the effective layer.
type ChannelSelectionPreference struct {
	ChannelID        int
	SelectedPriority int64
	dbChannelEnabled bool
}

// The caller holds the channel cache lock and has already filtered the IDs.
func (p *ChannelSelectionPreference) preferredInMemory(ids []int, priority int64) *Channel {
	if p == nil {
		return nil
	}
	p.SelectedPriority = priority
	preferred := channelsIDM[p.ChannelID]
	if preferred != nil && preferred.Status == common.ChannelStatusEnabled && preferred.GetPriority() == priority && slices.Contains(ids, preferred.Id) {
		return preferred
	}
	return nil
}
