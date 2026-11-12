package service

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relaymode "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/gin-gonic/gin"
	"github.com/go-redis/redis/v8"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func TestChannelAlertDiscoversOnlineInstancesFromHeartbeats(t *testing.T) {
	db := withErrorReportTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.SystemInstance{}))
	previousNode := common.NodeName
	common.NodeName = "node-b"
	t.Cleanup(func() { common.NodeName = previousNode })
	now := time.Unix(1000, 0)
	for node, lastSeen := range map[string]int64{
		"node-a": 1000 - model.SystemInstanceStaleAfterSeconds,
		"node-b": 1000,
		"stale":  999 - model.SystemInstanceStaleAfterSeconds,
	} {
		require.NoError(t, model.UpsertSystemInstance(node, nil, 100, lastSeen))
	}

	nodes, valid := activeChannelAlertNodes(context.Background(), now)
	require.True(t, valid)
	assert.Equal(t, []string{"node-a", "node-b"}, nodes)

	nodes, valid = activeChannelAlertNodes(context.Background(), now.Add(time.Second))
	require.True(t, valid)
	assert.Equal(t, []string{"node-b"}, nodes)
}

func TestChannelAlertLocalWindowDeliversWithoutRedis(t *testing.T) {
	db := withErrorReportTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.Channel{}, &model.ChannelAlertEpisode{}, &model.ChannelAlertDelivery{}))
	require.NoError(t, model.UpdateOption("error_report_setting.recipients", "ops@example.com"))
	inbox := newLocalMailInbox(t)
	channel := model.Channel{Name: "local-fixture", Models: "model-a", Key: "fixture-private"}
	require.NoError(t, db.Create(&channel).Error)
	scope := channelAlertScope{Channel: channel.Id, Revision: model.ChannelAlertRevision(&channel), Model: "model-a", Operation: "chat", Slot: -1}
	window := channelAlertWindowState{localNode: "gateway-local"}
	window.append(channelAlertRound{start: 1000, batches: []channelAlertBatch{{Instance: "boot", Scopes: []channelAlertObservation{{
		Scope: scope,
		Failures: []channelAlertSample{
			{ID: "one", At: 1001000, Kind: channelAlertService, Status: 503},
			{ID: "two", At: 1002000, Kind: channelAlertService, Status: 503},
			{ID: "three", At: 1003000, Kind: channelAlertService, Status: 503},
		},
	}}}}}, 256)

	require.NoError(t, evaluateChannelAlertWindow(context.Background(), window, []string{"gateway-local"}, time.Unix(1005, 0)))
	sent, err := deliverChannelAlertMail("smtp-worker", common.SendEmailContext, time.Now)
	require.NoError(t, err)
	require.True(t, sent)
	mail := requireLocalMail(t, inbox, "Channel outage", "ops@example.com")
	assert.Contains(t, mail.body, "gateway-local")
	assert.NotContains(t, mail.body, channel.Key)
	var episode model.ChannelAlertEpisode
	require.NoError(t, db.First(&episode).Error)
	assert.Equal(t, "gateway-local", episode.Node)
}

