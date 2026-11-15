package controller

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/i18n"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestChannelFailoverRelay(t *testing.T) {
	for _, dialect := range []struct{ kind, env string }{{"sqlite", ""}, {"mysql", "TEST_MYSQL_DSN"}, {"postgres", "TEST_POSTGRES_DSN"}} {
		t.Run(dialect.kind, func(t *testing.T) {
			dsn := os.Getenv(dialect.env)
			if dialect.env != "" && dsn == "" {
				t.Skip("external database fixture unavailable")
			}
			testChannelFailoverRelay(t, dialect.kind, dsn)
		})
	}
}

func testChannelFailoverRelay(t *testing.T, kind, dsn string) {
	gin.SetMode(gin.TestMode)
	require.NoError(t, i18n.Init())
	service.InitHttpClient()
	previousTimeout := constant.StreamingTimeout
	constant.StreamingTimeout = 5
	t.Cleanup(func() { constant.StreamingTimeout = previousTimeout })
	db := modelManagementDB(t, kind, dsn)
	require.NoError(t, db.AutoMigrate(&model.Token{}, &model.Log{}))
	require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(`{"claude-failover-test":1}`))
	require.NoError(t, db.Create(&model.User{Id: 19, Username: "failover-test", Status: common.UserStatusEnabled, Group: "default", Quota: 10000000}).Error)
	token := model.Token{UserId: 19, Key: "channelfailoverlocaltest123456789012345", Name: "test", Status: common.TokenStatusEnabled, UnlimitedQuota: true, Group: "default", ExpiredTime: -1}
	require.NoError(t, db.Create(&token).Error)
	previous := model.CurrentRequestPolicy().Options
	t.Cleanup(func() { require.NoError(t, model.UpdateRequestPolicyOptions(previous)) })
	engine := gin.New()
	engine.POST("/v1/messages", middleware.TokenAuth(), middleware.Distribute(), func(c *gin.Context) {
		Relay(c, types.RelayFormatClaude)
	})

	for _, cached := range []bool{false, true} {
		t.Run(fmt.Sprintf("memory=%t", cached), func(t *testing.T) {
			common.MemoryCacheEnabled = cached
			var status, aCalls, bCalls, cCalls atomic.Int32
			var truncated, backupDown atomic.Bool
			status.Store(200)
			upstream := func(calls *atomic.Int32, primary, backup bool) *httptest.Server {
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
					calls.Add(1)
					if code := int(status.Load()); (primary || backup && backupDown.Load()) && code != 200 {
						w.WriteHeader(code)
						_, _ = fmt.Fprint(w, `{"type":"error","error":{"type":"api_error","message":"test upstream unavailable"}}`)
						return
					}
					if primary && truncated.Load() {
						w.Header().Set("Content-Type", "text/event-stream")
						_, _ = fmt.Fprint(w, "event: message_start\ndata: {\"type\":\"message_start\",\"message\":{\"id\":\"msg_partial\",\"type\":\"message\",\"role\":\"assistant\",\"model\":\"claude-failover-test\",\"usage\":{\"input_tokens\":2,\"output_tokens\":0}}}\n\n")
						return
					}
					w.Header().Set("Content-Type", "application/json")
					_, _ = fmt.Fprint(w, `{"id":"msg_test","type":"message","role":"assistant","model":"claude-failover-test","content":[{"type":"text","text":"ok"}],"stop_reason":"end_turn","usage":{"input_tokens":2,"output_tokens":1}}`)
				}))
				t.Cleanup(server.Close)
				return server
			}
			servers := []*httptest.Server{upstream(&aCalls, true, false), upstream(&bCalls, false, true), upstream(&cCalls, false, false)}
			channels := make([]model.Channel, len(servers))
			for i, server := range servers {
				priority := int64(10)
				if i == 2 {
					priority = 5
				}
				channel := model.Channel{Name: fmt.Sprintf("failover-%d", i), Key: "fixture-key", Type: constant.ChannelTypeAnthropic,
					Status: common.ChannelStatusEnabled, Models: "claude-failover-test", Group: "default", BaseURL: &server.URL, Priority: &priority}
				require.NoError(t, db.Create(&channel).Error)
				require.NoError(t, channel.AddAbilities(db))
				channels[i] = channel
			}
			t.Cleanup(func() {
				for _, channel := range channels {
					require.NoError(t, db.Where("channel_id = ?", channel.Id).Delete(&model.Ability{}).Error)
					require.NoError(t, db.Delete(&channel).Error)
				}
			})
			model.InitChannelCache()
			options := func(mode, budget, sessionMode string, allow bool) {
				rules := fmt.Sprintf(`[{"name":"claude cli trace","model_regex":["^claude-.*$"],"key_sources":[{"type":"request_header","key":"X-Session"}],"session_mode":%q,"include_rule_name":true,"failover_on_unavailable":%t},{"name":"codex cli trace"}]`, sessionMode, allow)
				require.NoError(t, model.UpdateRequestPolicyOptions(map[string]string{
					"channel_affinity_setting.enabled":              "true",
					"channel_affinity_setting.rules":                rules,
					"channel_affinity_setting.failover_mode":        mode,
					"channel_affinity_setting.failover_channel_ids": fmt.Sprintf("[%d]", channels[0].Id),
					"RetryTimes": budget,
				}))
			}
			send := func(session string, stream bool) *httptest.ResponseRecorder {
				req := httptest.NewRequest(http.MethodPost, "/v1/messages", strings.NewReader(fmt.Sprintf(`{"model":"claude-failover-test","max_tokens":20,"messages":[{"role":"user","content":"hello"}],"stream":%t}`, stream)))
				req.Header.Set("Authorization", "Bearer sk-"+token.Key)
				req.Header.Set("Content-Type", "application/json")
				req.Header.Set("X-Session", strings.ReplaceAll(t.Name(), "=", "-")+session)
				recorder := httptest.NewRecorder()
				engine.ServeHTTP(recorder, req)
				return recorder
			}
			bind := func(session string) {
				c := newPinRetryContext()
				c.Request.Header.Set("X-Session", strings.ReplaceAll(t.Name(), "=", "-")+session)
				service.GetPreferredChannelByAffinity(c, "claude-failover-test", "default")
				c.Set("channel_id", channels[0].Id)
				service.RecordChannelAffinity(c, channels[0].Id)
				id, found := service.GetPreferredChannelByAffinity(c, "claude-failover-test", "default")
				require.True(t, found, "seed session rule mode %s", service.RequestPolicy(c).SessionMode)
				require.Equal(t, channels[0].Id, id)
			}
			options("enabled", "5", "strict", false)
			bind("strict")
			status.Store(502)
			assert.Equal(t, 502, send("strict", false).Code)
			assert.Equal(t, 1, int(aCalls.Load()))
			assert.Zero(t, int(bCalls.Load()))

			options("enabled", "0", "strict", true)
			assert.Equal(t, 502, send("strict", false).Code)
			assert.Equal(t, 502, send("strict", false).Code)
			assert.Equal(t, 200, send("strict", false).Code, "the next first attempt can bypass with zero retry budget")
			assert.Equal(t, 3, int(aCalls.Load()))
			assert.Equal(t, 1, int(bCalls.Load()))
			assert.Zero(t, int(cCalls.Load()))

			options("off", "5", "strict", true)
			bind("rollback")
			assert.Equal(t, 502, send("rollback", false).Code)
			assert.Equal(t, 4, int(aCalls.Load()))
			assert.Equal(t, 1, int(bCalls.Load()))

			// Changing effective settings must invalidate the old cooldown.
			require.NoError(t, db.Model(&channels[0]).Update("param_override", `{"temperature":0}`).Error)
			model.InitChannelCache()
			options("enabled", "5", "prefer", false)
			bind("prefer")
			assert.Equal(t, 200, send("prefer", false).Code)
			assert.Equal(t, 5, int(aCalls.Load()))
			assert.Equal(t, 2, int(bCalls.Load()), "a supplement first tries the remaining channel in the original tier")
			assert.Zero(t, int(cCalls.Load()))
			assert.Equal(t, 200, send("prefer", false).Code)
			assert.Equal(t, 5, int(aCalls.Load()), "a complete backup response updates the binding")

			options("enabled", "5", "strict", true)
			bind("threshold")
			assert.Equal(t, 502, send("threshold", false).Code)
			assert.Equal(t, 200, send("threshold", false).Code, "the threshold request can supplement within its retry budget")
			assert.Equal(t, 7, int(aCalls.Load()))
			assert.Equal(t, 4, int(bCalls.Load()))
			assert.Zero(t, int(cCalls.Load()))

			// When the same-tier backup fails too, a prefer session continues
			// with its remaining budget and reaches the lower tier.
			require.NoError(t, db.Model(&channels[0]).Update("param_override", `{"temperature":0.5}`).Error)
			model.InitChannelCache()
			options("enabled", "5", "prefer", false)
			bind("cascade")
			backupDown.Store(true)
			assert.Equal(t, 200, send("cascade", false).Code)
			backupDown.Store(false)
			assert.Equal(t, 8, int(aCalls.Load()))
			assert.Equal(t, 5, int(bCalls.Load()))
			assert.Equal(t, 1, int(cCalls.Load()))

			status.Store(200)
			truncated.Store(true)
			require.NoError(t, db.Model(&channels[0]).Update("param_override", `{"temperature":1}`).Error)
			for _, channel := range channels[1:] {
				require.NoError(t, db.Model(&model.Channel{}).Where("id = ?", channel.Id).Update("status", common.ChannelStatusManuallyDisabled).Error)
				require.NoError(t, db.Model(&model.Ability{}).Where("channel_id = ?", channel.Id).Update("enabled", false).Error)
			}
			model.InitChannelCache()
			assert.Equal(t, 200, send("truncated", true).Code)
			check := newPinRetryContext()
			check.Request.Header.Set("X-Session", strings.ReplaceAll(t.Name(), "=", "-")+"truncated")
			_, found := service.GetPreferredChannelByAffinity(check, "claude-failover-test", "default")
			assert.False(t, found, "a 200 stream without message_stop cannot establish a binding")
		})
	}
}

