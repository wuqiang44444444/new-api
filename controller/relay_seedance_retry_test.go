package controller

import (
	"errors"
	"net/http"
	"testing"

	"github.com/QuantumNous/new-api/constant"
	taskdto "github.com/QuantumNous/new-api/dto"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/stretchr/testify/assert"
)

func TestSeedanceTaskNeverRetriesOrSwitchesChannel(t *testing.T) {
	info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{ChannelType: constant.ChannelTypeSeedanceLink}}
	taskErr := &taskdto.TaskError{Error: errors.New("provider unavailable"), StatusCode: http.StatusServiceUnavailable}

	assert.False(t, shouldRetryTaskRelay(nil, info, taskErr, 3))
}
