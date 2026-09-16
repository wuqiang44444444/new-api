package controller

import (
	"context"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/system_setting"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Exercise the actual image relay hooks, using only a scripted local provider
// and synthetic credentials. Evidence must not change rejection or refund rules.
func TestNativeImageErrorEvidenceAcrossSyncAndTask(t *testing.T) {
	for _, async := range []bool{false, true} {
		name := "sync"
		if async {
			name = "task"
		}
		t.Run(name, func(t *testing.T) {
			fixture := newNativeDiagFixture(t, 400, `{"error":{"code":"invalid_argument","message":"synthetic provider detail","api_key":"synthetic-private-key"}}`, nil)
			require.NoError(t, fixture.db.AutoMigrate(&model.TaskRequestEvidence{}, &model.TaskRequestEvidenceEvent{}))
			old := system_setting.GetTaskRequestEvidenceConfig()
			config := old
			config.Enabled = true
			config.StorageDir = t.TempDir()
			config.EncryptionKeyHex = "000102030405060708090a0b0c0d0e0f101112131415161718191a1b1c1d1e1f"
			system_setting.SetTaskRequestEvidenceConfig(config)
			require.NoError(t, service.InitTaskRequestEvidenceStore(config))
			t.Cleanup(func() {
				require.Eventually(t, func() bool { return service.ImageErrorEvidenceHealth()["active"] == 0 }, 3*time.Second, time.Millisecond)
				system_setting.SetTaskRequestEvidenceConfig(old)
				require.NoError(t, service.InitTaskRequestEvidenceStore(old))
			})
			response := submitNativeImageCreate(t, fixture.engine, "/v1/images/generations", `{"model":"gpt-image-2","prompt":"synthetic drawing"}`, async)
			if async {
				require.Equal(t, 202, response.Code)
				var accepted struct {
					ID string `json:"id"`
				}
				require.NoError(t, common.Unmarshal(response.Body.Bytes(), &accepted))
				service.RunImageTaskWorkerOnce(context.Background())
				var task model.Task
				require.NoError(t, fixture.db.First(&task, "task_id = ?", accepted.ID).Error)
				assert.EqualValues(t, model.TaskStatusFailure, task.Status)
				assert.Equal(t, model.TaskBillingStateSettled, task.BillingState)
				assert.Zero(t, task.Quota)
			} else {
				assert.Equal(t, 400, response.Code)
				assert.Contains(t, response.Body.String(), "synthetic provider detail")
			}
			require.Eventually(t, func() bool { return service.ImageErrorEvidenceHealth()["active"] == 0 }, 3*time.Second, time.Millisecond)
			indices, count := model.QueryTaskRequestEvidence(model.TaskRequestEvidenceQueryParams{Kind: "image_error", Num: 10})
			require.GreaterOrEqual(t, count, int64(1))
			found := false
			for _, index := range indices {
				events, err := model.ListTaskRequestEvidenceEvents(index.Id)
				require.NoError(t, err)
				for _, event := range events {
					if event.Stage != "generation" {
						continue
					}
					found = true
					assert.Equal(t, 400, event.StatusCode)
					assert.True(t, event.Complete)
					payload, err := service.GetTaskRequestEvidenceStore().Get(event.ObjectKey)
					require.NoError(t, err)
					assert.Contains(t, string(payload), "synthetic provider detail")
					assert.NotContains(t, string(payload), "synthetic-private-key")
				}
			}
			assert.True(t, found, "the adapter must expose its consumed response to evidence")
			assert.EqualValues(t, 1, fixture.calls.Load())
			require.NoError(t, fixture.db.First(&fixture.user, fixture.user.Id).Error)
			require.NoError(t, fixture.db.First(&fixture.token, fixture.token.Id).Error)
			assert.Equal(t, 100000, fixture.user.Quota)
			assert.Equal(t, 100000, fixture.token.RemainQuota)
		})
	}
}
