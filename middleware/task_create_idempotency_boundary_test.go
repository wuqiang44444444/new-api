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
