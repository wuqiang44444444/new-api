package relay

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/service"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestImageAcceptedTaskPollingErrorRemainsUnknown(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	oldDB := model.DB
	model.DB = db
	sqlDB, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() { model.DB = oldDB; _ = sqlDB.Close() })
	require.NoError(t, db.AutoMigrate(&model.Task{}, &model.ImageTaskSlot{}))
	for _, initial := range []bool{false, true} {
		t.Run(map[bool]string{false: "create rejection", true: "poll rejection"}[initial], func(t *testing.T) {
			posts, gets := 0, 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				if r.Method == http.MethodPost {
					posts++
					if initial {
						_, _ = io.WriteString(w, `{"code":0,"data":{"status":"queued","taskId":"accepted-id"}}`)
						return
					}
				} else {
					gets++
				}
				_, _ = io.WriteString(w, `{"code":10002}`)
			}))
			defer server.Close()
			task := &model.Task{TaskID: model.GenerateTaskID(), Status: model.TaskStatusInProgress, ClientProtocol: model.TaskClientProtocolImageOpenAIV1, PrivateData: model.TaskPrivateData{ImageTask: &model.TaskImageExecutionData{FundsHeld: true}}}
			require.NoError(t, db.Create(task).Error)
			info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{ApiKey: "test-key", ChannelBaseUrl: server.URL, UpstreamModelName: "nano-banana-2", ChannelOtherSettings: dto.ChannelOtherSettings{ImageUpstreamProtocol: dto.ImageUpstreamProtocolFunCloudAIGCV2}}}
			task.PrivateData.ImageTask.HeadersCiphertext, err = freezeImageTaskHeaders(task.TaskID, nil, info)
			require.NoError(t, err)
			result := executeImageRelayTask(context.Background(), task, info, []byte(`{}`))
			assert.Equal(t, 1, posts)
			if initial {
				assert.Equal(t, 1, gets)
				assert.Equal(t, service.ImageTaskOutcomeUnknown, result.Outcome)
				assert.Equal(t, "accepted-id", result.ProviderTaskID)
				var stored model.Task
				require.NoError(t, db.First(&stored, task.ID).Error)
				assert.Equal(t, "accepted-id", stored.PrivateData.ImageTask.ProviderTaskID)
			} else {
				assert.Zero(t, gets)
				assert.Equal(t, service.ImageTaskOutcomeFailure, result.Outcome)
			}
		})
	}
}
