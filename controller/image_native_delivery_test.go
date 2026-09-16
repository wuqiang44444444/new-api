package controller

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/system_setting"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Generation succeeds but result retrieval fails: preserve the generation
// evidence and holds across recovery, never classify a download 403 as a
// rejected generation, and never regenerate or expose the signed URL.
func TestNativeImageDeliveryFailurePreservesEvidenceAndHolds(t *testing.T) {
	for _, tc := range []struct {
		name, image, code string
		downloadStatus    int
	}{
		{name: "decode", image: `{"b64_json":"invalid-secret-base64"}`, code: "result_decode_failed"},
		{name: "missing", image: `{}`, code: "result_missing"},
		{name: "download", code: "result_download_http_error", downloadStatus: 403},
	} {
		t.Run(tc.name, func(t *testing.T) {
			originalFetch := *system_setting.GetFetchSetting()
			system_setting.GetFetchSetting().EnableSSRFProtection = false // local fixture only
			t.Cleanup(func() { *system_setting.GetFetchSetting() = originalFetch })
			var downloads int
			download := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				downloads++
				assert.Empty(t, r.Header.Get("Authorization"), "provider credentials must not accompany result downloads")
				w.WriteHeader(http.StatusForbidden)
			}))
			t.Cleanup(download.Close)
			image := tc.image
			if image == "" {
				encoded, err := common.Marshal(map[string]string{"url": download.URL + "/result?signature=private-result-secret"})
				require.NoError(t, err)
				image = string(encoded)
			}
			fixture := newNativeDiagFixture(t, 200, `{"data":[`+image+`],"usage":{"input_tokens":24,"output_tokens":196}}`, map[string]string{"X-Request-Id": "generation-request-1"})
			diag := captureNativeImageDiag(t)
			response := submitNativeImageCreate(t, fixture.engine, "/v1/images/generations", `{"model":"gpt-image-2","prompt":"draw"}`, true)
			require.Equal(t, 202, response.Code)
			var accepted struct {
				ID string `json:"id"`
			}
			require.NoError(t, common.Unmarshal(response.Body.Bytes(), &accepted))
			service.RunImageTaskWorkerOnce(context.Background())
			for _, recovery := range []bool{false, true} {
				if recovery {
					service.RunImageTaskWorkerOnce(context.Background())
				}
				var task model.Task
				require.NoError(t, fixture.db.First(&task, "task_id = ?", accepted.ID).Error)
				assert.Equal(t, model.TaskStatusReconciliationRequired, task.Status)
				assert.Equal(t, model.TaskBillingStatePending, task.BillingState)
				assert.Equal(t, tc.code, task.PrivateData.ImageTask.FailureCode)
				assert.Equal(t, 200, task.PrivateData.ImageTask.FailureStatus)
				assert.Equal(t, "generation-request-1", task.PrivateData.ImageTask.ProviderRequestID)
				assert.True(t, task.PrivateData.ImageTask.GenerationComplete)
				assert.Empty(t, task.PrivateData.ImageTask.Artifacts)
				require.NoError(t, fixture.db.First(&fixture.user, fixture.user.Id).Error)
				require.NoError(t, fixture.db.First(&fixture.token, fixture.token.Id).Error)
				assert.Equal(t, 80000, fixture.user.Quota)
				assert.Equal(t, 80000, fixture.token.RemainQuota)
				assert.EqualValues(t, 1, fixture.calls.Load(), "recovery must never generate again")
			}
			line := waitNativeImageDiagLine(t, diag, accepted.ID)
			assert.Contains(t, line, "stage="+tc.code)
			assert.Contains(t, line, "upstream_status=200")
			assert.Contains(t, line, "provider_request_id=generation-request-1")
			assert.Contains(t, line, "trusted=false")
			if tc.downloadStatus > 0 {
				assert.Contains(t, line, "download_status=403")
				assert.Equal(t, 1, downloads)
			}
			assert.NotContains(t, diag.String(), "private-result-secret")
			assert.NotContains(t, diag.String(), "invalid-secret-base64")
		})
	}
}