func TestChannelAlertRedisWindowAndMail(t *testing.T) {
	addr := os.Getenv("CHANNEL_ALERT_TEST_REDIS")
	if addr == "" {
		t.Skip("set CHANNEL_ALERT_TEST_REDIS to an isolated Redis fixture")
	}
	ctx := context.Background()
	redisClient := redis.NewClient(&redis.Options{Addr: addr})
	require.NoError(t, redisClient.Ping(ctx).Err())
	previousRedis := common.RDB
	common.RDB = redisClient
	t.Cleanup(func() { common.RDB = previousRedis; _ = redisClient.Close() })
	db := withErrorReportTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.Channel{}, &model.ChannelAlertEpisode{}, &model.ChannelAlertDelivery{}))
	require.NoError(t, db.Create(&model.Option{Key: "error_report_setting.recipients", Value: "ops@example.com"}).Error)
	inbox := newLocalMailInbox(t)

	channel := model.Channel{Name: "fixture", Models: "model-a", Key: "fixture-private"}
	require.NoError(t, db.Create(&channel).Error)
	scope := channelAlertScope{Channel: channel.Id, Revision: model.ChannelAlertRevision(&channel), Model: "model-a", Operation: "chat", Slot: -1}
	nodes := []string{"a", "b"}
	members := channelAlertDigest(t.Name(), common.NewRequestId())
	start := channelAlertBucket(time.Now().Unix()) - 30
	current := time.Unix(start+25, 0)
	window := channelAlertWindowState{}

	storeBatch := func(node string, bucket int64, observations ...channelAlertObservation) {
		t.Helper()
		key := channelAlertBatchKey(members, node, bucket)
		t.Cleanup(func() { redisClient.Del(ctx, key) })
		payload, err := common.Marshal(channelAlertBatch{Start: bucket, Instance: node + "-boot", Members: members, Scopes: observations})
		require.NoError(t, err)
		require.NoError(t, redisClient.Set(ctx, key, payload, channelAlertTTL).Err())
	}
	failure := func(bucket int64, ids ...string) channelAlertObservation {
		samples := make([]channelAlertSample, 0, len(ids))
		for index, id := range ids {
			samples = append(samples, channelAlertSample{ID: id, At: bucket*1000 + int64(index+1)*1000, Status: 503, Kind: channelAlertService})
		}
		return channelAlertObservation{Scope: scope, Failures: samples}
	}
	read := func(bucket int64, complete bool) {
		t.Helper()
		round, size, valid := readChannelAlertRound(ctx, nodes, members, bucket, &window)
		require.Equal(t, complete, valid)
		if valid {
			window.append(round, size)
		} else {
			window.reset()
		}
		require.NoError(t, evaluateChannelAlertWindow(ctx, window, nodes, current))
	}
	countEpisodes := func(want int64) {
		t.Helper()
		var count int64
		require.NoError(t, db.Model(&model.ChannelAlertEpisode{}).Count(&count).Error)
		assert.Equal(t, want, count)
	}

	storeBatch("a", start, failure(start, "a1", "a2", "a3"))
	read(start, false)
	countEpisodes(0)
	storeBatch("b", start, channelAlertObservation{Scope: scope, Success: start*1000 + 2400})
	read(start, true)
	countEpisodes(0)

	next := start + 5
	storeBatch("a", next, failure(next, "a4"))
	storeBatch("b", next)
	read(next, true)
	countEpisodes(0)

	// Redis data loss drops the unconfirmed window. Two observations before
	// the gap and one afterward cannot produce an incident.
	missing := start + 10
	storeBatch("a", missing, failure(missing, "a5"))
	read(missing, false)
	next = start + 15
	storeBatch("a", next, failure(next, "a6", "a6"))
	storeBatch("b", next)
	read(next, true)
	countEpisodes(0)
	next = start + 20
	storeBatch("a", next, failure(next, "a7"))
	storeBatch("b", next, failure(next, "b8"))
	read(next, true)
	countEpisodes(1)
	read(next, true)
	countEpisodes(1)

	var episode model.ChannelAlertEpisode
	require.NoError(t, db.First(&episode).Error)
	assert.Equal(t, (start+16)*1000, episode.FirstAt)
	assert.Equal(t, current.UnixMilli(), episode.ConfirmedAt)
	assert.Equal(t, "upstream_service", episode.Kind)

	// Delivery uses the persisted work even when the observation cache vanishes.
	common.RDB = nil
	sent, err := deliverChannelAlertMail("smtp-worker", common.SendEmailContext, time.Now)
	require.NoError(t, err)
	require.True(t, sent)
	mail := requireLocalMail(t, inbox, "Channel outage", "ops@example.com")
	assert.Contains(t, mail.body, "model-a")
	assert.NotContains(t, mail.body, channel.Key)
	var delivery model.ChannelAlertDelivery
	require.NoError(t, db.First(&delivery).Error)
	assert.Equal(t, model.MailDeliveryAccepted, delivery.Status)
	assert.Positive(t, delivery.FirstSubmittedAt)
	assert.GreaterOrEqual(t, delivery.AcceptedAt, delivery.FirstSubmittedAt)
	common.RDB = redisClient

	next = start + 25
	storeBatch("a", next, channelAlertObservation{Scope: scope, Success: (next + 1) * 1000})
	storeBatch("b", next)
	read(next, true)
	require.NoError(t, db.First(&episode, episode.ID).Error)
	assert.Nil(t, episode.OpenKey)
	assert.Equal(t, (next+1)*1000, episode.ClosedAt)
	countEpisodes(1)

	// Evict a batch already read into the current window, then present a
	// complete next round. The reader must drop the previous observations.
	next = start + 30
	storeBatch("a", next, failure(next, "b1", "b2"))
	storeBatch("b", next)
	read(next, true)
	require.NoError(t, redisClient.Del(ctx, channelAlertBatchKey(members, "b", next)).Err())
	next = start + 35
	storeBatch("a", next, failure(next, "b3"))
	storeBatch("b", next)
	read(next, true)
	countEpisodes(1)
}

