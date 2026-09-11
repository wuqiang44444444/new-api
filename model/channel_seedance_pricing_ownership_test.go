package model

import (
	"fmt"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSeedancePricingOwnershipRejectsCrossTypeChannelInsert(t *testing.T) {
	for _, existingType := range []int{constant.ChannelTypeSeedanceLink, constant.ChannelTypeDoubaoVideo} {
		for _, status := range []int{common.ChannelStatusEnabled, common.ChannelStatusManuallyDisabled} {
			t.Run(fmt.Sprintf("%d/%d", existingType, status), func(t *testing.T) {
				db := withSeedanceChannelDB(t)
				seedPublishedSeedanceTestArtifact(t)
				existing := seedanceTestChannel("shared-price", status)
				existing.Type = existingType
				require.NoError(t, db.Create(existing).Error)
				candidate := seedanceTestChannel("shared-price", common.ChannelStatusManuallyDisabled)
				if existingType == constant.ChannelTypeSeedanceLink {
					candidate.Type = constant.ChannelTypeDoubaoVideo
				}
				require.ErrorContains(t, candidate.Insert(), "distinct customer model names")
				var count int64
				require.NoError(t, db.Model(&Channel{}).Count(&count).Error)
				assert.EqualValues(t, 1, count, "rejected channel must roll back")
				candidate.Id = 0
				candidate.Models = "independent-price"
				require.NoError(t, candidate.Insert(), "an independent customer name resolves the conflict")
			})
		}
	}
}
