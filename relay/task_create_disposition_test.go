package relay

import (
	"net/http"
	"testing"

	"github.com/QuantumNous/new-api/constant"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/stretchr/testify/assert"
)

func TestTaskCreateHTTPDispositionRequiresVerifiedRejection(t *testing.T) {
	for _, tc := range []struct {
		name     string
		protocol dto.VideoUpstreamProtocol
		status   int
		body     string
		rejected bool
	}{
		{"TokenSave quota", dto.VideoUpstreamProtocolTokenSaveMediaTaskV1, 403, `{"error":{"code":"user_quota_insufficient","message":"Insufficient quota"}}`, true},
		{"Moxing ModelArk quota", dto.VideoUpstreamProtocolMoxingModelArkV1, 403, `{"code":"user_quota_insufficient","message":"Insufficient quota"}`, true},
		{"TokenSave permission", dto.VideoUpstreamProtocolTokenSaveMediaTaskV1, 403, `{"error":{"code":"group_model_permission_denied","message":"Model unavailable in current group"}}`, true},
		{"official authentication", dto.VideoUpstreamProtocolModelArkV3Volcengine, 401, `{"error":{"code":"AuthenticationError","message":"The API key status is not active."}}`, true},
		{"CMCC authentication", dto.VideoUpstreamProtocolModelArkV3CMCC, 403, `{"message":"api key is invalid"}`, true},
		{"foreign protocol code", dto.VideoUpstreamProtocolMoxingModelArkV1, 403, `{"error":{"code":"group_model_permission_denied","message":"Model unavailable"}}`, false},
		{"foreign authentication", dto.VideoUpstreamProtocolModelArkV3BytePlus, 401, `{"error":{"code":"AuthenticationError","message":"Invalid key"}}`, false},
		{"wrong status", dto.VideoUpstreamProtocolModelArkV3Volcengine, 503, `{"error":{"code":"AuthenticationError","message":"Invalid key"}}`, false},
		{"unregistered quota", dto.VideoUpstreamProtocolTokenSaveMediaTaskV1, 403, `{"error":{"code":"insufficient_quota","message":"Insufficient quota"}}`, false},
		{"CMCC unregistered error", dto.VideoUpstreamProtocolModelArkV3CMCC, 403, `{"message":"request failed"}`, false},
		{"feicai unavailable", dto.VideoUpstreamProtocolFeicaiVideosV1, 503, `{"error":{"code":"model_not_found","message":"No available channel"}}`, false},
		{"feicai account", dto.VideoUpstreamProtocolFeicaiVideosV1, 403, `{"error":{"code":"feicai_account_required","message":"Account required"}}`, false},
		{"HTML", dto.VideoUpstreamProtocolModelArkV3Volcengine, 401, `<html>Unauthorized</html>`, false},
		{"truncated response", dto.VideoUpstreamProtocolModelArkV3Volcengine, 401, `{"error":{"code":"AuthenticationError"`, false},
		{"missing error detail", dto.VideoUpstreamProtocolModelArkV3Volcengine, 401, `{"error":{"code":"AuthenticationError"}}`, false},
		{"conflicting id", dto.VideoUpstreamProtocolModelArkV3Volcengine, 401, `{"id":"task-1","error":{"code":"AuthenticationError","message":"Invalid key"}}`, false},
		{"conflicting task id", dto.VideoUpstreamProtocolTokenSaveMediaTaskV1, 403, `{"task_id":"task-1","error":{"code":"group_model_permission_denied","message":"Denied"}}`, false},
		{"conflicting nested task", dto.VideoUpstreamProtocolTokenSaveMediaTaskV1, 403, `{"data":{"task_id":"task-1"},"error":{"code":"group_model_permission_denied","message":"Denied"}}`, false},
		{"ambiguous success", dto.VideoUpstreamProtocolTokenSaveMediaTaskV1, 200, `{"error":{"code":"group_model_permission_denied","message":"Denied"}}`, false},
		{"FunCloud error", dto.VideoUpstreamProtocolFunCloudModelArkV3, 403, `{"error":{"code":"ContentPolicyViolation","message":"Denied"}}`, true},
		{"FunCloud conflicting task", dto.VideoUpstreamProtocolFunCloudModelArkV3, 403, `{"data":{"id":"task-1"},"error":{"code":"ContentPolicyViolation","message":"Denied"}}`, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{
				ChannelType:          constant.ChannelTypeSeedanceLink,
				ChannelOtherSettings: dto.ChannelOtherSettings{VideoUpstreamProtocol: tc.protocol},
			}}
			want := relaycommon.TaskCreateOutcomeUnknown
			if tc.rejected {
				want = relaycommon.TaskCreateTerminalRejection
			}
			assert.Equal(t, want, taskCreateHTTPDisposition(info, tc.status, []byte(tc.body)))
		})
	}
}

func TestTaskCreateHTTPDispositionDoesNotClassifyNativeChannels(t *testing.T) {
	body := []byte(`{"error":{"code":"AuthenticationError","message":"Invalid key"}}`)
	for _, info := range []*relaycommon.RelayInfo{nil, {}, {ChannelMeta: &relaycommon.ChannelMeta{
		ChannelType:          constant.ChannelTypeDoubaoVideo,
		ChannelOtherSettings: dto.ChannelOtherSettings{VideoUpstreamProtocol: dto.VideoUpstreamProtocolModelArkV3Volcengine},
	}}} {
		assert.Equal(t, relaycommon.TaskCreateOutcomeUnknown, taskCreateHTTPDisposition(info, http.StatusUnauthorized, body))
	}
}
