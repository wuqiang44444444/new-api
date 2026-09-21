package service

import "github.com/gin-gonic/gin"

// This is an observation of the test, not a supplier settlement or a wallet
// operation. Before DoRequest, not_sent proves no generation request was sent.
// Once the adaptor is invoked, pending is conservative even if the transport
// never connects: an error or missing response cannot prove a free request.
// priced means the local frozen pricing evidence is complete, not reconciled
// with a supplier invoice. Older events without this observation stay unknown.
const ChannelTestUpstreamCostKey = "upstream_cost_status"

func ChannelTestUpstreamCostStatus(c *gin.Context) string {
	if c == nil {
		return ""
	}
	switch status := c.GetString(ChannelTestUpstreamCostKey); status {
	case "not_sent", "pending", "priced":
		return status
	default:
		return ""
	}
}
