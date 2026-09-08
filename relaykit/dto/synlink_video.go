package dto

import "net/url"

func SynlinkLastFramePath(taskID string) string {
	return "/v1/video/files/" + url.PathEscape(taskID) + "/last-frame"
}

// SynlinkVideoModels lists the exact Provider IDs in the supplied Synlink guide.
// It makes no claims about undocumented per-model parameter limits.
func SynlinkVideoModels() []string {
	return []string{"doubao-seedance-2-0-260128", "doubao-seedance-2-0-fast-260128", "doubao-seedance-2-0-mini-260615", "doubao-seedance-2-5-260628"}
}

// SupportsPlatformHostedImages is the code-backed pairing boundary. The stored
// asset protocol and resource IDs remain unchanged so existing assets are shared.
func (p VideoUpstreamProtocol) SupportsPlatformHostedImages() bool {
	return p == VideoUpstreamProtocolFunCloudModelArkV3 || p == VideoUpstreamProtocolSynlinkVideoV1
}
