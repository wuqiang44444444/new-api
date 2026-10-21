package middleware

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/system_setting"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

type limitedTaskClient struct {
	*httptest.ResponseRecorder
	limit int
	err   error
}

func (w limitedTaskClient) Write(body []byte) (int, error) {
	n, _ := w.ResponseRecorder.Write(body[:min(w.limit, len(body))])
	return n, w.err
}

func TestTaskCreateEvidenceRecordsFinalClientWrites(t *testing.T) {
	const requestBody = `{"model":"test-video","content":[{"type":"text","text":"fixture"}],"duration":4}`
	const publicTaskID = "task_0123456789abcdefghijklmnopqrstuv"
	const finalBody = `{"id":"` + publicTaskID + `"}`
	const internalBody = `{"id":"internal-id","task_id":"internal-task","model":"test-video","status":"queued","created_at":1}`
	const rawError = `{ "error": {"code":"fixture_error"}, "id":9007199254740993, "n":0, "n":1 }`
	for _, scenario := range []struct {
		name          string
		request       string
		wantBody      string
		wantStatus    int
		writeLimit    int
		writeErr      error
		wantWriteErr  string
		responseLimit int64
		disabled      bool
		storeFails    bool
	}{
		{name: "success", wantBody: finalBody, wantStatus: http.StatusOK},
		{name: "contract error", wantBody: `{"error":{"code":"create_outcome_unknown"}}`, wantStatus: http.StatusServiceUnavailable},
		{name: "unprojected error", wantBody: rawError, wantStatus: http.StatusBadGateway},
		{name: "invalid JSON", request: `{"model":`, wantStatus: http.StatusBadRequest},
		{name: "bounded capture", wantBody: finalBody, wantStatus: http.StatusOK, responseLimit: 8},
		{name: "disconnected", wantStatus: http.StatusOK, writeLimit: 0, writeErr: errors.New("fixture client disconnected"), wantWriteErr: "fixture client disconnected"},
		{name: "partial write error", wantBody: finalBody[:8], wantStatus: http.StatusOK, writeLimit: 8, writeErr: errors.New("fixture client disconnected"), wantWriteErr: "fixture client disconnected"},
		{name: "short write", wantBody: finalBody[:8], wantStatus: http.StatusOK, writeLimit: 8, wantWriteErr: io.ErrShortWrite.Error()},
		{name: "disabled", wantBody: finalBody, wantStatus: http.StatusOK, disabled: true},
		{name: "storage unavailable", wantBody: finalBody, wantStatus: http.StatusOK, storeFails: true},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			previousDB, previousConfig := model.DB, system_setting.GetTaskRequestEvidenceConfig()
			previousMain, previousLog := common.MainDatabaseType(), common.LogDatabaseType()
			db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "evidence.db")), &gorm.Config{})
			require.NoError(t, err)
			sqlDB, err := db.DB()
			require.NoError(t, err)
			sqlDB.SetMaxOpenConns(1)
			config := modelArkEvidenceErrorConfig(t, 1<<20, t.TempDir())
			config.Enabled = !scenario.disabled
			if scenario.responseLimit > 0 {
				config.MaxResponseBytes = scenario.responseLimit
			}
			if scenario.storeFails {
				config.StorageDir = filepath.Join(t.TempDir(), "occupied")
				require.NoError(t, os.WriteFile(config.StorageDir, []byte("occupied"), 0600))
			}
			model.DB = db
			common.SetDatabaseTypes(common.DatabaseTypeSQLite, common.DatabaseTypeSQLite)
			system_setting.SetTaskRequestEvidenceConfig(config)
			require.NoError(t, service.InitTaskRequestEvidenceStore(config))
			t.Cleanup(func() {
				model.DB = previousDB
				common.SetDatabaseTypes(previousMain, previousLog)
				system_setting.SetTaskRequestEvidenceConfig(previousConfig)
				require.NoError(t, service.InitTaskRequestEvidenceStore(previousConfig))
				require.NoError(t, sqlDB.Close())
			})
			require.NoError(t, db.AutoMigrate(&model.Task{}, &model.TaskRequestEvidence{}, &model.TaskRequestEvidenceEvent{}))
			task := model.Task{TaskID: publicTaskID, UserId: 7, AppID: 3, ClientProtocol: model.TaskClientProtocolModelArkV3, Quota: 25, Status: model.TaskStatusSubmitted}
			require.NoError(t, db.Create(&task).Error)
			calls := 0
			engine := gin.New()
			engine.Use(service.APIRequestEvidence())
			engine.POST("/api/v3/contents/generations/tasks", func(c *gin.Context) {
				c.Set("id", 7)
				c.Set("app_id", 3)
				c.Set(common.RequestIdKey, "final-delivery-fixture")
				c.Next()
			}, TaskCreateResponseContract(), ModelArkVideoCreateConvert(), func(c *gin.Context) {
				calls++
				defer service.TrackVideoCreateDelivery(c, &task)()
				defer service.FinishTaskRequestEvidenceClientDelivery(c)
				switch scenario.name {
				case "contract error":
					c.Set(TaskCreateContractErrorKey, TaskCreateContractError{Status: http.StatusServiceUnavailable, Body: gin.H{"error": gin.H{"code": "create_outcome_unknown"}}})
					c.Data(http.StatusInternalServerError, "application/json", []byte(internalBody))
				case "unprojected error":
					c.Data(http.StatusBadGateway, "application/json", []byte(rawError))
				default:
					c.Set(TaskCreateContractResponseKey, gin.H{"id": task.TaskID})
					c.Header("Content-Length", strconv.Itoa(len(internalBody)))
					c.Data(http.StatusOK, "application/json", []byte(internalBody))
				}
			})
			body := requestBody
			if scenario.request != "" {
				body = scenario.request
			}
			request := httptest.NewRequest(http.MethodPost, "/api/v3/contents/generations/tasks", strings.NewReader(body))
			request.Header.Set("Content-Type", "application/json")
			response := httptest.NewRecorder()
			var client http.ResponseWriter = response
			if scenario.wantWriteErr != "" {
				client = limitedTaskClient{ResponseRecorder: response, limit: scenario.writeLimit, err: scenario.writeErr}
			}
			engine.ServeHTTP(client, request)
			require.Equal(t, scenario.wantStatus, response.Code)
			if scenario.name == "invalid JSON" {
				assert.Zero(t, calls)
				assert.Contains(t, response.Body.String(), `"code":"invalid_request"`)
			} else {
				assert.Equal(t, 1, calls)
				assert.Equal(t, scenario.wantBody, response.Body.String())
			}
			var saved model.Task
			require.NoError(t, db.First(&saved, task.ID).Error)
			assert.Equal(t, task.Status, saved.Status)
			assert.Equal(t, task.Quota, saved.Quota)
			if calls > 0 {
				wantDelivery := "server_written"
				if scenario.wantWriteErr != "" {
					wantDelivery = "write_failed"
				}
				assert.Equal(t, wantDelivery, saved.VideoDeliveryState)
			}
			var indexes []model.TaskRequestEvidence
			require.NoError(t, db.Find(&indexes).Error)
			if scenario.disabled {
				assert.Empty(t, indexes)
				return
			}
			require.Len(t, indexes, 1)
			events, err := model.ListTaskRequestEvidenceEvents(indexes[0].Id)
			require.NoError(t, err)
			var requests, deliveries []*model.TaskRequestEvidenceEvent
			for _, event := range events {
				if event.Stage == model.TaskRequestEvidenceStageNorthReceive {
					requests = append(requests, event)
				}
				if event.Stage == model.TaskRequestEvidenceStageClientDelivery {
					deliveries = append(deliveries, event)
				}
			}
			require.Len(t, deliveries, 1, "inner handlers and the outer observer must not duplicate final delivery")
			content, status := service.ReadEvidenceEventContent(deliveries[0], false)
			if scenario.storeFails {
				assert.NotEqual(t, "available", status)
				return
			}
			require.Equal(t, "available", status)
			require.Len(t, requests, 1)
			original, originalStatus := service.ReadEvidenceEventContent(requests[0], false)
			require.Equal(t, "available", originalStatus)
			assert.Equal(t, body, string(original.Body), "keep the request before ModelArk conversion")
			wantCaptured := response.Body.Bytes()
			if int64(len(wantCaptured)) > config.MaxResponseBytes {
				wantCaptured = wantCaptured[:config.MaxResponseBytes]
			}
			assert.Equal(t, string(wantCaptured), string(content.Body), "preserve the exact bytes accepted by the client writer")
			require.NotNil(t, content.Meta)
			assert.Equal(t, response.Code, content.Meta.StatusCode)
			assert.Equal(t, scenario.wantWriteErr, content.Meta.Error)
		})
	}
}
