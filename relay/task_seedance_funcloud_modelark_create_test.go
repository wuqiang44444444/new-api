package relay

import (
	"github.com/QuantumNous/new-api/constant"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestFunCloudModelArkCreationRejectionRequiresDocumentedError(t *testing.T) {
	info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{ChannelType: constant.ChannelTypeSeedanceLink, ChannelOtherSettings: dto.ChannelOtherSettings{VideoUpstreamProtocol: dto.VideoUpstreamProtocolFunCloudModelArkV3}}}
	body := []byte(`{"error":{"code":"provider_rejected","message":"request rejected"}}`)
	for _, status := range []int{400, 401, 402, 403} {
		assert.True(t, isFunCloudModelArkCreateRejection(info, status, body))
	}
	for _, status := range []int{200, 201, 202, 301, 404, 429, 500, 502} {
		assert.False(t, isFunCloudModelArkCreateRejection(info, status, body))
	}
	for _, body := range []string{`<html>bad request</html>`, `{}`, `{"code":"bad","message":"bad"}`, `{"error":{"code":"bad"}}`, `{"id":"task-1","error":{"code":"bad","message":"bad"}}`} {
		assert.False(t, isFunCloudModelArkCreateRejection(info, 400, []byte(body)))
	}
	info.ChannelOtherSettings.VideoUpstreamProtocol = dto.VideoUpstreamProtocolFunCloudSeedance
	assert.False(t, isFunCloudModelArkCreateRejection(info, 400, body))
}
