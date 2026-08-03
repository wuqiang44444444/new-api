package dto

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestModelArkJSONVideoIncompatibilityValidatesTransportMediaBeforeHold(t *testing.T) {
	wrongMIME := &ModelArkVideoCreateRequest{
		Model: "seedance-2.0-standard-720p",
		Content: []ModelArkVideoContent{
			{
				Type:     "image_url",
				ImageURL: &VideoMediaURL{URL: "data:audio/mpeg;base64,YQ=="},
			},
		},
	}
	assert.Contains(t, ModelArkJSONVideoIncompatibility(wrongMIME), "image input is invalid")

	valid := &ModelArkVideoCreateRequest{
		Model: "seedance-2.0-standard-720p",
		Content: []ModelArkVideoContent{
			{
				Type:     "image_url",
				ImageURL: &VideoMediaURL{URL: "https://example.com/reference.png"},
			},
		},
	}
	assert.Empty(t, ModelArkJSONVideoIncompatibility(valid))
}
