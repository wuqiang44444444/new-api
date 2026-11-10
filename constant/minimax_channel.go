package constant

// ChannelTypeMiniMaxLink identifies the local MiniMax standard-video typed
// extension (ModelArk V3 northbound, code-registered southbound protocols).
// It mirrors constant/seedance_channel.go: a local typed Link identity kept
// out of the native channel-type enum block so upstream merges cannot collide
// with it. MiniMax Link channels never enter the native Ability distribution
// pool.
const ChannelTypeMiniMaxLink = 69

// VideoUpstreamProtocolJdCloudTaskV1 is the first code-registered southbound
// protocol of the MiniMax Link channel type. It is deliberately not added to
// the Seedance protocol enum in relaykit/dto; validation lives in the typed
// minimax package and the versioned artifact declaration.
const VideoUpstreamProtocolJdCloudTaskV1 = "jdcloud_video_task_v1"
const VideoUpstreamProtocolMiniMaxOfficialV2 = "minimax_official_video_v2"
const VideoUpstreamProtocolNexhinaHailuoV2 = "nexhina_hailuo_v2"

// Fixed JD Cloud task API paths for the protocol. Code-registered, never
// administrator-configured.
const (
	JdCloudCreatePath                = "/v1/task/submit"
	JdCloudQueryPathTemplate         = "/v1/task/{task_id}"
	MiniMaxOfficialCreatePath        = "/v2/video_generation"
	MiniMaxOfficialQueryPathTemplate = "/v2/query/video_generation/{task_id}"
)

// MiniMaxVideoPaths returns the host-owned paths for a registered protocol.
func MiniMaxVideoPaths(protocol string) (createPath, queryPath string, ok bool) {
	switch protocol {
	case VideoUpstreamProtocolJdCloudTaskV1:
		return JdCloudCreatePath, JdCloudQueryPathTemplate, true
	case VideoUpstreamProtocolMiniMaxOfficialV2:
		return MiniMaxOfficialCreatePath, MiniMaxOfficialQueryPathTemplate, true
	case VideoUpstreamProtocolNexhinaHailuoV2:
		return "/hailuo" + MiniMaxOfficialCreatePath, "/hailuo" + MiniMaxOfficialQueryPathTemplate, true
	default:
		return "", "", false
	}
}

func IsMiniMaxVideoV2(protocol string) bool {
	return protocol == VideoUpstreamProtocolMiniMaxOfficialV2 || protocol == VideoUpstreamProtocolNexhinaHailuoV2
}
