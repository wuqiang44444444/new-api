package service

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNestedPluginEvidenceUsesAuthenticatedSessionAndQueryContext(t *testing.T) {
	setupEvidenceTestEnv(t)
	inner := gin.New()
	inner.Use(APIRequestEvidence(), func(c *gin.Context) {
		c.Set("id", 7)
		c.Set("token_id", 3)
		c.Set("app_id", 3)
		c.Next()
	})
	inner.POST("/suno/submit/:action", func(c *gin.Context) {
		var body map[string]any
		require.NoError(t, common.UnmarshalBodyReusable(c, &body))
		require.NoError(t, BeginTaskRequestEvidence(c, TaskRequestEvidenceKindForTaskRelay(c)))
		task := &model.Task{TaskID: "nested-music", UserId: 7, AppID: 3, Platform: "sunoapi", Action: "MUSIC", CreatedAt: common.GetTimestamp()}
		require.NoError(t, model.DB.Create(task).Error)
		AttachTaskRequestEvidenceTask(c, task)
		c.JSON(200, gin.H{"code": "success"})
		FinishTaskRequestEvidenceClientDelivery(c)
	})
	inner.POST("/suno/fetch", func(c *gin.Context) {
		var body map[string]any
		require.NoError(t, c.ShouldBindJSON(&body))
		c.JSON(200, gin.H{"code": "success"})
	})
	outer := gin.New()
	outer.Use(APIRequestEvidence())
	outer.NoRoute(func(c *gin.Context) {
		inner.ServeHTTP(c.Writer, c.Request)
		c.Abort()
	})
	for _, request := range []struct{ path, body string }{
		{"/suno/submit/MUSIC", `{"prompt":"song"}`},
		{"/suno/fetch", `{"ids":["nested-music","nested-music"]}`},
	} {
		req := httptest.NewRequest(http.MethodPost, request.path, strings.NewReader(request.body))
		req.Header.Set("Content-Type", "application/json")
		response := httptest.NewRecorder()
		outer.ServeHTTP(response, req)
		require.Equal(t, 200, response.Code, response.Body.String())
	}
	indexes, count, err := model.QueryTaskRequestEvidence(model.TaskRequestEvidenceQueryParams{})
	require.NoError(t, err)
	require.EqualValues(t, 1, count)
	assert.Equal(t, 7, indexes[0].UserID)
	assert.Equal(t, 3, indexes[0].AppID)
	assert.Equal(t, "nested-music", indexes[0].TaskID)
	events := evidenceEventRows(t, indexes[0].Id)
	require.Len(t, events, 4)
	assert.Equal(t, model.TaskRequestEvidenceStageNorthReceive, events[0].Stage)
	assert.Equal(t, model.TaskRequestEvidenceStageClientDelivery, events[1].Stage)
	assert.Equal(t, model.TaskRequestEvidenceStageNorthReceive, events[2].Stage)
	assert.Equal(t, model.TaskRequestEvidenceStageClientDelivery, events[3].Stage)
}
