package service

import (
	"context"
	"net/http"
	"testing"

	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// observationUnavailableAdaptor simulates a provider observation whose
// plugin engine infrastructure is unavailable (admission or execution
// timeout wrapped in the shared sentinel).
type observationUnavailableAdaptor struct{}

func (observationUnavailableAdaptor) Init(*relaycommon.RelayInfo) {}

func (observationUnavailableAdaptor) FetchTask(string, string, *model.Task, string) (*http.Response, error) {
	return nil, relaycommon.ErrUpstreamObservationUnavailable
}

func (observationUnavailableAdaptor) ParseTaskResult(*model.Task, *http.Response, []byte) (*relaycommon.TaskInfo, error) {
	return &relaycommon.TaskInfo{}, nil
}

func (observationUnavailableAdaptor) AdjustBillingOnComplete(*model.Task, *relaycommon.TaskInfo) int {
	return 0
}

// TestUpdateVideoSingleTaskSkipsRoundOnObservationUnavailable goes through
// the real polling entry: a local plugin-engine timeout must not count
// toward the consecutive-failure cutoff, must not change task state, and
// must never manufacture a task failure or refund.
func TestUpdateVideoSingleTaskSkipsRoundOnObservationUnavailable(t *testing.T) {
	task := &model.Task{TaskID: "task-obs-1"}
	task.Platform = "62"
	task.Status = model.TaskStatusInProgress
	task.PrivateData.VideoUpstreamProfile = "third_party_feicai_videos"
	task.PrivateData.SouthboundAdapterVersion = "62:third_party_feicai_videos:v2"

	channel := &model.Channel{Type: 62}
	channel.Status = 1

	adaptor := observationUnavailableAdaptor{}
	tasks := map[string]*model.Task{task.TaskID: task}

	for round := 0; round < 5; round++ {
		require.NoError(t, updateVideoSingleTask(context.Background(), adaptor, channel, task.TaskID, tasks))
	}

	assert.Equal(t, model.TaskStatus(model.TaskStatusInProgress), task.Status)
	assert.Equal(t, 0, task.PrivateData.PollFailures, "local infrastructure timeouts must not count toward the failure cutoff")
	assert.Empty(t, task.FailReason)
}
