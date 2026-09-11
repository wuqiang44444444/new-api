package controller

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/plugins"
	taskseedance "github.com/QuantumNous/new-api/relay/channel/task/seedance"
	"github.com/QuantumNous/new-api/relay/channel/task/seedance/thirdparty/feicai"
	kitdto "github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/config"
	"github.com/QuantumNous/new-api/setting/system_setting"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// This fixture closes the gap left by the per-stage unit tests: one
// controller-level run over the real middleware chain (TokenAuth →
// ModelArkVideoCreateConvert → ResolveSeedanceChannel → RelayTask) and the real
// polling entry (service.UpdateVideoTasks with the code-registered Seedance
// adaptor), asserting the exact money facts at every stage of the seedance-link
// plugin path: pin → prepared freeze → byte-equal provider POST → insert-time
// hold transfer → observation classification → settle/refund.

const seedanceFundsInitialQuota = 10_000_000

// seedanceFundsHold is the frozen pre-consume for the fixture expression
// tier("feicai", param("_task.duration_seconds") * param("_task.size_multiplier") * 250000)
// evaluated over the plugin probe of the fixture request:
// 4 × 1 × 250000 = 1_000_000 → /1e6 × common.QuotaPerUnit(500000) = 500000 quota.
const seedanceFundsHold = 500_000

type seedanceFundsFixture struct {
	t         *testing.T
	db        *gorm.DB
	engine    *gin.Engine
	server    *httptest.Server
	userID    int
	channelID int

	createCalls   atomic.Int32
	queryCalls    atomic.Int32
	createStatus  atomic.Int32
	createBodyRes atomic.Value // string
	queryStatus   atomic.Int32
	queryBodyRes  atomic.Value // string

	createBodyMu sync.Mutex
	createBody   []byte

	// assertAttemptAtPost makes the provider handler verify, at the moment the
	// request bytes arrive, that the durable attempt already froze the plugin
	// pin and the funds hold — the exact window the deletion race closes.
	assertAttemptAtPost atomic.Bool

	interposerMu sync.Mutex
	interposerFn func(*gin.Context)
}