// Failover searches eligible backups before falling back to an ordinary retry.
func TestChannelFailoverChoosesBackupBeforeOrdinaryRetry(t *testing.T) {
	gin.SetMode(gin.TestMode)
	require.NoError(t, i18n.Init())
	service.InitHttpClient()
	db := modelManagementDB(t, "sqlite", "")
	require.NoError(t, db.AutoMigrate(&model.Token{}, &model.Log{}))
	require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(`{"failover-scope-test":1}`))
	require.NoError(t, db.Create(&model.User{Id: 19, Username: "failover-scope", Status: common.UserStatusEnabled, Group: "default", Quota: 10000000}).Error)
	token := model.Token{UserId: 19, Key: "failoverscopelocaltest12345678901234567", Name: "test", Status: common.TokenStatusEnabled, UnlimitedQuota: true, Group: "default", ExpiredTime: -1}
	require.NoError(t, db.Create(&token).Error)
	previous := model.CurrentRequestPolicy().Options
	t.Cleanup(func() { require.NoError(t, model.UpdateRequestPolicyOptions(previous)) })
	previousMemory := common.MemoryCacheEnabled
	t.Cleanup(func() { common.MemoryCacheEnabled = previousMemory })
	previousGroupRatios := ratio_setting.GroupRatio2JSONString()
	previousUsableGroups := setting.UserUsableGroups2JSONString()
	previousMaxAutoGroups := strconv.Itoa(setting.GetMaxTokenAutoGroups())
	t.Cleanup(func() {
		require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(previousGroupRatios))
		require.NoError(t, setting.UpdateUserUsableGroupsByJSONString(previousUsableGroups))
		require.NoError(t, setting.UpdateMaxTokenAutoGroups(previousMaxAutoGroups))
	})
	require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(`{"default":1,"backup":1}`))
	require.NoError(t, setting.UpdateUserUsableGroupsByJSONString(`{"auto":"Auto","default":"Default","backup":"Backup"}`))
	require.NoError(t, setting.UpdateMaxTokenAutoGroups("2"))
	engine := gin.New()
	engine.POST("/v1/messages", middleware.TokenAuth(), middleware.Distribute(), func(c *gin.Context) {
		Relay(c, types.RelayFormatClaude)
	})
	const passThrough = `{"pass_through_body_enabled":true}`

	for _, tc := range []struct {
		name, sourceSetting, backupSetting, backupGroup string
		memoryCache                                     bool
	}{
		{name: "source with body pass-through", sourceSetting: passThrough, backupGroup: "default", memoryCache: true},
		{name: "backup with body pass-through", backupSetting: passThrough, backupGroup: "default", memoryCache: true},
		{name: "auto finds a healthy later group with one retry from memory", backupGroup: "backup", memoryCache: true},
		{name: "auto finds a healthy later group with one retry from database", backupGroup: "backup"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			common.MemoryCacheEnabled = tc.memoryCache
			retryTimes := "3"
			tokenRouting := map[string]any{"group": "default", "auto_groups": "", "cross_group_retry": false}
			if tc.backupGroup != "default" {
				retryTimes = "1"
				tokenRouting = map[string]any{"group": "auto", "auto_groups": `["default","backup"]`, "cross_group_retry": true}
			}
			require.NoError(t, db.Model(&token).Updates(tokenRouting).Error)
			var sourceCalls, backupCalls atomic.Int32
			create := func(name, group string, priority int64, status int, setting string, calls *atomic.Int32) model.Channel {
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
					calls.Add(1)
					if status != http.StatusOK {
						w.WriteHeader(status)
						_, _ = fmt.Fprint(w, `{"type":"error","error":{"type":"api_error","message":"test upstream unavailable"}}`)
						return
					}
					w.Header().Set("Content-Type", "application/json")
					_, _ = fmt.Fprint(w, `{"id":"msg_test","type":"message","role":"assistant","model":"failover-scope-test","content":[{"type":"text","text":"ok"}],"stop_reason":"end_turn","usage":{"input_tokens":2,"output_tokens":1}}`)
				}))
				t.Cleanup(server.Close)
				channel := model.Channel{Name: name, Key: "fixture-key", Type: constant.ChannelTypeAnthropic, Status: common.ChannelStatusEnabled,
					Models: "failover-scope-test", Group: group, BaseURL: &server.URL, Priority: &priority}
				if setting != "" {
					channel.Setting = &setting
				}
				require.NoError(t, db.Create(&channel).Error)
				require.NoError(t, channel.AddAbilities(db))
				t.Cleanup(func() {
					require.NoError(t, db.Where("channel_id = ?", channel.Id).Delete(&model.Ability{}).Error)
					require.NoError(t, db.Delete(&channel).Error)
					model.InitChannelCache()
				})
				return channel
			}
			source := create("source", "default", 10, http.StatusBadGateway, tc.sourceSetting, &sourceCalls)
			create("backup", tc.backupGroup, 0, http.StatusOK, tc.backupSetting, &backupCalls)
			model.InitChannelCache()
			require.NoError(t, model.UpdateRequestPolicyOptions(map[string]string{
				"channel_affinity_setting.enabled":              "true",
				"channel_affinity_setting.rules":                `[{"name":"codex cli trace"},{"name":"claude cli trace"}]`,
				"channel_affinity_setting.failover_mode":        "enabled",
				"channel_affinity_setting.failover_channel_ids": fmt.Sprintf("[%d]", source.Id),
				"RetryTimes": retryTimes,
			}))
			req := httptest.NewRequest(http.MethodPost, "/v1/messages", strings.NewReader(`{"model":"failover-scope-test","max_tokens":20,"messages":[{"role":"user","content":"hello"}]}`))
			req.Header.Set("Authorization", "Bearer sk-"+token.Key)
			req.Header.Set("Content-Type", "application/json")
			recorder := httptest.NewRecorder()
			engine.ServeHTTP(recorder, req)
			assert.Equal(t, http.StatusOK, recorder.Code)
			assert.Equal(t, 1, int(sourceCalls.Load()))
			assert.Equal(t, 1, int(backupCalls.Load()))
		})
	}
}
