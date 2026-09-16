package middleware

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	logtest "github.com/QuantumNous/new-api/clienterrlog/testutil"
	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/i18n"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupAuthClientErrorDB(t *testing.T) {
	t.Helper()
	previousDB, previousPath, previousMaster := model.DB, common.SQLitePath, common.IsMasterNode
	previousMainType, previousLogType := common.MainDatabaseType(), common.LogDatabaseType()
	previousRedis := common.RedisEnabled
	common.RedisEnabled = false
	require.NoError(t, i18n.Init())
	t.Setenv("SQL_DSN", "local")
	t.Setenv("LOG_SQL_DSN", "")
	common.SQLitePath, common.IsMasterNode = "file:auth_client_error_init?mode=memory&cache=shared", true
	require.NoError(t, model.InitDB())
	bootstrap, err := model.DB.DB()
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = bootstrap.Close()
		model.DB, common.SQLitePath, common.IsMasterNode = previousDB, previousPath, previousMaster
		common.SetMainDatabaseType(previousMainType)
		common.SetLogDatabaseType(previousLogType)
		common.RedisEnabled = previousRedis
	})
}

func createAuthClientErrorUser(t *testing.T, username string) *model.User {
	t.Helper()
	user := &model.User{
		Username: username, Password: "password-placeholder", Role: common.RoleCommonUser,
		Status: common.UserStatusEnabled, Group: "default", AuthVersion: 1,
		AffCode: "ace-" + username,
	}
	require.NoError(t, model.DB.Create(user).Error)
	return user
}

func createAuthClientErrorToken(t *testing.T, userID int, key string) *model.Token {
	t.Helper()
	token := model.Token{
		UserId: userID, Key: key, Status: common.TokenStatusEnabled,
		ExpiredTime: -1, RemainQuota: 1000, Group: "default",
	}
	require.NoError(t, model.DB.Create(&token).Error)
	return &token
}

func newAuthClientErrorEngine(buffer *logtest.Capture, handlers ...gin.HandlerFunc) *gin.Engine {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.Use(buffer.Middleware())
	engine.POST("/v1/assets", handlers...)
	return engine
}

// 真实 TokenAuth 成功放行后的 400 必须记录为 api_token 身份事件。
func TestTokenAuthMarksAuthPassedAnd4xxIsRecorded(t *testing.T) {
	setupAuthClientErrorDB(t)
	buffer := logtest.New(t)
	user := createAuthClientErrorUser(t, "ace-token-user")
	token := createAuthClientErrorToken(t, user.Id, "aceclienttoken1")

	handlerReached := false
	engine := newAuthClientErrorEngine(buffer, TokenAuth(), func(c *gin.Context) {
		handlerReached = true
		c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": "invalid asset request"})
	})
	request := httptest.NewRequest(http.MethodPost, "/v1/assets", nil)
	request.Header.Set("Authorization", "Bearer sk-"+token.Key)
	response := httptest.NewRecorder()
	engine.ServeHTTP(response, request)

	require.Equal(t, http.StatusBadRequest, response.Code)
	require.True(t, handlerReached)
	log := buffer.String()
	assert.Equal(t, 1, strings.Count(log, "event=authenticated_api_client_error"), log)
	assert.Contains(t, log, "identity=api_token")
	assert.Contains(t, log, "user_id="+strconv.Itoa(user.Id))
	assert.Contains(t, log, "route=/v1/assets")
}

// 鉴权失败不产生事件；Token 可查到但状态失败时上下文已有 user_id 也不得记录。
func TestTokenAuthFailuresNeverRecordClientError(t *testing.T) {
	setupAuthClientErrorDB(t)
	buffer := logtest.New(t)
	user := createAuthClientErrorUser(t, "ace-fail-user")
	createAuthClientErrorToken(t, user.Id, "aceexhaustedtoken")
	exhausted := model.Token{
		UserId: user.Id, Key: "aceexhaustedtoken2", Status: common.TokenStatusEnabled,
		ExpiredTime: -1, RemainQuota: 0, Group: "default",
	}
	require.NoError(t, model.DB.Create(&exhausted).Error)

	handlerReached := false
	engine := newAuthClientErrorEngine(buffer, TokenAuth(), func(c *gin.Context) {
		handlerReached = true
		c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": "unreachable"})
	})
	for _, key := range []string{"sk-unknownkey", "sk-aceexhaustedtoken2"} {
		request := httptest.NewRequest(http.MethodPost, "/v1/assets", nil)
		request.Header.Set("Authorization", "Bearer "+key)
		response := httptest.NewRecorder()
		engine.ServeHTTP(response, request)
		require.Equal(t, http.StatusUnauthorized, response.Code, key)
	}
	assert.False(t, handlerReached)
	assert.NotContains(t, buffer.String(), "event=authenticated_api_client_error")
}