func TestChannelAlertPersistenceDialects(t *testing.T) {
	for _, dialect := range []string{"sqlite", "mysql", "postgres"} {
		t.Run(dialect, func(t *testing.T) {
			db := channelAlertDialectDB(t, dialect)
			ctx := context.Background()
			require.NoError(t, db.AutoMigrate(&model.Channel{}, &model.ChannelAlertEpisode{}, &model.ChannelAlertDelivery{}))
			channel := model.Channel{Name: "alert-dialect", Key: "old-fixture"}
			require.NoError(t, db.Create(&channel).Error)
			key := channelAlertDigest(dialect, common.NewRequestId())
			event := model.ChannelAlertEpisode{ChannelID: channel.Id, ScopeKey: key, OpenKey: &key,
				Revision: model.ChannelAlertRevision(&channel), FirstAt: 1000, LastAt: 3000, ConfirmedAt: 3500, KeySlot: -1}
			created, err := model.OpenChannelAlert(ctx, event, []string{"ops@example.com"})
			require.NoError(t, err)
			require.True(t, created)
			created, err = model.OpenChannelAlert(ctx, event, []string{"ops@example.com"})
			require.NoError(t, err)
			require.False(t, created)
			row, err := model.ClaimChannelAlertDelivery(ctx, "lease-owner", 3600, 4500)
			require.NoError(t, err)
			require.NotNil(t, row)
			require.NoError(t, model.ExpireChannelAlertLeases(ctx, 4500))
			next, err := model.ClaimChannelAlertDelivery(ctx, "other-owner", 4501, 5000)
			require.NoError(t, err)
			require.Nil(t, next)
			var unknown model.ChannelAlertDelivery
			require.NoError(t, db.First(&unknown, row.ID).Error)
			assert.Equal(t, model.MailDeliveryUnknown, unknown.Status)
			require.NoError(t, model.CloseChannelAlert(ctx, row.EpisodeID, 5000, model.ChannelAlertClosedRecovered))
			var closed model.ChannelAlertEpisode
			require.NoError(t, db.First(&closed, row.EpisodeID).Error)
			assert.Equal(t, model.ChannelAlertClosedRecovered, closed.CloseReason)
			created, err = model.OpenChannelAlert(ctx, event, []string{"ops@example.com"})
			require.NoError(t, err)
			require.False(t, created)
			require.NoError(t, db.Model(&channel).Update("key", "new-fixture").Error)
			event.FirstAt, event.LastAt = 6000, 8000
			created, err = model.OpenChannelAlert(ctx, event, nil)
			require.NoError(t, err)
			require.False(t, created)
		})
	}
}

