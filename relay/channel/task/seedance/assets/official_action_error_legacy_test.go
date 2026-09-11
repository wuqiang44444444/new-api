// Package assets implements Seedance asset-library protocols.
package assets

import (
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
)

func officialActionHTTPError(statusCode int, responseBody []byte) error {
	var envelope struct {
		ResponseMetadata struct {
			Error struct {
				Code  string `json:"Code"`
				CodeN int    `json:"CodeN"`
			} `json:"Error"`
		} `json:"ResponseMetadata"`
	}
	if err := common.Unmarshal(responseBody, &envelope); err != nil {
		return &upstreamHTTPError{StatusCode: statusCode}
	}
	providerCode := strings.TrimSpace(envelope.ResponseMetadata.Error.Code)
	if len(providerCode) > 128 {
		providerCode = ""
	}
	for _, character := range providerCode {
		if character >= 'a' && character <= 'z' ||
			character >= 'A' && character <= 'Z' ||
			character >= '0' && character <= '9' ||
			character == '.' || character == '_' || character == '-' {
			continue
		}
		providerCode = ""
		break
	}
	if providerCode == "" && envelope.ResponseMetadata.Error.CodeN != 0 {
		providerCode = strconv.Itoa(envelope.ResponseMetadata.Error.CodeN)
	}
	return &upstreamHTTPError{StatusCode: statusCode, ProviderCode: providerCode}
}