// 有效 Token 搭配非法指定渠道 ID：SetupContextForToken 在入口内返回 400，
// 成功标记未设置——这是有意排除项，不能误认作日志遗漏。
func TestTokenAuthSpecificChannelRejectIsIntentionallyExcluded(t *testing.T) {
	setupAuthClientErrorDB(t)
	buffer := logtest.New(t)
	admin := &model.User{
		Username: "ace-admin-user", Password: "password-placeholder", Role: common.RoleAdminUser,
		Status: common.UserStatusEnabled, Group: "default", AuthVersion: 1,
		AffCode: "ace-admin",
	}
	require.NoError(t, model.DB.Create(admin).Error)
	token := createAuthClientErrorToken(t, admin.Id, "acespecifictoken1")

	handlerReached := false
	engine := newAuthClientErrorEngine(buffer, TokenAuth(), func(c *gin.Context) {
		handlerReached = true
	})
	request := httptest.NewRequest(http.MethodPost, "/v1/assets", nil)
	request.Header.Set("Authorization", "Bearer sk-"+token.Key+"-notanumber")
	response := httptest.NewRecorder()
	engine.ServeHTTP(response, request)

	require.Equal(t, http.StatusBadRequest, response.Code)
	require.False(t, handlerReached)
	assert.NotContains(t, buffer.String(), "event=authenticated_api_client_error")
}

// TokenAuth 放行后 TokenModelAccess 返回 403：controller 未执行也必须记录。
func TestTokenModelAccess403AfterTokenAuthIsRecorded(t *testing.T) {
	setupAuthClientErrorDB(t)
	buffer := logtest.New(t)
	user := createAuthClientErrorUser(t, "ace-model-user")
	token := createAuthClientErrorToken(t, user.Id, "acemodellimitedtoken")
	token.ModelLimitsEnabled = true
	token.ModelLimits = "other-model"
	require.NoError(t, model.DB.Save(token).Error)

	handlerReached := false
	engine := newAuthClientErrorEngine(buffer, TokenAuth(), TokenModelAccess(), func(c *gin.Context) {
		handlerReached = true
	})
	body := strings.NewReader(`{"model":"customer-model"}`)
	request := httptest.NewRequest(http.MethodPost, "/v1/assets", body)
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", "Bearer sk-"+token.Key)
	response := httptest.NewRecorder()
	engine.ServeHTTP(response, request)

	require.Equal(t, http.StatusForbidden, response.Code)
	require.False(t, handlerReached)
	log := buffer.String()
	assert.Equal(t, 1, strings.Count(log, "event=authenticated_api_client_error"), log)
	assert.Contains(t, log, "stage=model_access")
	assert.Contains(t, log, "reason=token_model_forbidden")
	assert.Contains(t, log, "public_code=token_model_forbidden")
}

// PAT 通过 UserAuth 的共用成功放行点标记；4xx 记录为 personal_access_token 身份。
func TestUserAuthPATMarksAuthPassedAnd4xxIsRecorded(t *testing.T) {
	setupDashboardAuthMiddlewareTest(t)
	buffer := logtest.New(t)
	patUser := createMiddlewarePATUser(t, "ace-pat-user", "ace-pat-opaque-token")

	engine := gin.New()
	engine.Use(buffer.Middleware())
	engine.GET("/api/user/self", UserAuth(), func(c *gin.Context) {
		c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"success": false})
	})
	request := httptest.NewRequest(http.MethodGet, "/api/user/self", nil)
	request.Header.Set("Authorization", "Bearer ace-pat-opaque-token")
	response := httptest.NewRecorder()
	engine.ServeHTTP(response, request)

	require.Equal(t, http.StatusBadRequest, response.Code)
	log := buffer.String()
	assert.Equal(t, 1, strings.Count(log, "event=authenticated_api_client_error"), log)
	assert.Contains(t, log, "identity=personal_access_token")
	assert.Contains(t, log, "user_id="+strconv.Itoa(patUser.Id))
}

