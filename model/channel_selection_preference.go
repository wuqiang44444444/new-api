package model

import (
	"slices"

	"github.com/QuantumNous/new-api/common"
)

// ChannelSelectionPreference is an optional preference within the layer the
// ordinary selector would use. SelectedPriority reports the effective layer.
type ChannelSelectionPreference struct {
	ChannelID        int
	FallbackChannelID int
	Policy *ChannelSelectionPolicy
	SelectedPriority int64
	dbChannelEnabled bool
	dbFallbackEnabled bool
}

// The caller holds the channel cache lock and has already filtered the IDs.
func (p *ChannelSelectionPreference) preferredInMemory(ids []int, priority int64) *Channel {
	if p == nil {
		return nil
	}
	p.SelectedPriority = priority
	for _, id := range []int{p.ChannelID, p.FallbackChannelID} {
		preferred := channelsIDM[id]
		if preferred != nil && preferred.Status == common.ChannelStatusEnabled && preferred.GetPriority() == priority && slices.Contains(ids, id) {
			return preferred
		}
	}
	return nil
}

// ChannelSelectionPolicy applies only after the request's authorization and
// protocol filters. It can exclude candidates and cap the retry priority.
type ChannelSelectionPolicy struct {
	Eligible func([]*Channel) map[int]bool
	MaxPriority *int64
}

func (p *ChannelSelectionPreference) limitCandidates(channels []*Channel) map[int]bool {
	if p == nil || p.Policy == nil {
		return nil
	}
	return p.Policy.Eligible(channels)
}

func (p *ChannelSelectionPreference) canSelect(id int, priority int64, eligible map[int]bool) bool {
	return p == nil || p.Policy == nil ||
		(eligible[id] && (p.Policy.MaxPriority == nil || priority <= *p.Policy.MaxPriority))
}
