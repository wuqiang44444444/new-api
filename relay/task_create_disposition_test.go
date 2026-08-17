package relay

import (
	"net/http"
	"testing"

	"github.com/QuantumNous/new-api/constant"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/stretchr/testify/assert"
)

func TestFeicaiVideoCreateHTTPDispositionFailsClosedWithoutVerifiedProviderContract(t *testing.T) {
	info := &relaycommon.RelayInfo{
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelOtherSettings: dto.ChannelOtherSettings{
				VideoUpstreamProfile: dto.VideoUpstreamProfileThirdPartyFeicaiVideos,
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
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelType: constant.ChannelTypeSeedanceLink,
			ChannelOtherSettings: dto.ChannelOtherSettings{
				VideoUpstreamProtocol: dto.VideoUpstreamProtocolTokenSaveMediaTaskV1,
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

func TestFeicaiAccountRequiredRemainsUnknownWithoutExactContract(t *testing.T) {
	info := &relaycommon.RelayInfo{
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelType: constant.ChannelTypeSeedanceLink,
			ChannelOtherSettings: dto.ChannelOtherSettings{
				VideoUpstreamProtocol: dto.VideoUpstreamProtocolFeicaiVideosV1,
			},
		},
	}

	assert.Equal(t,
		relaycommon.TaskCreateOutcomeUnknown,
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
