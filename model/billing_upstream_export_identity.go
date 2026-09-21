package model

import "github.com/QuantumNous/new-api/common"

// Active upstream jobs are identified by the requested conditions. Generated
// snapshots stay on the accepted job and must not make a retry a new request.
func sameActiveCustomerExportFilters(jobType, existing, submitted string) bool {
	if existing == submitted {
		return true
	}
	if jobType != CustomerExportJobTypeUpstreamDetails && jobType != CustomerExportJobTypeUpstreamSummary {
		return false
	}
	var left, right CustomerExportFilters
	if common.UnmarshalJsonStr(existing, &left) != nil || common.UnmarshalJsonStr(submitted, &right) != nil || left.Upstream == nil || right.Upstream == nil {
		return false
	}
	for _, scope := range []*UpstreamExportScope{left.Upstream, right.Upstream} {
		scope.UpperLogID = nil
		scope.Channels = nil
		scope.GroupName = ""
		if jobType == CustomerExportJobTypeUpstreamSummary && scope.URLKey != "" {
			scope.ChannelIds = nil
		}
	}
	a, err := common.Marshal(left)
	if err != nil {
		return false
	}
	b, err := common.Marshal(right)
	return err == nil && string(a) == string(b)
}