func newSeedanceFundsFixture(t *testing.T) *seedanceFundsFixture {
	t.Helper()
	service.InitHttpClient()
	previousDBType, previousLogType := common.MainDatabaseType(), common.LogDatabaseType()
	common.SetDatabaseTypes(common.DatabaseTypeSQLite, common.DatabaseTypeSQLite)
	t.Cleanup(func() { common.SetDatabaseTypes(previousDBType, previousLogType) })

	fx := &seedanceFundsFixture{t: t, userID: 9101, channelID: 9101}
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	require.NoError(t, db.AutoMigrate(
		&model.User{}, &model.Token{}, &model.Channel{}, &model.Log{}, &model.Task{},
		&model.TaskCreateIdempotency{}, &model.TaskCreateAttempt{}, &model.UserSubscription{},
		&model.TaskPlugin{}, &model.SubscriptionPreConsumeRecord{},
	))
	fx.db = db
	previousDB := model.DB
	model.DB = db
	t.Cleanup(func() { model.DB = previousDB })

	oldLog, oldCache, oldRedis, oldBatch, oldConsume := model.LOG_DB, common.MemoryCacheEnabled, common.RedisEnabled, common.BatchUpdateEnabled, common.LogConsumeEnabled
	t.Setenv("LOG_SQL_DSN", "")
	require.NoError(t, model.InitLogDB())
	model.LOG_DB = db
	common.MemoryCacheEnabled, common.RedisEnabled, common.BatchUpdateEnabled = false, false, false
	common.LogConsumeEnabled = true
	t.Cleanup(func() {
		model.LOG_DB = oldLog
		common.MemoryCacheEnabled, common.RedisEnabled, common.BatchUpdateEnabled, common.LogConsumeEnabled = oldCache, oldRedis, oldBatch, oldConsume
	})

	oldEvidence := system_setting.GetTaskRequestEvidenceConfig()
	system_setting.SetTaskRequestEvidenceConfig(system_setting.TaskRequestEvidenceConfig{Enabled: false})
	t.Cleanup(func() { system_setting.SetTaskRequestEvidenceConfig(oldEvidence) })

	saved := map[string]string{}
	require.NoError(t, config.GlobalConfig.SaveToDB(func(k, v string) error { saved[k] = v; return nil }))
	t.Cleanup(func() { require.NoError(t, config.GlobalConfig.LoadFromDB(saved)) })
	require.NoError(t, config.GlobalConfig.LoadFromDB(map[string]string{
		"billing_setting.billing_mode":           `{"customer-video":"tiered_expr"}`,
		"billing_setting.billing_expr":           `{"customer-video":"tier(\"feicai\", param(\"_task.duration_seconds\") * param(\"_task.size_multiplier\") * 250000)"}`,
		"task_billing_setting.preconsume_tokens": `{"customer-video":100000}`,
	}))

	user := model.User{Id: fx.userID, Username: "seedance-funds-fixture", Quota: seedanceFundsInitialQuota, Status: common.UserStatusEnabled, Group: "default"}
	user.SetSetting(kitdto.UserSetting{BillingPreference: "wallet_only"})
	require.NoError(t, db.Create(&user).Error)
	token := model.Token{Id: fx.userID, UserId: fx.userID, Key: strings.Repeat("f", 32), Status: common.TokenStatusEnabled, RemainQuota: seedanceFundsInitialQuota, ExpiredTime: -1, Group: "default"}
	require.NoError(t, db.Create(&token).Error)

	mapping, err := common.Marshal(map[string]string{"customer-video": feicai.ProviderModelSeedance20Mini720P})
	require.NoError(t, err)
	fx.startProvider()
	channel := model.Channel{
		Id: fx.channelID, Type: constant.ChannelTypeSeedanceLink, Name: "seedance-funds-fixture",
		Key: "provider-fixture-key", BaseURL: &fx.server.URL, Status: common.ChannelStatusEnabled,
		Models: "customer-video", Group: "default", ModelMapping: common.GetPointer(string(mapping)),
	}
	channel.SetOtherSettings(dto.ChannelOtherSettings{VideoUpstreamProtocol: kitdto.VideoUpstreamProtocolFeicaiVideosV1})
	require.NoError(t, db.Create(&channel).Error)

	fx.seedExtension(seedanceGoodExtensionRows())

	fx.engine = gin.New()
	fx.engine.POST("/api/v3/contents/generations/tasks",
		middleware.TokenAuth(),
		middleware.TaskClientProtocol("modelark_v3"),
		middleware.TaskCreateResponseContract(),
		middleware.ModelArkVideoCreateConvert(),
		middleware.ResolveSeedanceChannel(),
		fx.interposer,
		RelayTask,
	)

	previousAdaptorFunc := service.GetTaskAdaptorFunc
	service.GetTaskAdaptorFunc = func(platform constant.TaskPlatform) service.TaskPollingAdaptor {
		if string(platform) == "62" {
			return &taskseedance.TaskAdaptor{}
		}
		return nil
	}
	t.Cleanup(func() { service.GetTaskAdaptorFunc = previousAdaptorFunc })
	return fx
}

func seedanceGoodExtensionRows() []model.TaskPlugin {
	source := plugins.SeedanceSource()
	return []model.TaskPlugin{{
		Key:        taskseedance.SeedanceExtensionPluginKey,
		Version:    plugins.SeedanceVersion(),
		APIVersion: 3,
		Source:     source,
		SourceHash: fmt.Sprintf("%x", common.Sha256Raw([]byte(source))),
		Enabled:    true,
		Active:     true,
	}}
}

// seedExtension persists the version rows in the fixture database (the
// ResolveVersion boundary reads the database, not the compiled store) and
// publishes them as the active extension store content. The store is restored
// to empty after the test so package state never leaks between fixtures.
func (fx *seedanceFundsFixture) seedExtension(rows []model.TaskPlugin) {
	fx.t.Helper()
	for i := range rows {
		require.NoError(fx.t, fx.db.Create(&rows[i]).Error)
	}
	require.NoError(fx.t, taskseedance.SyncExtensionSnapshot(nil, rows))
	fx.t.Cleanup(func() { _ = taskseedance.SyncExtensionSnapshot(nil, nil) })
}

func (fx *seedanceFundsFixture) startProvider() {
	fx.t.Helper()
	fx.createStatus.Store(http.StatusOK)
	fx.createBodyRes.Store(`{"id":"prov-task-1"}`)
	fx.queryStatus.Store(http.StatusOK)
	fx.queryBodyRes.Store(`{"id":"prov-task-1","status":"queued"}`)
	var handler http.HandlerFunc = func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		assert.NoError(fx.t, err)
		if r.Method == http.MethodPost {
			fx.createCalls.Add(1)
			fx.createBodyMu.Lock()
			fx.createBody = body
			fx.createBodyMu.Unlock()
			if fx.assertAttemptAtPost.Load() {
				fx.assertHeldAttemptAtPost()
			}
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(int(fx.createStatus.Load()))
			_, _ = w.Write([]byte(fx.createBodyRes.Load().(string)))
			return
		}
		fx.queryCalls.Add(1)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(int(fx.queryStatus.Load()))
		_, _ = w.Write([]byte(fx.queryBodyRes.Load().(string)))
	}
	fx.server = httptest.NewServer(handler)
	fx.t.Cleanup(fx.server.Close)
}

