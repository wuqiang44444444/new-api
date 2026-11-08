package router

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/config"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/QuantumNous/new-api/setting/system_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Exercise the registered production route, rather than adding the middleware
// only to a test router (which concealed the missing production connection).
func TestRegisteredImageRouteAsyncAdmissionAndIdempotency(t *testing.T) {
	for _, pricing := range []string{"per_call", "legacy_tokens"} {
		t.Run(pricing, func(t *testing.T) {
			setupRelayRouterTestDB(t)
			oldCache, oldCount := common.MemoryCacheEnabled, constant.CountToken
			common.MemoryCacheEnabled, constant.CountToken = false, false
			t.Cleanup(func() { common.MemoryCacheEnabled, constant.CountToken = oldCache, oldCount })
			saved := map[string]string{}
			require.NoError(t, config.GlobalConfig.SaveToDB(func(k, v string) error { saved[k] = v; return nil }))
			t.Cleanup(func() { require.NoError(t, config.GlobalConfig.LoadFromDB(saved)) })
			require.NoError(t, config.GlobalConfig.LoadFromDB(map[string]string{
				"billing_setting.billing_mode": `{}`, "quota_setting.trust_quota_usd": "0",
				"quota_setting.pre_consume_multiplier": "1",
			}))
			oldPrices, oldRatios, oldGroups := ratio_setting.ModelPrice2JSONString(), ratio_setting.ModelRatio2JSONString(), ratio_setting.GroupRatio2JSONString()
			t.Cleanup(func() {
				require.NoError(t, ratio_setting.UpdateModelPriceByJSONString(oldPrices))
				require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(oldRatios))
				require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(oldGroups))
			})
			require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(`{"default":1}`))
			require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(`{"gpt-image-2":2.5}`))
			prices := `{}`
			if pricing == "per_call" {
				prices = `{"gpt-image-2":0.04}`
			}
			require.NoError(t, ratio_setting.UpdateModelPriceByJSONString(prices))
			objects := map[string][]byte{}
			var mu sync.Mutex
			store := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				mu.Lock()
				defer mu.Unlock()
				if r.Method == http.MethodPut {
					body, err := io.ReadAll(r.Body)
					assert.NoError(t, err)
					objects[r.URL.Path] = body
					return
				}
				body, found := objects[r.URL.Path]
				if !found {
					w.WriteHeader(http.StatusNotFound)
					return
				}
				w.Header().Set("Content-Length", strconv.Itoa(len(body)))
				if r.Method == http.MethodGet {
					_, _ = w.Write(body)
				}
			}))
			defer store.Close()
			raw, err := common.Marshal(system_setting.ObjectStorageConfig{Backend: "s3", Endpoint: store.URL, Bucket: "images", AccountName: "fixture", Credential: "fixture", Region: "us-east-1", Revision: pricing})
			require.NoError(t, err)
			model.NotifyObjectStorageSettingUpdate(string(raw))
			t.Cleanup(func() { model.NotifyObjectStorageSettingUpdate("") })
			service.InitHttpClient()
			user := model.User{Username: "image-route-" + pricing, Status: common.UserStatusEnabled, Group: "default", Quota: 1000000}
			require.NoError(t, model.DB.Create(&user).Error)
			tokens := []model.Token{
				{UserId: user.Id, Key: strings.Repeat("a", 32), Status: common.TokenStatusEnabled, ExpiredTime: -1, RemainQuota: 1000000, Group: "default"},
				{UserId: user.Id, Key: strings.Repeat("b", 32), Status: common.TokenStatusEnabled, ExpiredTime: -1, RemainQuota: 1000000, Group: "default"},
			}
			require.NoError(t, model.DB.Create(&tokens).Error)
			channel := model.Channel{Type: constant.ChannelTypeAzure, Name: "image-fixture", Key: "upstream-fixture", Status: common.ChannelStatusEnabled, Models: "gpt-image-2", Group: "default", BaseURL: &store.URL}
			require.NoError(t, model.DB.Create(&channel).Error)
			require.NoError(t, model.DB.Create(&model.Ability{Group: "default", Model: "gpt-image-2", ChannelId: channel.Id, Enabled: true}).Error)
			engine := gin.New()
			SetTaskPluginProtocolRouter(engine)
			request := func(token model.Token, prompt string, async bool) *httptest.ResponseRecorder {
				body, err := common.Marshal(map[string]any{"model": "gpt-image-2", "prompt": prompt, "n": 1})
				require.NoError(t, err)
				r := httptest.NewRequest(http.MethodPost, "/v1/images/generations", bytes.NewReader(body))
				r.Header.Set("Authorization", "Bearer sk-"+token.Key)
				r.Header.Set("Content-Type", "application/json")
				r.Header.Set("Idempotency-Key", "same-key")
				if async {
					r.Header.Set("Prefer", "respond-async")
				}
				w := httptest.NewRecorder()
				engine.ServeHTTP(w, r)
				return w
			}
			assert.Equal(t, http.StatusBadRequest, request(tokens[0], "draw", false).Code)
			first := request(tokens[0], "draw", true)
			require.Equal(t, http.StatusAccepted, first.Code, first.Body.String())
			var accepted struct {
				ID string `json:"id"`
			}
			require.NoError(t, common.Unmarshal(first.Body.Bytes(), &accepted))
			require.NotEmpty(t, accepted.ID)
			var held model.User
			require.NoError(t, model.DB.First(&held, user.Id).Error)
			replay := request(tokens[0], "draw", true)
			require.Equal(t, http.StatusAccepted, replay.Code, replay.Body.String())
			var repeated struct {
				ID string `json:"id"`
			}
			require.NoError(t, common.Unmarshal(replay.Body.Bytes(), &repeated))
			assert.Equal(t, accepted.ID, repeated.ID)
			assert.Equal(t, http.StatusConflict, request(tokens[0], "changed", true).Code)
			var unchanged model.User
			require.NoError(t, model.DB.First(&unchanged, user.Id).Error)
			assert.Equal(t, held.Quota, unchanged.Quota, "replay and conflict must not hold funds again")
			otherApp := request(tokens[1], "draw", true)
			require.Equal(t, http.StatusAccepted, otherApp.Code, otherApp.Body.String())
			var tasks []model.Task
			require.NoError(t, model.DB.Where("user_id = ?", user.Id).Find(&tasks).Error)
			require.Len(t, tasks, 2)
			assert.NotEqual(t, tasks[0].AppID, tasks[1].AppID)
			for _, task := range tasks {
				if pricing == "per_call" {
					assert.Equal(t, 20000, task.Quota, strconv.Itoa(task.AppID))
				}
				assert.Equal(t, task.Quota, task.PrivateData.BillingContext.InitialCalculation.Quota)
			}
		})
	}
}
