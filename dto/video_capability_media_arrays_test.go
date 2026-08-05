package dto

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestModelArkVideoMediaArraysIncompatibilityValidatesTransportBeforeHold(t *testing.T) {
	wrongMIME := &ModelArkVideoCreateRequest{
		Model: "seedance-2.0-standard-720p",
		Content: []ModelArkVideoContent{{
			Type:     "image_url",
			ImageURL: &VideoMediaURL{URL: "data:audio/mpeg;base64,YQ=="},
		}},
	}
	assert.Contains(t, ModelArkVideoMediaArraysIncompatibility(wrongMIME), "image input is invalid")

	asset := &ModelArkVideoCreateRequest{
		Model: "seedance-2.0-standard-720p",
		Content: []ModelArkVideoContent{{
			Type:     "image_url",
			ImageURL: &VideoMediaURL{URL: "asset://ast_example"},
		}},
	}
	assert.Empty(t, ModelArkVideoMediaArraysIncompatibility(asset))

	audioData := &ModelArkVideoCreateRequest{
		Model: "seedance-2.0-standard-720p",
		Content: []ModelArkVideoContent{{
			Type:     "audio_url",
			AudioURL: &VideoMediaURL{URL: "data:audio/mpeg;base64,YQ=="},
		}},
	}
	assert.Contains(t, ModelArkVideoMediaArraysIncompatibility(audioData), "data URL is not supported")

	for _, item := range []ModelArkVideoContent{
		{Type: "audio_url", AudioURL: &VideoMediaURL{URL: "http://media.example.com/reference.mp3"}},
		{Type: "video_url", VideoURL: &VideoMediaURL{URL: "http://media.example.com/reference.mp4"}},
	} {
		request := &ModelArkVideoCreateRequest{Content: []ModelArkVideoContent{item}}
		assert.Contains(t, ModelArkVideoMediaArraysIncompatibility(request), "https URL")
	}

	videoAsset := &ModelArkVideoCreateRequest{Content: []ModelArkVideoContent{{
		Type: "video_url", VideoURL: &VideoMediaURL{URL: "asset://ast_video"},
	}}}
	assert.Empty(t, ModelArkVideoMediaArraysIncompatibility(videoAsset))
}
