package constant

// Local typed channel identities follow the rc.44 native 62/63 slots.
// Slots 64-66 remain reserved for the previous local layout and retired types.
const (
	ChannelTypeAsyncImage = 68
	ChannelTypeAzureBatch = 70
	localChannelTypeCount = 71
)

func init() {
	for len(ChannelBaseURLs) < localChannelTypeCount {
		ChannelBaseURLs = append(ChannelBaseURLs, "")
	}
	ChannelBaseURLs[ChannelTypeSeedanceLink] = "https://ark.cn-beijing.volces.com"
	ChannelBaseURLs[ChannelTypeAsyncImage] = "https://mm-internal-cn.leonecloud.com"
	ChannelTypeNames[ChannelTypeSeedanceLink] = "Seedance Link"
	ChannelTypeNames[ChannelTypeMiniMaxLink] = "MiniMax Link"
	ChannelTypeNames[ChannelTypeAsyncImage] = "Image Relay"
	ChannelTypeNames[ChannelTypeAzureBatch] = "Azure Batch"
}