func TestChannelAlertDeliveryRetryAfterRestart(t *testing.T) {
	db := withErrorReportTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.Channel{}, &model.ChannelAlertEpisode{}, &model.ChannelAlertDelivery{}))
	require.NoError(t, db.Create(&model.Option{Key: "error_report_setting.recipients", Value: "ops@example.com"}).Error)
	inbox := newLocalMailInbox(t)
	ctx := context.Background()
	channel := model.Channel{Name: "mail-restart", Key: "fixture"}
	require.NoError(t, db.Create(&channel).Error)
	now := time.Now()
	key := channelAlertDigest(t.Name())
	episode := model.ChannelAlertEpisode{ChannelID: channel.Id, ScopeKey: key, OpenKey: &key,
		Revision: model.ChannelAlertRevision(&channel), Kind: string(channelAlertService), KeySlot: -1,
		FirstAt: now.Add(-time.Second).UnixMilli(), LastAt: now.Add(-time.Millisecond).UnixMilli(), ConfirmedAt: now.UnixMilli()}
	created, err := model.OpenChannelAlert(ctx, episode, []string{"ops@example.com"})
	require.NoError(t, err)
	require.True(t, created)
	processed, err := deliverChannelAlertMail("busy-transport", func(context.Context, string, string, string) error {
		return common.ErrMailBusy
	}, func() time.Time { return now })
	require.NoError(t, err)
	require.True(t, processed)
	var pending model.ChannelAlertDelivery
	require.NoError(t, db.First(&pending).Error)
	assert.Equal(t, model.MailDeliveryPending, pending.Status)
	assert.Zero(t, pending.Attempts)
	assert.Zero(t, pending.FirstSubmittedAt)
	assert.Less(t, pending.NextAttemptAt, now.Add(time.Minute).UnixMilli())
	now = time.UnixMilli(pending.NextAttemptAt)
	processed, err = deliverChannelAlertMail("before-restart", func(context.Context, string, string, string) error {
		return fmt.Errorf("fixture connection refused before submission")
	}, func() time.Time { return now })
	require.NoError(t, err)
	require.True(t, processed)
	require.NoError(t, db.First(&pending, pending.ID).Error)
	assert.Equal(t, model.MailDeliveryRetryWait, pending.Status)
	assert.Zero(t, pending.FirstSubmittedAt)
	assert.Greater(t, pending.NextAttemptAt, now.UnixMilli())
	processed, err = deliverChannelAlertMail("after-restart", common.SendEmailContext, func() time.Time {
		return time.UnixMilli(pending.NextAttemptAt)
	})
	require.NoError(t, err)
	require.True(t, processed)
	requireLocalMail(t, inbox, "Channel outage", "ops@example.com")
	require.NoError(t, db.First(&pending, pending.ID).Error)
	assert.Equal(t, model.MailDeliveryAccepted, pending.Status)
	assert.Equal(t, 2, pending.Attempts)
	assert.Positive(t, pending.FirstSubmittedAt)
}

func channelAlertDialectDB(t *testing.T, dialect string) *gorm.DB {
	t.Helper()
	if dialect == "sqlite" {
		return withErrorReportTestDB(t)
	}
	dsn := os.Getenv("CHANNEL_ALERT_TEST_" + map[string]string{"mysql": "MYSQL", "postgres": "POSTGRES"}[dialect])
	if dsn == "" {
		t.Skipf("configure an isolated %s fixture", dialect)
	}
	var dialector gorm.Dialector
	var dbType common.DatabaseType
	switch dialect {
	case "mysql":
		dialector, dbType = mysql.Open(dsn), common.DatabaseTypeMySQL
	case "postgres":
		dialector, dbType = postgres.Open(dsn), common.DatabaseTypePostgreSQL
	}
	db, err := gorm.Open(dialector, &gorm.Config{})
	require.NoError(t, err)
	old, oldType := model.DB, common.MainDatabaseType()
	model.DB = db
	common.SetMainDatabaseType(dbType)
	t.Cleanup(func() {
		model.DB = old
		common.SetMainDatabaseType(oldType)
		conn, err := db.DB()
		if err == nil {
			_ = conn.Close()
		}
	})
	return db
}

