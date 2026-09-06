package controller

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/system_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestImageQueryStorageDeliveryAndOwnership(t *testing.T) {
	t.Setenv("CRYPTO_SECRET", "image-query-test-secret")
	setupGenericTaskTest(t)
	previousSecret := common.CryptoSecret
	common.CryptoSecret = "image-query-test-secret"
	t.Cleanup(func() { common.CryptoSecret = previousSecret; model.NotifyObjectStorageSettingUpdate("") })
	cipher, err := common.EncryptObjectStorageCredential("test-key")
	require.NoError(t, err)
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		assert.True(t, strings.HasPrefix(r.URL.Path, "/images/prod/images/tasks/"))
		switch {
		case strings.HasSuffix(r.URL.Path, "result-1"):
			w.WriteHeader(http.StatusNotFound)
		case strings.HasSuffix(r.URL.Path, "result-2"):
			w.WriteHeader(http.StatusForbidden)
		default:
			w.Header().Set("Content-Length", "5")
			if r.Method == http.MethodGet {
				_, _ = w.Write([]byte("image"))
			}
		}
	}))
	t.Cleanup(server.Close)
	config := system_setting.ObjectStorageConfig{Backend: "s3", Endpoint: server.URL, Bucket: "images", Prefix: "prod", AccountName: "account", Region: "us-east-1", CredentialCiphertext: cipher, Revision: "image-query"}
	encoded, err := common.Marshal(config)
	require.NoError(t, err)
	model.NotifyObjectStorageSettingUpdate(string(encoded))
	task := &model.Task{TaskID: "task_images", UserId: 7, AppID: 11, Status: model.TaskStatusSuccess,
		ClientProtocol: model.TaskClientProtocolImageOpenAIV1, SubmitTime: time.Now().Add(-time.Hour).Unix(), FinishTime: time.Now().Add(-30 * time.Minute).Unix()}
	task.PrivateData.ImageTask = &model.TaskImageExecutionData{ImageCount: 3}
	for i := 0; i < 3; i++ {
		task.PrivateData.ImageTask.Artifacts = append(task.PrivateData.ImageTask.Artifacts, model.TaskImageArtifact{ObjectKey: "images/tasks/task_images/result-" + strconv.Itoa(i), MimeType: "image/png", Size: 5})
	}
	require.NoError(t, model.DB.Create(task).Error)
	query := func(userID, appID int) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest(http.MethodGet, "/v1/tasks/task_images", nil)
		c.Set("id", userID)
		c.Set("token_id", appID)
		require.True(t, projectImageTaskQuery(c, task.TaskID))
		return w
	}
	for _, owner := range [][2]int{{7, 12}, {8, 11}} {
		w := query(owner[0], owner[1])
		assert.Equal(t, http.StatusNotFound, w.Code)
	}
	assert.Zero(t, requests.Load(), "unauthorized queries never probe or sign objects")
	for _, format := range []string{"url", "b64_json"} {
		t.Run(format, func(t *testing.T) {
			task.PrivateData.ImageTask.ResponseFormat = format
			require.NoError(t, model.DB.Save(task).Error)
			before := time.Now().Unix()
			w := query(7, 11)
			require.Equal(t, http.StatusOK, w.Code)
			var response struct {
				Status string                   `json:"status"`
				Data   []imageTaskQueryDataItem `json:"data"`
			}
			require.NoError(t, common.Unmarshal(w.Body.Bytes(), &response))
			assert.Equal(t, "succeeded", response.Status, "delivery availability does not rewrite generation history")
			require.Len(t, response.Data, 3)
			assert.Equal(t, "available", response.Data[0].Status)
			assert.Equal(t, "deleted", response.Data[1].Status)
			assert.Equal(t, "unavailable", response.Data[2].Status)
			assert.Empty(t, response.Data[1].URL)
			assert.Empty(t, response.Data[2].URL)
			if format == "url" {
				parsed, err := url.Parse(response.Data[0].URL)
				require.NoError(t, err)
				assert.Equal(t, "300", parsed.Query().Get("X-Amz-Expires"))
				assert.Equal(t, "/images/prod/images/tasks/task_images/result-0", parsed.Path)
				assert.GreaterOrEqual(t, response.Data[0].URLExpiresAt, before+300, "old tasks receive a fresh expiry at query time")
				assert.LessOrEqual(t, response.Data[0].URLExpiresAt, time.Now().Unix()+300)
				assert.Empty(t, response.Data[0].B64JSON)
			} else {
				assert.Equal(t, "aW1hZ2U=", response.Data[0].B64JSON)
				assert.Empty(t, response.Data[0].URL)
			}
		})
	}
}
