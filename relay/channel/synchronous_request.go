package channel

import (
	"net/http"

	"github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
)

func doNativeAPIRequest(c *gin.Context, req *http.Request, info *common.RelayInfo) (*http.Response, error) {
	if info.UsesSynchronousTextCancellation() {
		return doSynchronousRequest(c, req, info)
	}
	return doRequest(c, req, info)
}

func doSynchronousRequest(c *gin.Context, req *http.Request, info *common.RelayInfo) (*http.Response, error) {
	downstream := c.Request.Context()
	return doRequestWithSender(c, req, info, func(client *http.Client, req *http.Request) (*http.Response, error) {
		return service.DoSynchronousTextRequest(client, req, downstream)
	})
}