// assertHeldAttemptAtPost pins the concurrency window fix: by the time request
// bytes reach the provider, the prepared attempt must already carry the frozen
// plugin identity and the funds hold.
func (fx *seedanceFundsFixture) assertHeldAttemptAtPost() {
	fx.t.Helper()
	var attempt model.TaskCreateAttempt
	require.NoError(fx.t, fx.db.First(&attempt).Error)
	require.Equal(fx.t, model.TaskCreateAttemptSending, attempt.Status)
	require.Equal(fx.t, model.TaskCreateAttemptBillingHeld, attempt.BillingHoldState)
	require.Equal(fx.t, seedanceFundsHold, attempt.HeldQuota)
	require.NotNil(fx.t, attempt.FrozenConnectionSnapshot)
	var frozen struct {
		PluginKey     string `json:"plugin_key"`
		PluginVersion string `json:"plugin_version"`
	}
	require.NoError(fx.t, common.Unmarshal(attempt.FrozenConnectionSnapshot, &frozen))
	assert.Equal(fx.t, taskseedance.SeedanceExtensionPluginKey, frozen.PluginKey)
	assert.Equal(fx.t, plugins.SeedanceVersion(), frozen.PluginVersion)
}

// interposer runs between channel pinning and task submission. It stays empty
// unless a test installs work into it, simulating events that race the
// in-flight request (for example an administrator deleting the pinned version
// between pin and request build).
func (fx *seedanceFundsFixture) interposer(c *gin.Context) {
	fx.interposerMu.Lock()
	fn := fx.interposerFn
	fx.interposerMu.Unlock()
	if fn != nil {
		fn(c)
	}
}

func (fx *seedanceFundsFixture) onInterpose(fn func(*gin.Context)) {
	fx.t.Helper()
	fx.interposerMu.Lock()
	fx.interposerFn = fn
	fx.interposerMu.Unlock()
	fx.t.Cleanup(func() {
		fx.interposerMu.Lock()
		fx.interposerFn = nil
		fx.interposerMu.Unlock()
	})
}

func (fx *seedanceFundsFixture) submitCreate() string {
	fx.t.Helper()
	requestBody := `{"model":"customer-video","content":[{"type":"text","text":"A landscape"}],"duration":4,"resolution":"720p","ratio":"21:9"}`
	return fx.submitCreateBody(requestBody)
}

func (fx *seedanceFundsFixture) submitCreateBody(requestBody string) string {
	fx.t.Helper()
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/v3/contents/generations/tasks", bytes.NewBufferString(requestBody))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", "Bearer sk-"+strings.Repeat("f", 32))
	fx.engine.ServeHTTP(recorder, request)
	return strings.TrimSpace(recorder.Body.String())
}

func (fx *seedanceFundsFixture) createRequestBody() []byte {
	fx.t.Helper()
	fx.createBodyMu.Lock()
	defer fx.createBodyMu.Unlock()
	return append([]byte(nil), fx.createBody...)
}

func (fx *seedanceFundsFixture) legacyOracleBody() []byte {
	fx.t.Helper()
	duration, resolution, ratio := 4, "720p", "21:9"
	request := &dto.ModelArkVideoCreateRequest{
		Model:      "customer-video",
		Duration:   &duration,
		Resolution: &resolution,
		Ratio:      &ratio,
		Content: []dto.ModelArkVideoContent{
			{Type: "text", Text: common.GetPointer("A landscape")},
		},
	}
	body, err := feicai.CreateRequest(request, feicai.ProviderModelSeedance20Mini720P)
	require.NoError(fx.t, err)
	return body
}

func (fx *seedanceFundsFixture) loadTask(publicID string) model.Task {
	fx.t.Helper()
	var task model.Task
	require.NoError(fx.t, fx.db.Where("task_id = ?", publicID).First(&task).Error)
	return task
}

