package openai

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestImageResponsePreservesReportedTokenModalities(t *testing.T) {
	for _, tc := range []struct {
		name, response string
		want           *dto.ImageTokenUsage
	}{
		{
			name:     "keeps explicit zero and omits absent values",
			response: `{"usage":{"input_tokens":21,"output_tokens":196,"input_tokens_details":{"text_tokens":21,"image_tokens":0},"output_tokens_details":{"image_tokens":196}}}`,
			want: &dto.ImageTokenUsage{
				TextInput: common.GetPointer(21), ImageInput: common.GetPointer(0),
				ImageOutput: common.GetPointer(196),
			},
		},
		{
			name:     "preserves a partial compatible response without reporting contradictory input",
			response: `{"usage":{"prompt_tokens":21,"completion_tokens":196,"prompt_tokens_details":{"text_tokens":21,"image_tokens":1},"completion_tokens_details":{"image_tokens":100}}}`,
			want:     &dto.ImageTokenUsage{ImageOutput: common.GetPointer(100)},
		},
		{
			name:     "aggregate only response has no modality evidence",
			response: `{"usage":{"input_tokens":21,"output_tokens":196}}`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, recorder, resp, info := newImageTestContext(t, tc.response, "application/json", false)
			usage, apiErr := OpenaiImageHandler(ctx, info, resp)
			require.Nil(t, apiErr)
			require.NotNil(t, usage)
			assert.Equal(t, tc.want, usage.ImageTokenUsage)
			assert.JSONEq(t, tc.response, recorder.Body.String())
		})
	}
}
