package openai

import (
	"github.com/QuantumNous/new-api/common"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
)

// ImageTaskUsage shares native normalization without writing a client response
// or entering request-scoped billing a second time.
func ImageTaskUsage(info *relaycommon.RelayInfo, body []byte) (*dto.Usage, error) {
	var response struct {
		Usage *dto.Usage `json:"usage"`
	}
	if err := common.Unmarshal(body, &response); err != nil {
		return nil, err
	}
	normalizeOpenAIUsage(response.Usage, body)
	applyUsagePostProcessing(info, response.Usage, body)
	return response.Usage, nil
}
