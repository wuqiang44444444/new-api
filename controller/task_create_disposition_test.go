package controller

import (
	"errors"
	"net/http"
	"testing"

	taskdto "github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

func TestTaskCreateDispositionPrecedesLegacyRetryPolicy(t *testing.T) {
	gin.SetMode(gin.TestMode)
	tests := []struct {
		name        string
		disposition relaycommon.TaskCreateDisposition
		taskErr     *taskdto.TaskError
		want        bool
	}{
		{
			name:        "safe transport failure may use existing retry policy",
			disposition: relaycommon.TaskCreateSafeToRetryBeforeCreate,
			taskErr:     &taskdto.TaskError{StatusCode: http.StatusBadGateway, Error: errors.New("safe failure")},
			want:        true,
		},
		{
			name:        "terminal rejection never retries",
			disposition: relaycommon.TaskCreateTerminalRejection,
			taskErr:     &taskdto.TaskError{StatusCode: http.StatusBadGateway, Error: errors.New("rejected")},
		},
		{
			name:        "unknown outcome never retries",
			disposition: relaycommon.TaskCreateOutcomeUnknown,
			taskErr:     &taskdto.TaskError{StatusCode: http.StatusTooManyRequests, Error: errors.New("unknown")},
		},
		{
			name:    "missing disposition fails closed",
			taskErr: &taskdto.TaskError{StatusCode: http.StatusBadGateway, Error: errors.New("unclassified")},
		},
		{
			name:        "local validation still follows existing no-retry rule",
			disposition: relaycommon.TaskCreateSafeToRetryBeforeCreate,
			taskErr: &taskdto.TaskError{
				StatusCode: http.StatusInternalServerError,
				LocalError: true,
				Error:      errors.New("local"),
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			context, _ := gin.CreateTestContext(nil)
			if test.disposition != "" {
				relaycommon.SetTaskCreateDisposition(context, test.disposition)
			}
			info := &relaycommon.RelayInfo{
				TaskRelayInfo: &relaycommon.TaskRelayInfo{ClientProtocol: model.TaskClientProtocolModelArkV3},
			}
			assert.Equal(t, test.want, shouldRetryTaskRelay(context, info, test.taskErr, 1))
		})
	}
}

func TestTaskCreateDispositionDoesNotChangeLegacyNonVideoRetry(t *testing.T) {
	context, _ := gin.CreateTestContext(nil)
	taskErr := &taskdto.TaskError{
		StatusCode: http.StatusBadGateway,
		Error:      errors.New("legacy upstream failure"),
	}
	info := &relaycommon.RelayInfo{TaskRelayInfo: &relaycommon.TaskRelayInfo{}}

	assert.True(t, shouldRetryTaskRelay(context, info, taskErr, 1))
}