func (fx *seedanceFundsFixture) pollOnce(publicID string) {
	fx.t.Helper()
	task := fx.loadTask(publicID)
	tasks := map[string]*model.Task{task.TaskID: &task}
	require.NoError(fx.t, service.UpdateVideoTasks(
		context.Background(), constant.TaskPlatform("62"),
		map[int][]string{fx.channelID: {task.TaskID}}, tasks,
	))
}

func (fx *seedanceFundsFixture) userQuota() int {
	fx.t.Helper()
	var user model.User
	require.NoError(fx.t, fx.db.Select("quota").First(&user, fx.userID).Error)
	return user.Quota
}

func (fx *seedanceFundsFixture) tokenQuota() (int, int) {
	fx.t.Helper()
	var token model.Token
	require.NoError(fx.t, fx.db.Select("remain_quota", "used_quota").First(&token, fx.userID).Error)
	return token.RemainQuota, token.UsedQuota
}

func (fx *seedanceFundsFixture) logsByType() map[int][]model.Log {
	fx.t.Helper()
	var logs []model.Log
	require.NoError(fx.t, model.LOG_DB.Order("id asc").Find(&logs).Error)
	byType := map[int][]model.Log{}
	for _, entry := range logs {
		byType[entry.Type] = append(byType[entry.Type], entry)
	}
	return byType
}

// TestSeedancePluginFundsChainCreateToSettle walks the whole successful money
// chain over the plugin path: the pin is durable before the POST, the provider
// receives exactly the legacy Go request bytes, the hold transfers onto the
// task at insert, and the succeeded observation settles at the frozen probe
// without any further money movement.
func TestSeedancePluginFundsChainCreateToSettle(t *testing.T) {
	fx := newSeedanceFundsFixture(t)
	fx.assertAttemptAtPost.Store(true)
	fx.queryBodyRes.Store(fmt.Sprintf(`{"id":"prov-task-1","status":"completed","video_url":%q}`, fx.server.URL+"/v/1.mp4"))

	publicID := decodeSeedanceFundsCreateID(t, fx.submitCreate())

	// Provider received byte-identical bytes to the legacy Go conversion.
	require.Equal(t, 1, int(fx.createCalls.Load()))
	require.Equal(t, string(fx.legacyOracleBody()), string(fx.createRequestBody()))

	// Insert-time atomic transfer: attempt complete, hold transferred to task.
	var attempt model.TaskCreateAttempt
	require.NoError(t, fx.db.First(&attempt).Error)
	assert.Equal(t, model.TaskCreateAttemptComplete, attempt.Status)
	assert.Equal(t, model.TaskCreateAttemptBillingTransferred, attempt.BillingHoldState)
	assert.Equal(t, seedanceFundsHold, attempt.HeldQuota)
	assert.Equal(t, "prov-task-1", attempt.UpstreamTaskID)
	assert.Equal(t, "wallet", attempt.BillingSource)

	task := fx.loadTask(publicID)
	assert.Equal(t, constant.TaskPlatform("62"), constant.TaskPlatform(task.Platform))
	assert.Equal(t, "prov-task-1", task.PrivateData.UpstreamTaskID)
	assert.Equal(t, seedanceFundsHold, task.Quota)
	require.NotNil(t, task.PrivateData.Execution)
	require.NotNil(t, task.PrivateData.Execution.TaskPlugin)
	assert.Equal(t, taskseedance.SeedanceExtensionPluginKey, task.PrivateData.Execution.TaskPlugin.Key)
	assert.Equal(t, plugins.SeedanceVersion(), task.PrivateData.Execution.TaskPlugin.Version)
	require.NotNil(t, task.PrivateData.AsyncBilling)
	assert.Equal(t, model.TaskBillingStatePending, task.PrivateData.AsyncBilling.State)
	require.NotNil(t, task.PrivateData.AsyncBilling.TieredSnapshot)
	assert.False(t, task.PrivateData.AsyncBilling.TieredSnapshot.TaskUsageBilling, "the plugin must not enter the native usage-billing branch")
	require.NotNil(t, task.PrivateData.AsyncBilling.BillingProbe)
	assert.Contains(t, string(task.PrivateData.AsyncBilling.BillingProbe.Body), `"duration_seconds":4`)
	assert.Contains(t, string(task.PrivateData.AsyncBilling.BillingProbe.Body), `"size_multiplier":1`)
	assert.Equal(t, model.TaskBillingStatePending, task.BillingState)

	// Hold funds: user wallet and token both carry the reservation.
	assert.Equal(t, seedanceFundsInitialQuota-seedanceFundsHold, fx.userQuota())
	remain, used := fx.tokenQuota()
	assert.Equal(t, seedanceFundsInitialQuota-seedanceFundsHold, remain)
	assert.Equal(t, seedanceFundsHold, used)

	// The create consume log charges exactly the held amount.
	logs := fx.logsByType()
	require.Len(t, logs[model.LogTypeConsume], 1)
	assert.Equal(t, seedanceFundsHold, logs[model.LogTypeConsume][0].Quota)
	assert.Equal(t, "customer-video", logs[model.LogTypeConsume][0].ModelName)

	// Succeeded observation settles at the frozen probe: no further movement.
	fx.pollOnce(publicID)
	task = fx.loadTask(publicID)
	assert.Equal(t, model.TaskStatus(model.TaskStatusSuccess), task.Status)
	assert.Equal(t, fx.server.URL+"/v/1.mp4", task.PrivateData.ResultURL)
	assert.Equal(t, seedanceFundsHold, task.Quota)
	assert.Equal(t, model.TaskBillingStateSettled, task.PrivateData.AsyncBilling.State)
	assert.Equal(t, model.TaskBillingStateSettled, task.BillingState)
	assert.Equal(t, seedanceFundsInitialQuota-seedanceFundsHold, fx.userQuota())
	remain, used = fx.tokenQuota()
	assert.Equal(t, seedanceFundsInitialQuota-seedanceFundsHold, remain)
	assert.Equal(t, seedanceFundsHold, used)
	logs = fx.logsByType()
	require.Len(t, logs[model.LogTypeConsume], 1)
	assert.Empty(t, logs[model.LogTypeRefund])
}

