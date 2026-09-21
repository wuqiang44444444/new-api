package openai

import (
	"fmt"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestImageCacheReadEvidenceAcrossResponseModes(t *testing.T) {
	previousTimeout := constant.StreamingTimeout
	constant.StreamingTimeout = 30
	t.Cleanup(func() { constant.StreamingTimeout = previousTimeout })
	for _, tc := range []struct {
		name, cache, write string
		reported           bool
		tokens             int
	}{
		{"absent", "", "", false, 0},
		{"null", `,"cached_tokens":null`, `,"cache_write_tokens":null`, false, 0},
		{"explicit zero", `,"cached_tokens":0`, `,"cache_write_tokens":0`, true, 0},
		{"cache hit", `,"cached_tokens":80`, `,"cache_write_tokens":80`, true, 80},
		{"creation alias", `,"cached_tokens":80`, `,"cached_creation_tokens":80`, true, 80},
	} {
		t.Run(tc.name, func(t *testing.T) {
			body := fmt.Sprintf(`{"data":[{"b64_json":"image"}],"usage":{"input_tokens":100,"output_tokens":20,"input_tokens_details":{"text_tokens":100%s%s}}}`, tc.cache, tc.write)
			for _, mode := range []string{"json", "sse", "json-as-sse", "task"} {
				t.Run(mode, func(t *testing.T) {
					var usage *dto.Usage
					if mode == "task" {
						var err error
						usage, err = ImageTaskUsage(nil, []byte(body))
						require.NoError(t, err)
						stored, err := common.Marshal(usage)
						require.NoError(t, err)
						usage = &dto.Usage{}
						require.NoError(t, common.Unmarshal(stored, usage))
					} else {
						payload, contentType := body, "application/json"
						if mode == "sse" {
							payload, contentType = "data: "+body+"\n\ndata: [DONE]\n\n", "text/event-stream"
						}
						c, recorder, resp, info := newImageTestContext(t, payload, contentType, mode != "json")
						if mode == "json" {
							result, apiErr := OpenaiImageHandler(c, info, resp)
							require.Nil(t, apiErr)
							usage = result
						} else {
							result, apiErr := OpenaiImageStreamHandler(c, info, resp)
							require.Nil(t, apiErr)
							usage = result
						}
						assert.NotContains(t, recorder.Body.String(), "cache_write_tokens_reported")
						assert.NotContains(t, recorder.Body.String(), "cache_read_tokens_reported", "internal evidence must not change the customer response")
					}
					require.NotNil(t, usage)
					require.NotNil(t, usage.CacheReadTokensReported)
					assert.Equal(t, tc.reported, *usage.CacheReadTokensReported)
					require.NotNil(t, usage.CacheWriteTokensReported)
					assert.Equal(t, tc.reported, *usage.CacheWriteTokensReported)
					assert.Equal(t, tc.tokens, usage.PromptTokensDetails.CacheCreationTokensTotal())
					assert.Equal(t, tc.tokens, usage.PromptTokensDetails.CachedTokens)
					assert.Equal(t, 100, usage.PromptTokens)
					assert.Equal(t, 20, usage.CompletionTokens)
				})
			}
		})
	}
}
