package model

import (
	"github.com/QuantumNous/new-api/common"
	"strings"
)

func mappedCustomerModel(channel *Channel, customerModel string) (string, error) {
	mapping := make(map[string]string)
	if raw := strings.TrimSpace(channel.GetModelMapping()); raw != "" && raw != "{}" {
		if err := common.UnmarshalJsonStr(raw, &mapping); err != nil {
			return "", err
		}
	}
	providerModel, _, err := ResolveModelMapping(customerModel, mapping)
	return providerModel, err
}
