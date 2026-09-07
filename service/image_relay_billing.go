package service

import (
	"encoding/json"
	"fmt"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
)

// ImageRelayBillingBody strips media and adds a host-validated count for param().
// Reused at admission and snapshot creation so frozen settlement sees identical facts.
func ImageRelayBillingBody(request *dto.ImageRequest, count int) ([]byte, error) {
	if count < 0 || count > MaxImageInputs {
		return nil, fmt.Errorf("invalid image input count")
	}
	clean := *request
	clean.Image, clean.Images, clean.Mask = nil, nil, nil
	body, err := common.Marshal(clean)
	if err != nil {
		return nil, err
	}
	var fields map[string]json.RawMessage
	if err := common.Unmarshal(body, &fields); err != nil {
		return nil, err
	}
	fields["input_image_count"], err = common.Marshal(count)
	if err != nil {
		return nil, err
	}
	return common.Marshal(fields)
}