func TestChannelAlertCandidateUsesIndependentCurrentEvidence(t *testing.T) {
	group := channelAlertGroup{scope: channelAlertScope{Channel: 1, Model: "fixture", Operation: "video_submit", Slot: -1}, kind: channelAlertService}
	samples := []channelAlertSample{{ID: "a", At: 1000}, {ID: "a", At: 2000}, {ID: "b", At: 3000}, {ID: "c", At: 4000}}
	_, confirmed := channelAlertCandidate(group, samples[:3], 0)
	assert.False(t, confirmed)
	_, confirmed = channelAlertCandidate(group, samples, 2500)
	assert.False(t, confirmed)
	episode, confirmed := channelAlertCandidate(group, samples, 0)
	require.True(t, confirmed)
	assert.EqualValues(t, 1000, episode.FirstAt)
	assert.EqualValues(t, 4000, episode.LastAt)
	group.kind = channelAlertLimit
	_, confirmed = channelAlertCandidate(group, samples, 0)
	assert.False(t, confirmed)
	_, confirmed = channelAlertCandidate(group, samples, 2500)
	assert.True(t, confirmed)
}

func BenchmarkChannelAlertSuccessfulObservation(b *testing.B) {
	buffer := channelAlertBuffer{buckets: make(map[int64]map[channelAlertScope]*channelAlertObservation), dropped: make(map[int64]bool)}
	scopes := make([]channelAlertScope, 50)
	for i := range scopes {
		scopes[i] = channelAlertScope{Channel: i + 1, Revision: "fixture-revision", Model: fmt.Sprintf("model-%d", i), Operation: "chat", Slot: -1}
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		buffer.add(scopes[i%len(scopes)], channelAlertSample{})
	}
}

func TestChannelAlertMultiKeyScope(t *testing.T) {
	db := withErrorReportTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.Channel{}, &model.ChannelAlertEpisode{}, &model.ChannelAlertDelivery{}))
	require.NoError(t, model.UpdateOption("error_report_setting.recipients", "ops@example.com"))
	channel := model.Channel{Name: "multi-key", Models: "model-a", Key: "first-fixture\nsecond-fixture", ChannelInfo: model.ChannelInfo{IsMultiKey: true}}
	require.NoError(t, db.Create(&channel).Error)
	scope := channelAlertScope{Channel: channel.Id, Revision: model.ChannelAlertRevision(&channel), Model: "model-a", Operation: "video_submit", Slot: -1}
	first, second := scope, scope
	first.Slot, second.Slot = 0, 1
	now := time.Unix(1005, 0)
	window := channelAlertWindowState{}
	window.append(channelAlertRound{start: 1000, batches: []channelAlertBatch{{Instance: "boot", Scopes: []channelAlertObservation{
		{Scope: first, Failures: []channelAlertSample{{ID: "first", At: 1001000, Kind: channelAlertKey, Status: 401}}},
	}}}}, 256)
	require.NoError(t, evaluateChannelAlertWindow(context.Background(), window, []string{"node"}, now))
	var events []model.ChannelAlertEpisode
	require.NoError(t, db.Find(&events).Error)
	assert.Empty(t, events)

	window.append(channelAlertRound{start: 1005, batches: []channelAlertBatch{{Instance: "boot", Scopes: []channelAlertObservation{
		{Scope: second, Success: 1006000},
	}}}}, 256)
	require.NoError(t, evaluateChannelAlertWindow(context.Background(), window, []string{"node"}, now.Add(5*time.Second)))
	require.NoError(t, db.Order("id").Find(&events).Error)
	require.Len(t, events, 1)
	assert.Equal(t, 0, events[0].KeySlot)
	assert.False(t, events[0].Preview)

	channel.ChannelInfo.MultiKeyStatusList = map[int]int{0: common.ChannelStatusAutoDisabled, 1: common.ChannelStatusAutoDisabled}
	require.NoError(t, db.Model(&channel).Update("channel_info", channel.ChannelInfo).Error)
	window.append(channelAlertRound{start: 1010, batches: []channelAlertBatch{{Instance: "boot", Scopes: []channelAlertObservation{
		{Scope: second, Failures: []channelAlertSample{{ID: "second", At: 1011000, Kind: channelAlertKey, Status: 401}}},
	}}}}, 256)
	require.NoError(t, evaluateChannelAlertWindow(context.Background(), window, []string{"node"}, now.Add(10*time.Second)))
	require.NoError(t, db.Order("id").Find(&events).Error)
	require.Len(t, events, 2)
	assert.Equal(t, -1, events[1].KeySlot)
	assert.Equal(t, 2, events[1].EvidenceCount)
}

