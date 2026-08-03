package middleware

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestGPTImage2DistributionPreservesPriorityWeightAffinityAndTaskIsolation(t *testing.T) {
	gin.SetMode(gin.TestMode)

	previousDB := model.DB
	previousMemoryCacheEnabled := common.MemoryCacheEnabled
	previousRedisEnabled := common.RedisEnabled
	previousMainDatabaseType := common.MainDatabaseType()
	affinitySetting := operation_setting.GetChannelAffinitySetting()
	previousAffinitySetting := *affinitySetting

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	require.NoError(t, db.AutoMigrate(
		&model.Channel{},
		&model.Ability{},
		&model.Task{},
		&model.TaskCreateIdempotency{},
	))
	model.DB = db
	common.MemoryCacheEnabled = false
	common.RedisEnabled = false
	common.SetMainDatabaseType(common.DatabaseTypeSQLite)
	*affinitySetting = operation_setting.ChannelAffinitySetting{
		Enabled:           true,
		SwitchOnSuccess:   true,
		DefaultTTLSeconds: 60,
		Rules: []operation_setting.ChannelAffinityRule{{
			Name:       "gpt-image-2-regression",
			ModelRegex: []string{`^gpt-image-2$`},
			PathRegex:  []string{`^/v1/images/generations$`},
			KeySources: []operation_setting.ChannelAffinityKeySource{{
				Type: "request_header",
				Key:  "X-Image-Affinity",
			}},
			IncludeRuleName:   true,
			IncludeModelName:  true,
			IncludeUsingGroup: true,
		}},
	}
	service.ClearChannelAffinityCacheAll()
	t.Cleanup(func() {
		service.ClearChannelAffinityCacheAll()
		*affinitySetting = previousAffinitySetting
		common.SetMainDatabaseType(previousMainDatabaseType)
		common.RedisEnabled = previousRedisEnabled
		common.MemoryCacheEnabled = previousMemoryCacheEnabled
		model.DB = previousDB
		if previousMemoryCacheEnabled && previousDB != nil {
			model.InitChannelCache()
		}
		_ = sqlDB.Close()
	})

	priorityA, priorityB := int64(20), int64(5)
	weightA, weightB := uint(1), uint(5000)
	channels := []model.Channel{
		{
			Id: 7101, Name: "gpt-image-2-a", Type: constant.ChannelTypeOpenAI,
			Key: "key-a", Status: common.ChannelStatusEnabled, Group: "default", Models: "gpt-image-2",
			Priority: &priorityA, Weight: &weightA,
		},
		{
			Id: 7102, Name: "gpt-image-2-b", Type: constant.ChannelTypeOpenAI,
			Key: "key-b", Status: common.ChannelStatusEnabled, Group: "default", Models: "gpt-image-2",
			Priority: &priorityB, Weight: &weightB,
		},
	}
	require.NoError(t, db.Create(&channels).Error)
	abilities := []model.Ability{
		{Group: "default", Model: "gpt-image-2", ChannelId: 7101, Enabled: true, Priority: &priorityA, Weight: weightA},
		{Group: "default", Model: "gpt-image-2", ChannelId: 7102, Enabled: true, Priority: &priorityB, Weight: weightB},
	}
	require.NoError(t, db.Create(&abilities).Error)
	common.MemoryCacheEnabled = true
	model.InitChannelCache()

	selectedChannelIDs := make([]int, 0, 3)
	claimObserved := false
	engine := gin.New()
	engine.Use(func(c *gin.Context) {
		common.SetContextKey(c, constant.ContextKeyTaskClientProtocol, model.TaskClientProtocolOpenAIImages)
		common.SetContextKey(c, constant.ContextKeyUsingGroup, "default")
		common.SetContextKey(c, constant.ContextKeyUserGroup, "default")
		c.Next()
	})
	engine.Use(Distribute(), ImageTaskCreateIdempotency())
	engine.POST("/v1/images/generations", func(c *gin.Context) {
		selectedChannelIDs = append(selectedChannelIDs, common.GetContextKeyInt(c, constant.ContextKeyChannelId))
		if _, ok := common.GetContextKey(c, constant.ContextKeyTaskIdempotencyID); ok {
			claimObserved = true
		}
		c.Status(http.StatusNoContent)
	})

	sendRequest := func(affinity string) int {
		request := httptest.NewRequest(
			http.MethodPost,
			"/v1/images/generations",
			strings.NewReader(`{"model":"gpt-image-2","prompt":"a production image"}`),
		)
		request.Header.Set("Content-Type", "application/json")
		request.Header.Set("Idempotency-Key", "gpt-image-2-shared-key")
		request.Header.Set("X-Image-Affinity", affinity)
		recorder := httptest.NewRecorder()
		engine.ServeHTTP(recorder, request)
		return recorder.Code
	}

	assert.Equal(t, http.StatusNoContent, sendRequest("tenant-a"))

	require.NoError(t, db.Model(&model.Ability{}).
		Where("channel_id = ?", 7101).
		Update("priority", int64(1)).Error)
	require.NoError(t, db.Model(&model.Ability{}).
		Where("channel_id = ?", 7102).
		Update("priority", int64(30)).Error)
	require.NoError(t, db.Model(&model.Channel{}).
		Where("id = ?", 7101).
		Update("priority", int64(1)).Error)
	require.NoError(t, db.Model(&model.Channel{}).
		Where("id = ?", 7102).
		Update("priority", int64(30)).Error)
	model.InitChannelCache()

	assert.Equal(t, http.StatusNoContent, sendRequest("tenant-b"))
	assert.Equal(t, http.StatusNoContent, sendRequest("tenant-a"))
	assert.Equal(t, []int{7101, 7102, 7101}, selectedChannelIDs)
	assert.False(t, claimObserved)

	var storedAbilities []model.Ability
	require.NoError(t, db.Where("model = ?", "gpt-image-2").Order("channel_id").Find(&storedAbilities).Error)
	require.Len(t, storedAbilities, 2)
	assert.Equal(t, weightA, storedAbilities[0].Weight)
	assert.Equal(t, weightB, storedAbilities[1].Weight)

	var taskCount int64
	require.NoError(t, db.Model(&model.Task{}).Count(&taskCount).Error)
	assert.Zero(t, taskCount)
	var claimCount int64
	require.NoError(t, db.Model(&model.TaskCreateIdempotency{}).Count(&claimCount).Error)
	assert.Zero(t, claimCount)
}

