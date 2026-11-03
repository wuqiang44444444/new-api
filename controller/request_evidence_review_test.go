package controller

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	relaychannel "github.com/QuantumNous/new-api/relay/channel"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEvidenceTaskQueriesUseOneAuthenticatedRequestWindow(t *testing.T) {
	for _, operation := range []string{"query", "image", "artifacts"} {
		t.Run(operation, func(t *testing.T) {
			setupEvidenceViewFixture(t)
			task := &model.Task{TaskID: "task_review", UserId: 7, AppID: 3, Platform: "document", Status: model.TaskStatusSuccess}
			if operation == "image" {
				task.ClientProtocol = model.TaskClientProtocolImageOpenAIV1
				task.PrivateData.ImageTask = &model.TaskImageExecutionData{ImageCount: 1}
			}
			require.NoError(t, model.DB.Create(task).Error)
			engine := gin.New()
			engine.Use(func(c *gin.Context) {
				c.Set("id", 7)
				c.Set("token_id", 3)
				c.Set(common.RequestIdKey, "query-review")
				c.Next()
			}, service.APIRequestEvidence())
			path := "/v1/tasks/" + task.TaskID
			if operation == "artifacts" {
				engine.GET("/v1/tasks/:key/artifacts", GetTaskArtifacts)
				path += "/artifacts"
			} else {
				engine.GET("/v1/tasks/:key", GetTask)
			}
			response := httptest.NewRecorder()
			engine.ServeHTTP(response, httptest.NewRequest("GET", path, nil))
			require.Equal(t, 200, response.Code, response.Body.String())
			flushControllerEvidence(t)
			indexes, total, err := model.QueryTaskRequestEvidence(model.TaskRequestEvidenceQueryParams{TaskID: task.TaskID})
			require.NoError(t, err)
			require.EqualValues(t, 1, total)
			assert.Equal(t, "query-review", indexes[0].RequestID)
			events, err := model.ListTaskRequestEvidenceEvents(indexes[0].Id)
			require.NoError(t, err)
			require.Len(t, events, 2, "one north event and one client delivery")
			deliveries := 0
			for _, event := range events {
				if event.Stage != model.TaskRequestEvidenceStageClientDelivery {
					continue
				}
				deliveries++
				content, status := service.ReadEvidenceEventContent(event, false)
				require.Equal(t, "available", status)
				assert.Equal(t, response.Body.Bytes(), content.Body)
			}
			assert.Equal(t, 1, deliveries)
		})
	}
}

func TestEvidenceProxiedTextArtifactDoesNotSaveFileOrInfrastructureExchange(t *testing.T) {
	setupEvidenceViewFixture(t)
	allowPrivateTaskMediaTest(t)
	previousCache := common.MemoryCacheEnabled
	common.MemoryCacheEnabled = false
	t.Cleanup(func() { common.MemoryCacheEnabled = previousCache })
	proxyCalls := 0
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		proxyCalls++
		assert.Equal(t, "127.0.0.1:1", r.URL.Host)
		w.Header().Set("Content-Type", "text/plain")
		_, _ = w.Write([]byte("file delivered once"))
	}))
	defer proxy.Close()
	var channel model.Channel
	require.NoError(t, model.DB.First(&channel, 1).Error)
	channel.SetSetting(dto.ChannelSettings{Proxy: proxy.URL})
	require.NoError(t, model.DB.Save(&channel).Error)
	task := &model.Task{TaskID: "task_proxy_review", UserId: 7, AppID: 3, ChannelId: 1, Status: model.TaskStatusSuccess}
	require.NoError(t, model.DB.Create(task).Error)
	engine := gin.New()
	engine.Use(func(c *gin.Context) {
		c.Set("id", 7)
		c.Set("token_id", 3)
		c.Set(common.RequestIdKey, "proxy-review")
		c.Next()
	}, service.APIRequestEvidence())
	engine.GET("/v1/tasks/:key/artifacts/:artifact_key/content", func(c *gin.Context) {
		require.NoError(t, proxyTaskMedia(c, task, &relaychannel.TaskContentRequest{URL: "http://127.0.0.1:1/object.txt", Method: "GET", Credentialless: true}))
	})
	response := httptest.NewRecorder()
	engine.ServeHTTP(response, httptest.NewRequest("GET", "/v1/tasks/"+task.TaskID+"/artifacts/file/content", nil))
	assert.Equal(t, "file delivered once", response.Body.String())
	assert.Equal(t, 1, proxyCalls)
	flushControllerEvidence(t)
	indexes, total, err := model.QueryTaskRequestEvidence(model.TaskRequestEvidenceQueryParams{TaskID: task.TaskID})
	require.NoError(t, err)
	require.EqualValues(t, 1, total)
	events, err := model.ListTaskRequestEvidenceEvents(indexes[0].Id)
	require.NoError(t, err)
	require.Len(t, events, 2, "no extra download index or southbound infrastructure event")
	for _, event := range events {
		assert.Contains(t, []string{model.TaskRequestEvidenceStageNorthReceive, model.TaskRequestEvidenceStageClientDelivery}, event.Stage)
		_, status := service.ReadEvidenceEventContent(event, false)
		assert.Equal(t, "missing", status)
	}
}
