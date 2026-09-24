package model

import (
	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"gorm.io/gorm"
)

// typedStandardVideoChannelTypes are the typed channels sharing the ModelArk
// V3 standard entry. A customer model can be enabled on at most one enabled
// channel across these types; the check runs only on management writes.
var typedStandardVideoChannelTypes = []int{
	constant.ChannelTypeSeedanceLink,
	constant.ChannelTypeMiniMaxLink,
}

func isTypedStandardVideoChannelType(channelType int) bool {
	for _, typedType := range typedStandardVideoChannelTypes {
		if typedType == channelType {
			return true
		}
	}
	return false
}

// ValidateMiniMaxChannelModelUniqueness keeps Link/native price keys distinct
// and enforces one enabled typed channel per model across the standard entry
// on management writes. Runtime routing does not repeat this audit or repair
// direct database edits.
func ValidateMiniMaxChannelModelUniqueness(tx *gorm.DB, channel *Channel) error {
	if channel == nil {
		return nil
	}
	if err := validateMinimaxPublishedChannelConfiguration(tx, channel); err != nil {
		return err
	}
	if err := validateSeedancePricingOwnership(tx, channel); err != nil {
		return err
	}
	if channel.Type != constant.ChannelTypeMiniMaxLink || channel.Status != common.ChannelStatusEnabled {
		return nil
	}
	return validateTypedStandardVideoModelConflict(tx, channel, "MiniMax Link")
}
