package constant

// ChannelTypeMiniMaxLink identifies the local MiniMax standard-video typed
// extension (ModelArk V3 northbound, code-registered southbound protocols).
// It mirrors constant/seedance_channel.go: a local typed Link identity kept
// out of the native channel-type enum block so upstream merges cannot collide
// with it. MiniMax Link channels never enter the native Ability distribution
// pool.
const ChannelTypeMiniMaxLink = 64

// VideoUpstreamProtocolJdCloudTaskV1 is the first code-registered southbound
// protocol of the MiniMax Link channel type. It is deliberately not added to
// the Seedance protocol enum in relaykit/dto; validation lives in the typed
// minimax package and the versioned artifact declaration.
const VideoUpstreamProtocolJdCloudTaskV1 = "jdcloud_video_task_v1"

// Fixed JD Cloud task API paths for the protocol. Code-registered, never
// administrator-configured.
const (
	JdCloudCreatePath        = "/v1/task/submit"
	JdCloudQueryPathTemplate = "/v1/task/{task_id}"
)
