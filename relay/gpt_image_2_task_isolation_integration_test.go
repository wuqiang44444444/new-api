package relay

import (
	"io"
	"math"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/perf_metrics_setting"
	"github.com/QuantumNous/new-api/types"
	"github.com/bytedance/gopkg/util/gopool"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestGPTImage2SynchronousRelayDoesNotCreateTask(t *testing.T) {
	gin.SetMode(gin.TestMode)
	service.InitHttpClient()

	var upstreamCalls atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		upstreamCalls.Add(1)
		assert.Equal(t, http.MethodPost, request.Method)
		assert.Equal(t, "/v1/images/generations", request.URL.Path)
		body, err := io.ReadAll(request.Body)
		require.NoError(t, err)
		assert.JSONEq(t, `{"model":"gpt-image-2","prompt":"a production image","n":1}`, string(body))
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{
			"created":1700000000,
			"data":[{"url":"https://cdn.example/gpt-image-2.png"}],
			"usage":{"input_tokens":1,"output_tokens":1,"total_tokens":2}
		}`))
	}))
	defer upstream.Close()

	previousDB, previousLogDB := model.DB, model.LOG_DB
	previousMainDatabaseType := common.MainDatabaseType()
	previousLogDatabaseType := common.LogDatabaseType()
	previousRedisEnabled := common.RedisEnabled
	previousBatchUpdateEnabled := common.BatchUpdateEnabled
	previousLogConsumeEnabled := common.LogConsumeEnabled
	perfSetting := perf_metrics_setting.GetSetting()
	previousPerfMetricsEnabled := perfSetting.Enabled
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	model.DB, model.LOG_DB = db, db
	common.SetDatabaseTypes(common.DatabaseTypeSQLite, common.DatabaseTypeSQLite)
	common.RedisEnabled = false
	common.BatchUpdateEnabled = false
	common.LogConsumeEnabled = true
	perfSetting.Enabled = false
	gopool.SetCap(1)
	t.Setenv("LOG_SQL_DSN", "")
	require.NoError(t, model.InitLogDB())
	require.NoError(t, db.AutoMigrate(
		&model.Task{},
		&model.TaskCreateIdempotency{},
		&model.User{},
		&model.Channel{},
		&model.Log{},
	))
	t.Cleanup(func() {
		model.DB, model.LOG_DB = previousDB, previousLogDB
		common.SetDatabaseTypes(previousMainDatabaseType, previousLogDatabaseType)
		common.RedisEnabled = previousRedisEnabled
		common.BatchUpdateEnabled = previousBatchUpdateEnabled
		common.LogConsumeEnabled = previousLogConsumeEnabled
		perfSetting.Enabled = previousPerfMetricsEnabled
		gopool.SetCap(math.MaxInt32)
		_ = sqlDB.Close()
	})

	require.NoError(t, db.Create(&model.User{
		Id: 7301, Username: "gpt-image-2-user", Quota: 1_000_000, Status: common.UserStatusEnabled,
	}).Error)
	require.NoError(t, db.Create(&model.Channel{
		Id: 7302, Name: "gpt-image-2-openai", Type: constant.ChannelTypeOpenAI,
		Key: "upstream-key", Status: common.ChannelStatusEnabled,
	}).Error)

	requestBody := `{"model":"gpt-image-2","prompt":"a production image","n":1}`
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/images/generations", strings.NewReader(requestBody))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set("token_name", "gpt-image-2-regression")
	c.Set("username", "gpt-image-2-user")
	common.SetContextKey(c, constant.ContextKeyChannelId, 7302)
	common.SetContextKey(c, constant.ContextKeyChannelType, constant.ChannelTypeOpenAI)
	common.SetContextKey(c, constant.ContextKeyChannelBaseUrl, upstream.URL)
	common.SetContextKey(c, constant.ContextKeyChannelKey, "upstream-key")
	common.SetContextKey(c, constant.ContextKeyOriginalModel, "gpt-image-2")
	common.SetContextKey(c, constant.ContextKeyChannelSetting, dto.ChannelSettings{})
	common.SetContextKey(c, constant.ContextKeyChannelOtherSetting, dto.ChannelOtherSettings{})

	imageCount := uint(1)
	imageRequest := &dto.ImageRequest{
		Model:  "gpt-image-2",
		Prompt: "a production image",
		N:      &imageCount,
	}
	startedAt := time.Now()
	info := &relaycommon.RelayInfo{
		UserId:            7301,
		UsingGroup:        "default",
		OriginModelName:   "gpt-image-2",
		RequestURLPath:    "/v1/images/generations",
		Request:           imageRequest,
		RelayMode:         relayconstant.RelayModeImagesGenerations,
		RelayFormat:       types.RelayFormatOpenAIImage,
		StartTime:         startedAt,
		FirstResponseTime: startedAt.Add(100 * time.Millisecond),
		PriceData: types.PriceData{
			ModelPrice: 0.04,
			UsePrice:   true,
			GroupRatioInfo: types.GroupRatioInfo{
				GroupRatio: 1,
			},
		},
		FinalPreConsumedQuota: 20_000,
	}
	info.PriceData.AddOtherRatio("n", 1)

	PreparePersistentImageTaskRequest(c, info)
	imageErr := ImageHelper(c, info)
	asyncBarrier := make(chan struct{})
	gopool.Go(func() {
		close(asyncBarrier)
	})
	<-asyncBarrier
	require.Nil(t, imageErr)

	assert.Equal(t, http.StatusOK, recorder.Code)
	assert.JSONEq(t, `{
		"created":1700000000,
		"data":[{"url":"https://cdn.example/gpt-image-2.png"}],
		"usage":{"input_tokens":1,"output_tokens":1,"total_tokens":2}
	}`, recorder.Body.String())
	assert.Equal(t, int32(1), upstreamCalls.Load())
	assert.False(t, common.GetContextKeyBool(c, constant.ContextKeyTaskPersistenceEnabled))
	assert.Nil(t, info.TaskRelayInfo)
	assert.False(t, info.BillingTransferredToTask)

	var taskCount int64
	require.NoError(t, db.Model(&model.Task{}).Count(&taskCount).Error)
	assert.Zero(t, taskCount)
	var claimCount int64
	require.NoError(t, db.Model(&model.TaskCreateIdempotency{}).Count(&claimCount).Error)
	assert.Zero(t, claimCount)
}
