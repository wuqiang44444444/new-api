package openai

import (
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestImageTaskStreamCollectsOnlyCompletedImagesAndLastUsage(t *testing.T) {
	body := ": heartbeat\r\n\r\nevent: image_generation.partial_image\r\ndata: {\"type\":\"image_generation.partial_image\",\"b64_json\":\"preview\"}\r\n\r\n" +
		"data: {\"type\":\"image_generation.completed\",\"b64_json\":\"first\",\n" +
		"data: \"usage\":{\"input_tokens\":3,\"output_tokens\":4}}\n\n" +
		"data: {\"type\":\"image_edit.completed\",\"b64_json\":\"second\",\"usage\":{\"input_tokens\":5,\"output_tokens\":6}}\n\n" +
		"data: [DONE]\n\n"
	images, usage, err := ImageTaskStreamResponse(nil, []byte(body))
	require.NoError(t, err)
	require.Len(t, images, 2)
	assert.Equal(t, "first", images[0].B64Json)
	assert.Equal(t, "second", images[1].B64Json)
	require.NotNil(t, usage)
	assert.Equal(t, 5, usage.PromptTokens)
	assert.Equal(t, 6, usage.CompletionTokens)
	assert.Equal(t, 11, usage.TotalTokens, "cumulative usage must not be summed across events")
}

func TestImageTaskStreamRejectsUncertainResults(t *testing.T) {
	complete := "data: {\"type\":\"image_generation.completed\",\"b64_json\":\"image\"}\n\n"
	for name, body := range map[string]string{
		"partial_only":    "data: {\"type\":\"image_generation.partial_image\",\"b64_json\":\"preview\"}\n\ndata: [DONE]\n\n",
		"invalid_json":    complete + "data: {\"type\":\n\n",
		"late_error":      complete + "data: {\"type\":\"error\",\"message\":\"private provider detail\"}\n\n",
		"missing_image":   "data: {\"type\":\"image_edit.completed\"}\n\n",
		"too_many_images": strings.Repeat(complete, int(dto.MaxImageN)+1),
	} {
		t.Run(name, func(t *testing.T) {
			images, usage, err := ImageTaskStreamResponse(nil, []byte(body))
			require.Error(t, err)
			assert.NotContains(t, err.Error(), "private provider detail")
			assert.Nil(t, images)
			assert.Nil(t, usage)
		})
	}
}

func TestImageTaskStreamPreservesAbsentAndZeroUsageAtEOF(t *testing.T) {
	for _, tc := range []struct {
		suffix string
		absent bool
	}{{"", true}, {`,"usage":{"input_tokens":0,"output_tokens":0}`, false}} {
		images, usage, err := ImageTaskStreamResponse(nil, []byte(`data: {"type":"image_edit.completed","b64_json":"image"`+tc.suffix+"}"))
		require.NoError(t, err)
		require.Len(t, images, 1)
		if tc.absent {
			assert.Nil(t, usage)
		} else {
			require.NotNil(t, usage)
			assert.Zero(t, usage.TotalTokens)
		}
	}
}