// TestSeedancePluginFundsChainFailureRefundsExactly proves a trusted failed
// observation refunds the full hold exactly once through the plugin path.
func TestSeedancePluginFundsChainFailureRefundsExactly(t *testing.T) {
	fx := newSeedanceFundsFixture(t)
	fx.queryBodyRes.Store(`{"id":"prov-task-1","status":"failed","error":{"code":"E01","message":"content policy rejection"}}`)

	publicID := decodeSeedanceFundsCreateID(t, fx.submitCreate())
	require.Equal(t, seedanceFundsInitialQuota-seedanceFundsHold, fx.userQuota())

	fx.pollOnce(publicID)

	task := fx.loadTask(publicID)
	assert.Equal(t, model.TaskStatus(model.TaskStatusFailure), task.Status)
	assert.Contains(t, task.FailReason, "content policy rejection")
	assert.Zero(t, task.Quota)
	assert.Equal(t, seedanceFundsInitialQuota, fx.userQuota())
	remain, used := fx.tokenQuota()
	assert.Equal(t, seedanceFundsInitialQuota, remain)
	assert.Zero(t, used)

	logs := fx.logsByType()
	require.Len(t, logs[model.LogTypeConsume], 1)
	require.Len(t, logs[model.LogTypeRefund], 1)
	assert.Equal(t, seedanceFundsHold, logs[model.LogTypeRefund][0].Quota)

	// A repeated poll of the same failed task must not refund twice.
	fx.pollOnce(publicID)
	assert.Equal(t, seedanceFundsInitialQuota, fx.userQuota())
	logs = fx.logsByType()
	require.Len(t, logs[model.LogTypeRefund], 1)
}

// TestSeedancePluginFundsChainUnknownOutcomeRetainsHold proves an unverified
// provider rejection stays unknown: the hold survives, no task is created, and
// the funds are untouched for the reconciliation path.
func TestSeedancePluginFundsChainUnknownOutcomeRetainsHold(t *testing.T) {
	fx := newSeedanceFundsFixture(t)
	fx.createStatus.Store(http.StatusInternalServerError)
	fx.createBodyRes.Store(`{"error":{"code":"InternalError","message":"backend exploded"}}`)

	body := fx.submitCreate()
	assert.Contains(t, body, "create_outcome_unknown")
	require.Equal(t, 1, int(fx.createCalls.Load()))
	assert.Equal(t, string(fx.legacyOracleBody()), string(fx.createRequestBody()))

	var count int64
	require.NoError(t, fx.db.Model(&model.Task{}).Count(&count).Error)
	assert.Zero(t, count, "no trusted provider task ID means no Task")

	var attempt model.TaskCreateAttempt
	require.NoError(t, fx.db.First(&attempt).Error)
	assert.Equal(t, model.TaskCreateAttemptUnknown, attempt.Status)
	assert.Equal(t, model.TaskCreateAttemptBillingHeld, attempt.BillingHoldState)
	assert.Equal(t, seedanceFundsHold, attempt.HeldQuota)

	assert.Equal(t, seedanceFundsInitialQuota-seedanceFundsHold, fx.userQuota())
	remain, used := fx.tokenQuota()
	assert.Equal(t, seedanceFundsInitialQuota-seedanceFundsHold, remain)
	assert.Equal(t, seedanceFundsHold, used)
}

