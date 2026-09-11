package seedance

import (
	"fmt"
	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/gin-gonic/gin"
	"strings"
)

// Only the host can resolve frozen asset facts and sign a temporary URL. The
// compiled conversion is kept unsigned so neither scripts nor snapshots see it.
func resolvePluginHostedContent(c *gin.Context, data []byte, protocol dto.VideoUpstreamProtocol) ([]byte, error) {
	var body map[string]any
	if err := common.Unmarshal(data, &body); err != nil {
		return nil, err
	}
	contentBytes, err := common.Marshal(body["content"])
	if err != nil {
		return nil, err
	}
	var content []ContentItem
	if err = common.Unmarshal(contentBytes, &content); err != nil {
		return nil, err
	}
	content, err = resolveHostedImageContent(c, content)
	if err != nil {
		return nil, err
	}
	if err = rejectUnresolvedHostedMedia(content); err != nil {
		return nil, err
	}
	if protocol == dto.VideoUpstreamProtocolSynlinkVideoV1 {
		for _, item := range content {
			for _, media := range []*MediaURL{item.ImageURL, item.VideoURL, item.AudioURL} {
				if media != nil && strings.HasPrefix(strings.TrimSpace(media.URL), "asset://") {
					return nil, fmt.Errorf("unresolved asset reference cannot be sent to this video protocol")
				}
			}
		}
	}
	body["content"] = content
	return common.Marshal(body)
}
