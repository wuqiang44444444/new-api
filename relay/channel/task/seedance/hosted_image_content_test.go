package seedance

import (
	"strings"

	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
)

// resolveHostedMediaContent applies only the URLs prepared before the funding hold.
func resolveHostedMediaContent(c *gin.Context, content []ContentItem) []ContentItem {
	if len(service.GetFunCloudHostedMediaFacts(c)) == 0 {
		return content
	}
	content = append([]ContentItem(nil), content...)
	for i := range content {
		item := &content[i]
		for _, media := range []**MediaURL{&item.ImageURL, &item.VideoURL, &item.AudioURL} {
			if *media == nil {
				continue
			}
			if url := service.GetFunCloudHostedMediaURL(c, strings.TrimSpace((*media).URL)); url != "" {
				*media = &MediaURL{URL: url}
			}
		}
	}
	return content
}