// TokenAuthReadOnly 以代码语义为准：过期令牌放行即标记；禁用令牌拒绝不标记。
func TestTokenAuthReadOnlyMarksSuccessButNotRejections(t *testing.T) {
	setupAuthClientErrorDB(t)
	buffer := logtest.New(t)
	user := createAuthClientErrorUser(t, "ace-readonly-user")
	expired := model.Token{
		UserId: user.Id, Key: "acereadonlyexpired", Status: common.TokenStatusEnabled,
		ExpiredTime: common.GetTimestamp() - 100, RemainQuota: 0, Group: "default",
	}
	require.NoError(t, model.DB.Create(&expired).Error)
	disabled := model.Token{
		UserId: user.Id, Key: "acereadonlydisabled", Status: common.TokenStatusDisabled,
		ExpiredTime: -1, RemainQuota: 1000, Group: "default",
	}
	require.NoError(t, model.DB.Create(&disabled).Error)

	engine := newAuthClientErrorEngine(buffer, TokenAuthReadOnly(), func(c *gin.Context) {
		c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": "x"})
	})
	for _, tc := range []struct {
		key       string
		status    int
		eventLine bool
	}{
		{"acereadonlyexpired", http.StatusBadRequest, true},
		{"acereadonlydisabled", http.StatusUnauthorized, false},
	} {
		before := buffer.Len()
		request := httptest.NewRequest(http.MethodPost, "/v1/assets", nil)
		request.Header.Set("Authorization", "Bearer "+tc.key)
		response := httptest.NewRecorder()
		engine.ServeHTTP(response, request)
		require.Equal(t, tc.status, response.Code, tc.key)
		assert.Equal(t, tc.eventLine, strings.Contains(buffer.String()[before:], "event=authenticated_api_client_error"), tc.key)
	}
	log := buffer.String()
	assert.Equal(t, 1, strings.Count(log, "event=authenticated_api_client_error"), log)
	assert.Contains(t, log, "identity=api_token_readonly")
}

func TestOptionalAndHeaderNavClientErrorCoverage(t *testing.T) {
	setupDashboardAuthMiddlewareTest(t)
	createMiddlewarePATUser(t, "optional-log-user", "optional-log-pat")
	common.OptionMapRWMutex.RLock()
	previous := common.OptionMap
	common.OptionMapRWMutex.RUnlock()
	t.Cleanup(func() {
		common.OptionMapRWMutex.Lock()
		common.OptionMap = previous
		common.OptionMapRWMutex.Unlock()
	})
	for _, tc := range []struct {
		name, options, token string
		handler              gin.HandlerFunc
		status               int
		events               uint64
	}{
		{"optional anonymous", "", "", TryUserAuth(), 400, 0},
		{"optional matched", "", "optional-log-pat", TryUserAuth(), 400, 1},
		{"header public anonymous", `{"pricing":{"enabled":true,"requireAuth":false}}`, "", HeaderNavModuleAuth("pricing"), 400, 0},
		{"header public matched", `{"pricing":{"enabled":true,"requireAuth":false}}`, "optional-log-pat", HeaderNavModuleAuth("pricing"), 400, 1},
		{"header required", `{"pricing":{"enabled":true,"requireAuth":true}}`, "optional-log-pat", HeaderNavModuleAuth("pricing"), 400, 1},
		{"header required rejects anonymous", `{"pricing":{"enabled":true,"requireAuth":true}}`, "", HeaderNavModuleAuth("pricing"), 401, 0},
		{"header disabled before auth", `{"pricing":{"enabled":false}}`, "optional-log-pat", HeaderNavModuleAuth("pricing"), 403, 0},
		{"metrics disabled requires auth", `{"pricing":{"enabled":false}}`, "optional-log-pat", HeaderNavModulePublicOrUserAuth("pricing"), 400, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			common.OptionMapRWMutex.Lock()
			common.OptionMap = map[string]string{"HeaderNavModules": tc.options}
			common.OptionMapRWMutex.Unlock()
			capture := logtest.New(t)
			engine := gin.New()
			engine.Use(capture.Middleware())
			engine.GET("/api/pricing", tc.handler, func(c *gin.Context) { c.String(400, "business error") })
			req := httptest.NewRequest("GET", "/api/pricing", nil)
			if tc.token != "" {
				req.Header.Set("Authorization", "Bearer "+tc.token)
			}
			r := httptest.NewRecorder()
			engine.ServeHTTP(r, req)
			assert.Equal(t, tc.status, r.Code)
			assert.Equal(t, tc.events, capture.Health().Accepted)
			if tc.events > 0 {
				assert.Contains(t, capture.String(), "identity=personal_access_token")
			}
		})
	}
}