func TestGPTImage2ConcurrentSameIdempotencyKeyDoesNotConflict(t *testing.T) {
	gin.SetMode(gin.TestMode)

	var handlerCalls atomic.Int32
	firstEntered := make(chan struct{}, 1)
	secondEntered := make(chan struct{}, 1)
	releaseFirst := make(chan struct{})
	engine := gin.New()
	engine.Use(func(c *gin.Context) {
		common.SetContextKey(c, constant.ContextKeyTaskClientProtocol, model.TaskClientProtocolOpenAIImages)
		common.SetContextKey(c, constant.ContextKeyChannelType, constant.ChannelTypeOpenAI)
		common.SetContextKey(c, constant.ContextKeyOriginalModel, "gpt-image-2")
		c.Next()
	})
	engine.Use(ImageTaskCreateIdempotency())
	engine.POST("/v1/images/generations", func(c *gin.Context) {
		switch handlerCalls.Add(1) {
		case 1:
			firstEntered <- struct{}{}
			<-releaseFirst
		case 2:
			secondEntered <- struct{}{}
		}
		c.Status(http.StatusNoContent)
	})

	sendRequest := func() int {
		request := httptest.NewRequest(
			http.MethodPost,
			"/v1/images/generations",
			strings.NewReader(`{"model":"gpt-image-2","prompt":"a production image"}`),
		)
		request.Header.Set("Content-Type", "application/json")
		request.Header.Set("Idempotency-Key", "gpt-image-2-concurrent-key")
		recorder := httptest.NewRecorder()
		engine.ServeHTTP(recorder, request)
		return recorder.Code
	}

	firstResult := make(chan int, 1)
	go func() {
		firstResult <- sendRequest()
	}()
	<-firstEntered

	secondResult := make(chan int, 1)
	go func() {
		secondResult <- sendRequest()
	}()

	var secondStatus int
	select {
	case <-secondEntered:
		close(releaseFirst)
		secondStatus = <-secondResult
	case secondStatus = <-secondResult:
		close(releaseFirst)
	}

	assert.Equal(t, http.StatusNoContent, <-firstResult)
	assert.Equal(t, http.StatusNoContent, secondStatus)
	assert.Equal(t, int32(2), handlerCalls.Load())
}
