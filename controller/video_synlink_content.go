package controller

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
)

// applySynlinkLastFrameSource uses only the frozen connection and task identity,
// never a Provider-supplied URL, before attaching a credential. The caller keeps
// SSRF protection and disables redirects for this authenticated request.
func applySynlinkLastFrameSource(task *model.Task, req *http.Request, contentURL *string) (bool, error) {
	if task.PrivateData.VideoUpstreamProtocol != dto.VideoUpstreamProtocolSynlinkVideoV1 {
		return false, nil
	}
	_, err := relaycommon.ResolveVideoSouthboundAdapterVersion(constant.ChannelTypeSeedanceLink, dto.VideoUpstreamProfileThirdPartySynlinkVideoV1, task.PrivateData.SouthboundAdapterVersion)
	if err != nil {
		return true, err
	}
	baseURL, key := task.PrivateData.VideoUpstreamQueryBaseURL, task.PrivateData.Key
	request := task.PrivateData.ClientRequest
	if baseURL == "" || key == "" || task.GetUpstreamTaskID() == "" || request == nil || request.ReturnLastFrame == nil || !*request.ReturnLastFrame {
		return true, fmt.Errorf("frozen last-frame connection is unavailable")
	}
	*contentURL = strings.TrimRight(baseURL, "/") + dto.SynlinkLastFramePath(task.GetUpstreamTaskID())
	req.Header.Set("Authorization", "Bearer "+key)
	return true, nil
}
