package dto

import "strings"

func ModelArkVideoProfileIncompatibility(request *ModelArkVideoCreateRequest, profile VideoUpstreamProfile, allowServiceTier bool) string {
	if request == nil {
		return "Seedance request is unavailable"
	}
	if request.ServiceTier != nil && strings.TrimSpace(*request.ServiceTier) == "flex" && !allowServiceTier {
		return "service_tier \"flex\" is not supported by the selected video channel"
	}
	if profile == VideoUpstreamProfileThirdPartyJSONVideoMediaArrays {
		return ModelArkVideoMediaArraysIncompatibility(request)
	}
	return ""
}
