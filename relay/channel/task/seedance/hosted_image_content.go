package seedance

import (
	"strings"

	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
)

// resolveHostedImageContent uses only the facts accepted before the funding hold.
func resolveHostedImageContent(c *gin.Context, content []ContentItem) ([]ContentItem, error) {
	facts := service.GetFunCloudHostedMediaFacts(c)
	if len(facts) > 0 {
		copied := make([]ContentItem, len(content))
		copy(copied, content)
		content = copied
		signed := make(map[string]string, len(facts))
		for i := range content {
			item := &content[i]
			if item.ImageURL == nil {
				continue
			}
			ref := strings.TrimSpace(item.ImageURL.URL)
			fact, ok := facts[ref]
			if !ok {
				continue
			}
			if signed[ref] == "" {
				url, err := service.SignFunCloudHostedAssetURL(c.Request.Context(), fact)
				if err != nil {
					return nil, err
				}
				signed[ref] = url
			}
			item.ImageURL = &MediaURL{URL: signed[ref]}
		}
	}
	return content, nil
}
