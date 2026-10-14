package service

import (
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMediaScopeExcludesMJManagementAndLyricsVariants(t *testing.T) {
	env := setupEvidenceTestEnv(t)
	engine := gin.New()
	engine.Use(APIRequestEvidence())
	for _, route := range []struct{ method, path string }{
		{"GET", "/api/mj/"}, {"GET", "/api/mj/self"},
		{"GET", "/nested/path/mj/self"}, {"POST", "/mj/unknown"},
		{"POST", "/suno/submit/lyrics"}, {"POST", "/suno/submit/LYRICS"}, {"POST", "/suno/submit/Lyrics"},
	} {
		engine.Handle(route.method, route.path, func(c *gin.Context) { c.Set("id", 7); c.JSON(200, gin.H{"data": "text"}) })
		response := httptest.NewRecorder()
		engine.ServeHTTP(response, httptest.NewRequest(route.method, route.path, strings.NewReader(`{"prompt":"text"}`)))
		assert.Equal(t, 200, response.Code)
	}
	for _, table := range []any{&model.TaskRequestEvidence{}, &model.TaskRequestEvidenceEvent{}, &model.TaskRequestEvidenceAccessLog{}} {
		var count int64
		require.NoError(t, model.DB.Model(table).Count(&count).Error)
		assert.Zero(t, count)
	}
	entries, err := os.ReadDir(env.storeDir)
	require.NoError(t, err)
	assert.Empty(t, entries)
}

func TestMidjourneyQueriesReuseCreationAnchorAndNeverReopen(t *testing.T) {
	setupEvidenceTestEnv(t)
	require.NoError(t, model.DB.AutoMigrate(&model.Midjourney{}))
	engine := gin.New()
	engine.Use(APIRequestEvidence(), func(c *gin.Context) {
		c.Set("id", 7)
		c.Set("token_id", 3)
		if c.GetHeader("Test-App") == "other" {
			c.Set("token_id", 4)
		}
		c.Next()
	})
	engine.POST("/mj/submit/imagine", func(c *gin.Context) {
		var request map[string]any
		require.NoError(t, c.ShouldBindJSON(&request))
		// Native TokenId is a mutable billing marker and can legitimately be zero.
		require.NoError(t, model.DB.Create(&model.Midjourney{UserId: 7, MjId: "mj-fixed", TokenId: 0, SubmitTime: common.GetTimestamp() * 1000}).Error)
		c.Set(common.RequestIdKey, "mj-create")
		c.JSON(200, gin.H{"code": 1, "result": "mj-fixed"})
	})
	for _, path := range []string{"/mj/task/:id/fetch", "/:mode/mj/task/:id/image-seed"} {
		engine.GET(path, func(c *gin.Context) { c.JSON(200, gin.H{"id": c.Param("id"), "status": "SUCCESS"}) })
	}
	engine.POST("/mj/task/list-by-condition", func(c *gin.Context) {
		var request map[string]any
		require.NoError(t, c.ShouldBindJSON(&request))
		c.JSON(200, []gin.H{{"id": "mj-fixed"}})
	})
	serve := func(method, path, body string, otherApp bool) {
		req := httptest.NewRequest(method, path, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		if otherApp {
			req.Header.Set("Test-App", "other")
		}
		response := httptest.NewRecorder()
		engine.ServeHTTP(response, req)
		require.Equal(t, 200, response.Code)
	}
	serve("POST", "/mj/submit/imagine", `{"prompt":"image"}`, false)
	indexes, count, err := model.QueryTaskRequestEvidence(model.TaskRequestEvidenceQueryParams{TaskID: "mj-fixed"})
	require.NoError(t, err)
	require.EqualValues(t, 1, count)
	index := indexes[0]
	assert.Equal(t, "midjourney", index.ClientProtocol)
	assert.Len(t, evidenceEventRows(t, index.Id), 2)
	// A provider update must not change the evidence lifetime.
	require.NoError(t, model.DB.Model(&model.Midjourney{}).Where("mj_id = ?", "mj-fixed").Update("submit_time", (common.GetTimestamp()+86400)*1000).Error)
	serve("GET", "/mj/task/mj-fixed/fetch", "", false)
	serve("GET", "/fast/mj/task/mj-fixed/image-seed", "", false)
	serve("POST", "/mj/task/list-by-condition", `{"ids":["mj-fixed","mj-fixed"]}`, false)
	assert.Len(t, evidenceEventRows(t, index.Id), 8)
	serve("GET", "/mj/task/mj-fixed/fetch", "", true)
	assert.Len(t, evidenceEventRows(t, index.Id), 8)
	indexes, count, err = model.QueryTaskRequestEvidence(model.TaskRequestEvidenceQueryParams{TaskID: "mj-fixed"})
	require.NoError(t, err)
	require.EqualValues(t, 1, count)
	assert.Equal(t, index.CreatedAt, indexes[0].CreatedAt)
	assert.Equal(t, index.RetainUntil, indexes[0].RetainUntil)
	require.NoError(t, model.RequestTaskRequestEvidenceCleanup(index.Id, common.GetTimestamp()))
	serve("GET", "/mj/task/mj-fixed/fetch", "", false)
	assert.Len(t, evidenceEventRows(t, index.Id), 8)
	require.NoError(t, model.DB.Where("evidence_id = ?", index.Id).Delete(&model.TaskRequestEvidenceEvent{}).Error)
	require.NoError(t, model.DB.Delete(index).Error)
	serve("GET", "/mj/task/mj-fixed/fetch", "", false)
	indexes, count, err = model.QueryTaskRequestEvidence(model.TaskRequestEvidenceQueryParams{TaskID: "mj-fixed"})
	require.NoError(t, err)
	assert.Zero(t, count)
	assert.Empty(t, indexes)
}

func TestMidjourneyExpiredAndUnanchoredTasksCannotCreateQueryEvidence(t *testing.T) {
	setupEvidenceTestEnv(t)
	require.NoError(t, model.DB.AutoMigrate(&model.Midjourney{}))
	now := common.GetTimestamp()
	for _, id := range []string{"expired", "legacy"} {
		require.NoError(t, model.DB.Create(&model.Midjourney{UserId: 7, MjId: id, SubmitTime: now * 1000}).Error)
	}
	index := &model.TaskRequestEvidence{TaskID: "expired", UserID: 7, AppID: 3, ClientProtocol: "midjourney", Kind: "image_request", CreatedAt: now - 8*86400, RetainUntil: now + 86400}
	require.NoError(t, model.CreateTaskRequestEvidence(index))
	engine := gin.New()
	engine.Use(APIRequestEvidence())
	engine.GET("/mj/task/:id/fetch", func(c *gin.Context) { c.Set("id", 7); c.Set("token_id", 3); c.JSON(200, gin.H{"status": "SUCCESS"}) })
	for _, id := range []string{"expired", "legacy", "missing"} {
		response := httptest.NewRecorder()
		engine.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/mj/task/"+id+"/fetch", nil))
		assert.Equal(t, 200, response.Code)
	}
	var events, indexes int64
	require.NoError(t, model.DB.Model(&model.TaskRequestEvidenceEvent{}).Count(&events).Error)
	require.NoError(t, model.DB.Model(&model.TaskRequestEvidence{}).Count(&indexes).Error)
	assert.Zero(t, events)
	assert.EqualValues(t, 1, indexes)
}

func TestSunoQueriesExcludeLyricsAndReuseMusicWindow(t *testing.T) {
	setupEvidenceTestEnv(t)
	now := common.GetTimestamp()
	for _, task := range []*model.Task{
		{TaskID: "music", UserId: 7, AppID: 3, Platform: "sunoapi", Action: "MUSIC", CreatedAt: now - 3600},
		{TaskID: "lyrics", UserId: 7, AppID: 3, Platform: "sunoapi", Action: "LYRICS", CreatedAt: now},
		{TaskID: "expired-music", UserId: 7, AppID: 3, Platform: "sunoapi", Action: "MUSIC", CreatedAt: now - 8*86400},
	} {
		require.NoError(t, model.DB.Create(task).Error)
	}
	engine := gin.New()
	engine.Use(APIRequestEvidence(), func(c *gin.Context) { c.Set("id", 7); c.Set("token_id", 3); c.Next() })
	engine.POST("/suno/fetch", func(c *gin.Context) {
		var request map[string]any
		require.NoError(t, c.ShouldBindJSON(&request))
		c.JSON(200, gin.H{"status": "running"})
	})
	engine.GET("/suno/fetch/:task_id", func(c *gin.Context) { c.JSON(200, gin.H{"status": "running"}) })
	for _, tc := range []struct{ method, path, body string }{
		{"GET", "/suno/fetch/lyrics", ""},
		{"POST", "/suno/fetch", `{"ids":["lyrics","expired-music"]}`},
		{"GET", "/suno/fetch/music", ""},
		{"POST", "/suno/fetch", `{"ids":["music","music"]}`},
	} {
		req := httptest.NewRequest(tc.method, tc.path, strings.NewReader(tc.body))
		req.Header.Set("Content-Type", "application/json")
		response := httptest.NewRecorder()
		engine.ServeHTTP(response, req)
		require.Equal(t, 200, response.Code)
	}
	indexes, count, err := model.QueryTaskRequestEvidence(model.TaskRequestEvidenceQueryParams{})
	require.NoError(t, err)
	require.EqualValues(t, 1, count)
	assert.Equal(t, "music", indexes[0].TaskID)
	assert.Equal(t, now-3600+7*86400, indexes[0].RetainUntil)
	assert.Len(t, evidenceEventRows(t, indexes[0].Id), 4)
}

func TestSunoMusicCasingPreservesAudioEvidenceKind(t *testing.T) {
	setupEvidenceTestEnv(t)
	engine := gin.New()
	engine.Use(APIRequestEvidence())
	engine.POST("/suno/submit/:action", func(c *gin.Context) {
		c.Set("id", 7)
		c.Set("token_id", 3)
		c.Set("platform", "sunoapi")
		var request map[string]any
		require.NoError(t, common.UnmarshalBodyReusable(c, &request))
		require.NoError(t, BeginTaskRequestEvidence(c, TaskRequestEvidenceKindForTaskRelay(c)))
		c.JSON(400, gin.H{"error": "invalid_request"})
	})
	for _, action := range []string{"music", "MUSIC"} {
		req := httptest.NewRequest("POST", "/suno/submit/"+action, strings.NewReader(`{"prompt":"song"}`))
		req.Header.Set("Content-Type", "application/json")
		response := httptest.NewRecorder()
		engine.ServeHTTP(response, req)
		require.Equal(t, 400, response.Code)
	}
	indexes, count, err := model.QueryTaskRequestEvidence(model.TaskRequestEvidenceQueryParams{})
	require.NoError(t, err)
	require.EqualValues(t, 2, count)
	for _, index := range indexes {
		assert.Equal(t, model.TaskRequestEvidenceKindAudioTask, index.Kind)
		assert.Len(t, evidenceEventRows(t, index.Id), 2)
	}
}
