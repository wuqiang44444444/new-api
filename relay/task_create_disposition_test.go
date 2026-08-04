package relay

import (
	"net/http"
	"testing"

	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/stretchr/testify/assert"
)

func TestJSONVideoCreateHTTPDispositionFailsClosedWithoutVerifiedProviderContract(t *testing.T) {
	info := &relaycommon.RelayInfo{
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelOtherSettings: dto.ChannelOtherSettings{
				VideoUpstreamProfile: dto.VideoUpstreamProfileThirdPartyJSONVideoMediaArrays,
			},
		},
	}

	for _, status := range []int{
		http.StatusBadRequest,
		http.StatusUnauthorized,
		http.StatusForbidden,
		http.StatusCreated,
		http.StatusAccepted,
		http.StatusTemporaryRedirect,
		http.StatusTooManyRequests,
		http.StatusConflict,
		http.StatusNotFound,
		http.StatusUnprocessableEntity,
		http.StatusInternalServerError,
		http.StatusBadGateway,
	} {
		assert.Equal(t,
			relaycommon.TaskCreateOutcomeUnknown,
			taskCreateHTTPDisposition(info, status, "provider_error"),
		)
	}
}

func TestUnregisteredTaskCreateHTTPDispositionFailsClosed(t *testing.T) {
	info := &relaycommon.RelayInfo{}

	assert.Equal(t, relaycommon.TaskCreateOutcomeUnknown, taskCreateHTTPDisposition(info, http.StatusOK, ""))
	assert.Equal(t, relaycommon.TaskCreateOutcomeUnknown, taskCreateHTTPDisposition(info, http.StatusAccepted, ""))
	assert.Equal(t, relaycommon.TaskCreateOutcomeUnknown, taskCreateHTTPDisposition(info, http.StatusTemporaryRedirect, ""))
	assert.Equal(t, relaycommon.TaskCreateOutcomeUnknown, taskCreateHTTPDisposition(info, http.StatusBadRequest, "invalid_request"))
	assert.Equal(t, relaycommon.TaskCreateOutcomeUnknown, taskCreateHTTPDisposition(info, http.StatusTooManyRequests, "rate_limited"))
	assert.Equal(t, relaycommon.TaskCreateOutcomeUnknown, taskCreateHTTPDisposition(info, http.StatusInternalServerError, "internal_error"))
}

func TestTokenSaveDoubaoInsufficientQuotaIsTerminalRejection(t *testing.T) {
	info := &relaycommon.RelayInfo{
		OriginModelName: model.VideoSKUDoubaoSeedance20260128,
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelOtherSettings: dto.ChannelOtherSettings{
				VideoUpstreamProfile: dto.VideoUpstreamProfileThirdPartyRelay,
			},
		},
	}

	assert.Equal(t,
		relaycommon.TaskCreateTerminalRejection,
		taskCreateHTTPDisposition(info, http.StatusForbidden, "user_quota_insufficient"),
	)
	assert.Equal(t,
		relaycommon.TaskCreateOutcomeUnknown,
		taskCreateHTTPDisposition(info, http.StatusForbidden, "insufficient_quota"),
	)
	assert.Equal(t,
		relaycommon.TaskCreateOutcomeUnknown,
		taskCreateHTTPDisposition(info, http.StatusUnauthorized, "user_quota_insufficient"),
	)
}

func TestFeicaiAccountRequiredIsTerminalRejection(t *testing.T) {
	info := &relaycommon.RelayInfo{
		OriginModelName: model.VideoSKUSeedance20Value720P,
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelOtherSettings: dto.ChannelOtherSettings{
				VideoUpstreamProfile: dto.VideoUpstreamProfileThirdPartyJSONVideoMediaArrays,
				LinkImplementation: dto.LinkImplementationRef{
					ID: model.LinkImplementationFeicaiSeedanceVideos, Version: model.LinkImplementationVersionV1,
				},
			},
		},
	}

	assert.Equal(t,
		relaycommon.TaskCreateTerminalRejection,
		taskCreateHTTPDisposition(info, http.StatusForbidden, "feicai_account_required"),
	)
	assert.Equal(t,
		relaycommon.TaskCreateOutcomeUnknown,
		taskCreateHTTPDisposition(info, http.StatusUnauthorized, "feicai_account_required"),
	)
	assert.Equal(t,
		relaycommon.TaskCreateOutcomeUnknown,
		taskCreateHTTPDisposition(info, http.StatusForbidden, "account_required"),
	)
}
