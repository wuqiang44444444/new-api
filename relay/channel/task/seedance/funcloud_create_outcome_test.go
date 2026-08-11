package seedance

import (
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/dto"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFunCloudApplicationErrorsDoNotMarkCreateAsTerminal(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, code := range []int{10002, 10005, 10006, 30003, 90003} {
		t.Run(fmt.Sprintf("code_%d", code), func(t *testing.T) {
			context, _ := gin.CreateTestContext(nil)
			adaptor := TaskAdaptor{profile: dto.VideoUpstreamProfileThirdPartyFunCloudSeedanceV2}
			response := &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader(fmt.Sprintf(`{"code":%d,"msg":"provider error","data":{}}`, code))),
			}

			_, _, taskErr := adaptor.DoResponse(context, response, &relaycommon.RelayInfo{})

			require.NotNil(t, taskErr)
			assert.NotEqual(t, relaycommon.TaskCreateTerminalRejection, relaycommon.GetTaskCreateDisposition(context))
		})
	}
}
