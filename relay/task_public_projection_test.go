package relay

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNativeTaskDTOStillContainsItsProtocolData(t *testing.T) {
	task := &model.Task{
		Platform: "suno", Status: model.TaskStatusSuccess, ChannelId: 12,
		Properties: model.Properties{OriginModelName: "native", UpstreamModelName: "native-provider", Input: "lyrics"},
		Data:       []byte(`[{"audio_url":"https://audio.example/song.mp3","title":"Song"}]`),
	}
	result := TaskModel2Dto(task)
	assert.Equal(t, task.Properties, result.Properties)
	assert.Equal(t, task.ChannelId, result.ChannelId)
	assert.JSONEq(t, string(task.Data), string(result.Data))
	encoded, err := common.Marshal(result)
	require.NoError(t, err)
	assert.Contains(t, string(encoded), "audio_url")
}