func TestChannelAlertRelayIgnoresLocalAcceptanceAndPreservesUpstreamStatus(t *testing.T) {
	previous := channelAlerts.enabled.Load()
	channelAlerts.buffer.buckets = make(map[int64]map[channelAlertScope]*channelAlertObservation)
	channelAlerts.buffer.dropped = make(map[int64]bool)
	channelAlerts.buffer.sealed = 0
	channelAlerts.enabled.Store(true)
	t.Cleanup(func() { channelAlerts.enabled.Store(previous) })

	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/images/generations", nil)
	c.Set(common.RequestIdKey, "synthetic-id")
	channel := &model.Channel{Id: 4, Key: "synthetic"}
	info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{ChannelId: 4}, OriginModelName: "image", RelayMode: relaymode.RelayModeImagesGenerations}
	SetChannelAlertSelection(c, channel, 0)
	BeginChannelAlertAttempt(c)
	ObserveChannelRelayResult(c, info, nil)
	assert.Empty(t, channelAlerts.buffer.buckets, "local task acceptance has no upstream result")

	RecordChannelAlertHTTP(c, &http.Response{StatusCode: http.StatusTooManyRequests}, nil)
	ObserveChannelRelayResult(c, info, types.NewOpenAIError(fmt.Errorf("mapped"), types.ErrorCodeBadResponseStatusCode, http.StatusServiceUnavailable))
	rows, ok := channelAlerts.buffer.seal(channelAlertBucket(time.Now().Unix()))
	require.True(t, ok)
	require.Len(t, rows, 1)
	require.Len(t, rows[0].Failures, 1)
	assert.Equal(t, channelAlertLimit, rows[0].Failures[0].Kind)
	assert.Equal(t, http.StatusTooManyRequests, rows[0].Failures[0].Status)

	// A caller can disconnect after receiving a completed response, while
	// quota accounting is still finishing in the handler.
	c, _ = gin.CreateTestContext(httptest.NewRecorder())
	requestContext, cancel := context.WithCancel(context.Background())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/images/generations", nil).WithContext(requestContext)
	SetChannelAlertSelection(c, channel, 0)
	BeginChannelAlertAttempt(c)
	RecordChannelAlertHTTP(c, &http.Response{StatusCode: http.StatusOK}, nil)
	cancel()
	channelAlerts.buffer.sealed = 0
	ObserveChannelRelayResult(c, info, nil)
	rows, ok = channelAlerts.buffer.seal(channelAlertBucket(time.Now().Unix()))
	require.True(t, ok)
	require.Len(t, rows, 1)
	assert.Positive(t, rows[0].Success)
}

func BenchmarkChannelAlertRelayHooks(b *testing.B) {
	for _, enabled := range []bool{false, true} {
		b.Run(fmt.Sprintf("enabled=%t", enabled), func(b *testing.B) {
			previous := channelAlerts.enabled.Load()
			channelAlerts.buffer.buckets = make(map[int64]map[channelAlertScope]*channelAlertObservation)
			channelAlerts.buffer.dropped = make(map[int64]bool)
			channelAlerts.buffer.sealed = 0
			channelAlerts.enabled.Store(enabled)
			b.Cleanup(func() { channelAlerts.enabled.Store(previous) })
			channel := &model.Channel{Id: 5, Key: "synthetic", Models: "model-a"}
			relay := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{ChannelId: 5}, OriginModelName: "model-a", RelayMode: relaymode.RelayModeChatCompletions}
			request := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
			response := &http.Response{StatusCode: http.StatusOK}
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				c := &gin.Context{Request: request}
				SetChannelAlertSelection(c, channel, 0)
				BeginChannelAlertAttempt(c)
				RecordChannelAlertHTTP(c, response, nil)
				ObserveChannelRelayResult(c, relay, nil)
			}
		})
	}
}
