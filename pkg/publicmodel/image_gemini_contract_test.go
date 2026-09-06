package publicmodel_test

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/pkg/publicmodel"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/service"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPublishedImageEditAlternativesMatchParser(t *testing.T) {
	edit := publicmodel.GeminiImageAPI("customer-image").Image.Edit
	for _, input := range edit.RequiredOneOf {
		t.Run(input, func(t *testing.T) {
			body := map[string]any{"model": edit.Model, "prompt": "edit"}
			switch input {
			case "image":
				body[input] = "https://media.example/image.png"
			case "images":
				body[input] = []string{"https://media.example/image.png"}
			default:
				t.Fatalf("unverified input alternative %q", input)
			}
			encoded, err := common.Marshal(body)
			require.NoError(t, err)
			var request dto.ImageRequest
			require.NoError(t, common.Unmarshal(encoded, &request))
			contract, apiErr := service.ParseImageContract(nil, &relaycommon.RelayInfo{RelayMode: relayconstant.RelayModeImagesEdits}, &request)
			require.Nil(t, apiErr)
			assert.Len(t, contract.Images, 1)
		})
	}
}
