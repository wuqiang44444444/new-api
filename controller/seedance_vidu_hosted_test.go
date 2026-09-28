package controller

import (
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/dto"
)

func TestViduHostedVideoDurableLifecycle(t *testing.T) {
	for _, region := range []string{"cn", "global"} {
		for _, variant := range []string{"viduq3.1-drama-std", "viduq3-drama-std", "viduq3-drama-fast", "viduq3-drama-mini"} {
			providerModel := variant
			if region == "global" {
				providerModel = strings.Replace(variant, "-drama-", "-drama-ab-", 1)
			}
			t.Run(providerModel, func(t *testing.T) {
				testHostedVideoCreation(t, dto.VideoUpstreamProtocolViduModelArkV3, "accepted", providerModel)
			})
		}
		providerModel := "viduq3-drama-mini"
		if region == "global" {
			providerModel = "viduq3-drama-ab-mini"
		}
		for _, outcome := range []string{"keyframes", "delete_after_accept", "ambiguous", "direct_url", "direct_url_without_library", "other_user", "deleted", "missing", "video_slot", "audio_slot", "non_hosted", "object_missing", "store_unavailable", "location_changed", "opaque"} {
			t.Run(region+"/"+outcome, func(t *testing.T) {
				testHostedVideoCreation(t, dto.VideoUpstreamProtocolViduModelArkV3, outcome, providerModel)
			})
		}
	}
}
