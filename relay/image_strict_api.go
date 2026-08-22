package relay

import "github.com/QuantumNous/new-api/constant"

func isStrictImageAPIType(apiType int) bool {
	return apiType == constant.APITypeAsyncImage
}