func TestArtifactCapabilityClientErrorCoverage(t *testing.T) {
	previous := common.CryptoSecret
	common.CryptoSecret = "client-error-artifact-fixture"
	t.Cleanup(func() { common.CryptoSecret = previous })
	access, err := service.IssueTaskArtifactAccess("private-task", "private-artifact")
	require.NoError(t, err)
	for _, valid := range []bool{false, true} {
		capture := logtest.New(t)
		engine := gin.New()
		engine.Use(capture.Middleware())
		engine.GET("/v1/tasks/:key/artifacts/:artifact_key/content", TokenOrTaskArtifactAccessAuth("key", "artifact_key"), func(c *gin.Context) { c.Status(404) })
		query := "invalid"
		if valid {
			query = access
		}
		req := httptest.NewRequest("GET", "/v1/tasks/private-task/artifacts/private-artifact/content?access="+urlQueryEscape(query), nil)
		r := httptest.NewRecorder()
		engine.ServeHTTP(r, req)
		assert.Equal(t, 404, r.Code)
		if valid {
			log := capture.String()
			assert.Contains(t, log, "identity=task_artifact_access")
			assert.Contains(t, log, "identity_context=missing")
			assert.NotContains(t, log, access)
			assert.NotContains(t, log, "private-task")
		} else {
			assert.Zero(t, capture.Health().Accepted)
		}
	}
}

func TestSessionClientErrorSuccessAndRoleRejection(t *testing.T) {
	setupDashboardAuthMiddlewareTest(t)
	user := createMiddlewarePATUser(t, "session-log-user", "unused-session-pat")
	session := &model.UserSession{SID: "session-log-fixture", UserID: user.Id, Version: 1, UserAuthVersion: user.AuthVersion, Status: model.UserSessionStatusActive, RefreshHash: "fixture", LoginMethod: "password", LastActiveAt: time.Now().Unix(), ExpiresAt: time.Now().Add(time.Hour).Unix()}
	require.NoError(t, model.CreateUserSession(session))
	token, _, err := service.IssueAccessToken(service.AuthIdentity{UserID: user.Id, SessionID: session.SID, UserAuthVersion: session.UserAuthVersion, SessionVersion: session.Version})
	require.NoError(t, err)
	for _, tc := range []struct {
		name   string
		auth   gin.HandlerFunc
		status int
		events uint64
	}{
		{"session", UserAuth(), 400, 1}, {"role rejected", AdminAuth(), 403, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			capture := logtest.New(t)
			engine := gin.New()
			engine.Use(capture.Middleware())
			engine.GET("/api/test", tc.auth, func(c *gin.Context) { c.String(400, "business error") })
			req := httptest.NewRequest("GET", "/api/test", nil)
			req.Header.Set("Authorization", "Bearer "+token)
			r := httptest.NewRecorder()
			engine.ServeHTTP(r, req)
			assert.Equal(t, tc.status, r.Code)
			assert.Equal(t, tc.events, capture.Health().Accepted)
			if tc.events > 0 {
				assert.Contains(t, capture.String(), "identity=session")
			}
		})
	}
}
