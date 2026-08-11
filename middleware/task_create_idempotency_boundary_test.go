package middleware

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestTaskCreateIdempotencyReleasesOnlyPreUpstreamServerFailures(t *testing.T) {
	previousDB := model.DB
	previousType := common.MainDatabaseType()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.TaskCreateIdempotency{}))
	model.DB = db
	common.SetMainDatabaseType(common.DatabaseTypeSQLite)
	t.Cleanup(func() {
		model.DB = previousDB
		common.SetMainDatabaseType(previousType)
	})

	tests := []struct {
		name       string
		key        string
		started    bool
		wantCount  int64
		wantStatus string
	}{
		{name: "local failure releases claim", key: "local-failure", wantCount: 0},
		{
			name: "ambiguous upstream failure remains unknown", key: "upstream-failure",
			started: true, wantCount: 1, wantStatus: model.TaskCreateIdempotencyUnknown,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			engine := gin.New()
			engine.Use(func(c *gin.Context) {
				c.Set("id", 7)
				common.SetContextKey(c, constant.ContextKeyTaskClientProtocol, model.TaskClientProtocolModelArkV3)
				c.Next()
			})
			engine.Use(TaskCreateIdempotency())
			engine.POST("/tasks", func(c *gin.Context) {
				if test.started {
					common.SetContextKey(c, constant.ContextKeyTaskUpstreamStarted, true)
				}
				c.Status(http.StatusInternalServerError)
			})
			request := httptest.NewRequest(http.MethodPost, "/tasks", strings.NewReader(`{"model":"seedance-model"}`))
			request.Header.Set("Content-Type", "application/json")
			request.Header.Set("Idempotency-Key", test.key)
			response := httptest.NewRecorder()

			engine.ServeHTTP(response, request)

			assert.Equal(t, http.StatusInternalServerError, response.Code)
			var claims []model.TaskCreateIdempotency
			require.NoError(t, db.Find(&claims).Error)
			assert.Len(t, claims, int(test.wantCount))
			if test.wantCount == 1 {
				assert.Equal(t, test.wantStatus, claims[0].Status)
				require.NoError(t, db.Delete(&claims[0]).Error)
			}
		})
	}
}

func TestTaskCreateIdempotencyAllowsSameRequestAfterManualVerifiedRejection(t *testing.T) {
	previousDB := model.DB
	previousType := common.MainDatabaseType()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(
		&model.TaskCreateIdempotency{},
		&model.TaskCreateAttempt{},
	))
	model.DB = db
	common.SetMainDatabaseType(common.DatabaseTypeSQLite)
	t.Cleanup(func() {
		model.DB = previousDB
		common.SetMainDatabaseType(previousType)
	})

	const requestBody = `{"model":"seedance-model"}`
	const idempotencyKey = "manual-rejection-safe-retry"
	handlerCalls := 0
	firstAttemptID := ""
	secondAttemptID := ""
	engine := gin.New()
	engine.Use(func(c *gin.Context) {
		c.Set("id", 7)
		common.SetContextKey(c, constant.ContextKeyTaskClientProtocol, model.TaskClientProtocolModelArkV3)
		c.Next()
	})
	engine.Use(TaskCreateIdempotency())
	engine.POST("/tasks", func(c *gin.Context) {
		handlerCalls++
		publicTaskID := "task-manual-retry-1"
		if handlerCalls > 1 {
			publicTaskID = "task-manual-retry-2"
		}
		attempt, createErr := model.CreatePreparedTaskAttempt(model.TaskCreateAttemptParams{
			PublicTaskID:   publicTaskID,
			UserID:         7,
			ClientProtocol: model.TaskClientProtocolModelArkV3,
			RequestHash:    "manual-retry-attempt",
		})
		require.NoError(t, createErr)
		claimID := int64(common.GetContextKeyInt(c, constant.ContextKeyTaskIdempotencyID))
		require.NotZero(t, claimID)
		require.NoError(t, model.BindTaskCreateIdempotencyAttempt(claimID, attempt.AttemptID))

		if handlerCalls == 1 {
			firstAttemptID = attempt.AttemptID
			transitioned, transitionErr := model.TransitionTaskCreateAttempt(
				nil,
				attempt.ID,
				model.TaskCreateAttemptPrepared,
				model.TaskCreateAttemptBillingUnheld,
				model.TaskCreateAttemptSending,
				model.TaskCreateAttemptBillingHeld,
				nil,
			)
			require.NoError(t, transitionErr)
			require.True(t, transitioned)
			require.NoError(t, model.MarkTaskCreateAttemptUnknown(attempt.ID, "provider-request-id"))
			common.SetContextKey(c, constant.ContextKeyTaskUpstreamStarted, true)
			c.Status(http.StatusBadGateway)
			return
		}

		secondAttemptID = attempt.AttemptID
		common.SetContextKey(c, constant.ContextKeyTaskIdempotencyCompletedNoReplay, true)
		c.JSON(http.StatusCreated, gin.H{"id": attempt.PublicTaskID})
	})

	firstRequest := httptest.NewRequest(http.MethodPost, "/tasks", strings.NewReader(requestBody))
	firstRequest.Header.Set("Content-Type", "application/json")
	firstRequest.Header.Set("Idempotency-Key", idempotencyKey)
	firstResponse := httptest.NewRecorder()
	engine.ServeHTTP(firstResponse, firstRequest)
	require.Equal(t, http.StatusBadGateway, firstResponse.Code)
	require.NotEmpty(t, firstAttemptID)

	var firstClaim model.TaskCreateIdempotency
	require.NoError(t, db.Where("attempt_id = ?", firstAttemptID).First(&firstClaim).Error)
	require.Equal(t, model.TaskCreateIdempotencyUnknown, firstClaim.Status)
	_, err = model.RejectUnknownTaskCreateAttempt(
		firstAttemptID,
		99,
		"provider console verified no task",
	)
	require.NoError(t, err)
	var releasedClaimCount int64
	require.NoError(t, db.Model(&model.TaskCreateIdempotency{}).
		Where("id = ?", firstClaim.ID).
		Count(&releasedClaimCount).Error)
	require.Zero(t, releasedClaimCount)

	secondRequest := httptest.NewRequest(http.MethodPost, "/tasks", strings.NewReader(requestBody))
	secondRequest.Header.Set("Content-Type", "application/json")
	secondRequest.Header.Set("Idempotency-Key", idempotencyKey)
	secondResponse := httptest.NewRecorder()
	engine.ServeHTTP(secondResponse, secondRequest)

	assert.Equal(t, http.StatusCreated, secondResponse.Code)
	assert.Equal(t, 2, handlerCalls)
	assert.NotEmpty(t, secondAttemptID)
	assert.NotEqual(t, firstAttemptID, secondAttemptID)
	assert.Contains(t, secondResponse.Body.String(), `"id":"task-manual-retry-2"`)
	var retriedClaim model.TaskCreateIdempotency
	require.NoError(t, db.Where("attempt_id = ?", secondAttemptID).First(&retriedClaim).Error)
	assert.Equal(t, model.TaskCreateIdempotencyCompletedNoReplay, retriedClaim.Status)
}
