package model

// projectCustomerLog applies the same visibility rules to customer log reads
// and streaming exports. The stored row remains available for reconciliation.
func projectCustomerLog(log *Log) {
	log.ChannelId = 0
	log.ChannelName = ""
	log.UpstreamRequestId = ""
	log.Other = formatLogOtherJSON(log.Other, logOtherVisibilityUser)
}