// Deletion winning before prepared must reject admission without holding funds.
func TestSeedancePluginFundsChainDeletedVersionRejectedBeforeHold(t *testing.T) {
	fx := newSeedanceFundsFixture(t)
	fx.onInterpose(func(*gin.Context) {
		_, err := model.DeleteTaskPluginVersion(taskseedance.SeedanceExtensionPluginKey, plugins.SeedanceVersion())
		require.NoError(t, err)
	})
	body := fx.submitCreate()
	assert.NotContains(t, body, `"id":`)
	assert.Zero(t, fx.createCalls.Load())
	var count int64
	require.NoError(t, fx.db.Model(&model.TaskCreateAttempt{}).Count(&count).Error)
	assert.Zero(t, count)
	require.NoError(t, fx.db.Model(&model.Task{}).Count(&count).Error)
	assert.Zero(t, count)
	assert.Equal(t, seedanceFundsInitialQuota, fx.userQuota())
	remain, used := fx.tokenQuota()
	assert.Equal(t, seedanceFundsInitialQuota, remain)
	assert.Zero(t, used)
}

// TestSeedancePluginFundsChainHostileConversionRejectedBeforeHold uploads a
// plugin version whose buildCreate rewrites the billed duration. The host must
// reject the conversion before pricing produces a hold: no attempt, no funds
// movement, no provider POST.
func TestSeedancePluginFundsChainHostileConversionRejectedBeforeHold(t *testing.T) {
	for _, tc := range []struct{ name, duration string }{
		{"explicit duration rewritten", `,"duration":4`},
		{"missing duration invented", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			requestBody := `{"model":"customer-video","content":[{"type":"text","text":"A landscape"}],"resolution":"720p","ratio":"21:9"` + tc.duration + `}`

			fx := newSeedanceFundsFixture(t)
			hostile := `
		export const meta = {
		  apiVersion: 1, key: "seedance-link", name: "hostile", version: "9.9.9",
		  author: { name: "t" }, seedanceProtocols: ["feicai_videos_v1"],
		};
		export const seedance = {
		  "feicai_videos_v1": {
		    buildCreate(input) {
		      return {
		        body: { model: input.providerModel, prompt: "x", duration: 15, ratio: "21:9" },
		        probe: { resolution: "720p", ratio: "21:9", size_multiplier: 1, billing_mode: "per-second" },
		      };
		    },
		    parseCreateResponse() { return { id: "hostile" }; },
		    parseTaskObservation() { return { violation: "x" }; },
		  },
		};
		`
			rows := seedanceGoodExtensionRows()
			rows[0].Version = "9.9.9"
			rows[0].Source = hostile
			rows[0].SourceHash = fmt.Sprintf("%x", common.Sha256Raw([]byte(hostile)))
			fx.seedExtension(rows)

			body := fx.submitCreateBody(requestBody)
			assert.Contains(t, body, "model_price_error")
			assert.Contains(t, body, "duration does not match")
			assert.Zero(t, fx.createCalls.Load())

			var attemptCount int64
			require.NoError(t, fx.db.Model(&model.TaskCreateAttempt{}).Count(&attemptCount).Error)
			assert.Zero(t, attemptCount, "the contradiction must be rejected before the durable hold")

			assert.Equal(t, seedanceFundsInitialQuota, fx.userQuota())
			remain, used := fx.tokenQuota()
			assert.Equal(t, seedanceFundsInitialQuota, remain)
			assert.Zero(t, used)

			var taskCount int64
			require.NoError(t, fx.db.Model(&model.Task{}).Count(&taskCount).Error)
			assert.Zero(t, taskCount)
		})
	}
}

func decodeSeedanceFundsCreateID(t *testing.T, body string) string {
	t.Helper()
	var parsed struct {
		ID string `json:"id"`
	}
	require.NoError(t, common.UnmarshalJsonStr(body, &parsed))
	require.NotEmpty(t, parsed.ID, body)
	return parsed.ID
}
