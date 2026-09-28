package main

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/billingexpr"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

type evidenceTransport func(*http.Request) (*http.Response, error)

func (f evidenceTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestHistoricalEvidenceRequiresExactIdentityAndBoundedUsage(t *testing.T) {
	original := http.DefaultTransport
	t.Cleanup(func() { http.DefaultTransport = original })
	good := `{"id":"provider-id","model":"viduq3-drama-std","status":"succeeded","content":{"video_url":"https://cdn.example/video.mp4"},"usage":{"completion_tokens":"108900","total_tokens":"108900"},"duration":"5","resolution":"720p"}`
	for _, tc := range []struct {
		name, body string
		valid      bool
	}{
		{"verified string integers", good, true},
		{"wrong task", strings.Replace(good, `"provider-id"`, `"another-id"`, 1), false},
		{"wrong model", strings.Replace(good, `"viduq3-drama-std"`, `"other-model"`, 1), false},
		{"not terminal", strings.Replace(good, `"succeeded"`, `"running"`, 1), false},
		{"missing completion", strings.Replace(good, `"completion_tokens":"108900",`, "", 1), false},
		{"negative usage", strings.Replace(good, `"completion_tokens":"108900"`, `"completion_tokens":"-1"`, 1), false},
		{"overflow usage", strings.Replace(good, `"completion_tokens":"108900"`, `"completion_tokens":"2147483648"`, 1), false},
		{"fractional usage", strings.Replace(good, `"completion_tokens":"108900"`, `"completion_tokens":"1.5"`, 1), false},
		{"contradictory total", strings.Replace(good, `"total_tokens":"108900"`, `"total_tokens":"1"`, 1), false},
		{"unsafe result scheme", strings.Replace(good, "https://cdn.example", "http://cdn.example", 1), false},
		{"unbounded duration", strings.Replace(good, `"duration":"5"`, `"duration":"999"`, 1), false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			http.DefaultTransport = evidenceTransport(func(r *http.Request) (*http.Response, error) {
				assert.Equal(t, http.MethodGet, r.Method)
				assert.Equal(t, "https://api.vidu.cn/ent/api/v3/contents/generations/tasks/provider-id", r.URL.String())
				assert.Equal(t, "Bearer frozen-key", r.Header.Get("Authorization"))
				return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(tc.body)), Header: make(http.Header)}, nil
			})
			task := model.Task{Properties: model.Properties{UpstreamModelName: "viduq3-drama-std"}, PrivateData: model.TaskPrivateData{UpstreamTaskID: "provider-id", VideoUpstreamQueryBaseURL: "https://api.vidu.cn/ent", Key: "frozen-key"}}
			got, err := queryEvidence(&task)
			if !tc.valid {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, 108900, got.Tokens)
			assert.Equal(t, 5, got.Duration)
			assert.Len(t, got.Digest, 64)
		})
	}
}

// A durable observation can outlive the provider's task retention window.
func TestHistoricalRecoveryResumesWithoutProvider(t *testing.T) {
	for _, stage := range []string{"funding", "log delivery", "conflicting audit"} {
		t.Run(stage, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "tasks.db")
			db, err := gorm.Open(sqlite.Open(path), &gorm.Config{Logger: logger.Discard})
			require.NoError(t, err)
			sqlDB, err := db.DB()
			require.NoError(t, err)
			t.Cleanup(func() { require.NoError(t, sqlDB.Close()) })
			require.NoError(t, db.AutoMigrate(&model.User{}, &model.Token{}, &model.Channel{}, &model.UserSubscription{}, &model.Task{}, &model.TaskBillingDelivery{}, &model.Log{}, &model.AuditLog{}, &model.QuotaData{}))
			require.NoError(t, db.Create(&model.User{Id: 1, Username: "operator", AffCode: "operator", Role: common.RoleRootUser, Status: common.UserStatusEnabled}).Error)
			require.NoError(t, db.Create(&model.User{Id: 2, Username: "customer", AffCode: "customer", Quota: 900, UsedQuota: 100}).Error)
			require.NoError(t, db.Create(&model.Token{Id: 3, UserId: 2, Key: "fixture-token", RemainQuota: 900, UsedQuota: 100}).Error)
			require.NoError(t, db.Create(&model.Channel{Id: 4, UsedQuota: 100}).Error)
			target := 40
			task := model.Task{TaskID: "task_fixture", UserId: 2, AppID: 3, ChannelId: 4, Quota: 100, Status: model.TaskStatusSuccess, BillingState: model.TaskBillingStatePending,
				Properties: model.Properties{OriginModelName: "customer-video", UpstreamModelName: "viduq3-drama-std"},
				PrivateData: model.TaskPrivateData{TokenId: 3, BillingSource: "wallet", UpstreamTaskID: "fixture-provider-task", VideoUpstreamProtocol: dto.VideoUpstreamProtocolModelArkV3Volcengine, VideoUpstreamQueryBaseURL: "https://api.vidu.cn/ent",
					Execution:    &model.TaskExecutionSnapshot{TaskPlugin: &model.TaskPluginSnapshot{Key: "seedance-link", Version: "1.3.5", APIVersion: 3}},
					AsyncBilling: &model.TaskAsyncBillingContext{State: model.TaskBillingStatePending, Operation: "settle", ActualUsageReported: true, ActualTokens: 10, TargetQuota: &target, CalculationVersion: 1, CalculationSource: "tiered_expr", Calculation: &billingexpr.Calculation{Version: 1, Quota: target}, TieredSnapshot: &billingexpr.BillingSnapshot{ExprString: "c * 8", GroupRatio: 1, QuotaPerUnit: 500000}},
				},
			}
			require.NoError(t, db.Create(&task).Error)
			auditTarget := target
			if stage == "conflicting audit" {
				auditTarget++
			}
			audit := model.AuditLog{EventId: fmt.Sprintf("vidu-history-success:%d", task.ID), UserId: 1, Action: "task.vidu_verified_success", Success: true, Other: model.AuditOther{RootInfo: model.AuditFields{"reference": "verified_fixture", "task_id": task.TaskID, "completion_tokens": 10, "held_quota": 100, "target_quota": auditTarget}}}
			require.NoError(t, db.Create(&audit).Error)
			original := http.DefaultTransport
			calls := 0
			http.DefaultTransport = evidenceTransport(func(*http.Request) (*http.Response, error) {
				calls++
				return &http.Response{StatusCode: 404, Body: io.NopCloser(strings.NewReader("gone")), Header: make(http.Header)}, nil
			})
			t.Cleanup(func() { http.DefaultTransport = original })
			args := []string{"-database", path, "-task-id", task.TaskID, "-operator-id", "1", "-expected-hold", "100", "-reference", "verified_fixture", "-apply", "-offline-no-redis"}
			if stage == "funding" {
				require.NoError(t, db.Exec(`CREATE TRIGGER reject_funding BEFORE UPDATE OF remain_quota ON tokens BEGIN SELECT RAISE(ABORT, 'fixture-error'); END`).Error)
			} else if stage == "log delivery" {
				require.NoError(t, db.Exec(`CREATE TRIGGER reject_delivery BEFORE INSERT ON logs BEGIN SELECT RAISE(ABORT, 'fixture-error'); END`).Error)
			}
			err = run(append(args, "-backup", filepath.Join(dir, "first.db")), &bytes.Buffer{})
			require.Error(t, err)
			if stage == "conflicting audit" {
				assert.Contains(t, err.Error(), "reconciliation reference conflicts")
			} else {
				if stage == "funding" {
					assert.Contains(t, err.Error(), "funding pending")
					require.NoError(t, db.Exec("DROP TRIGGER reject_funding").Error)
				} else {
					assert.Contains(t, err.Error(), "log delivery pending")
					require.NoError(t, db.Exec("DROP TRIGGER reject_delivery").Error)
				}
				for _, name := range []string{"retry.db", "repeat.db"} {
					require.NoError(t, run(append(args, "-backup", filepath.Join(dir, name)), &bytes.Buffer{}))
				}
				st, err := os.Stat(filepath.Join(dir, "retry.db"))
				require.NoError(t, err)
				assert.Equal(t, os.FileMode(0600), st.Mode().Perm())
			}
			assert.Zero(t, calls, "accepted evidence must not be queried again")
			var saved model.Task
			require.NoError(t, db.First(&saved, task.ID).Error)
			assert.Equal(t, task.PrivateData.Execution, saved.PrivateData.Execution)
			assert.Equal(t, task.PrivateData.VideoUpstreamProtocol, saved.PrivateData.VideoUpstreamProtocol)
			assert.Equal(t, task.PrivateData.AsyncBilling.TieredSnapshot, saved.PrivateData.AsyncBilling.TieredSnapshot)
			var user model.User
			var token model.Token
			var channel model.Channel
			require.NoError(t, db.First(&user, 2).Error)
			require.NoError(t, db.First(&token, 3).Error)
			require.NoError(t, db.First(&channel, 4).Error)
			var logs []model.Log
			var deliveries []model.TaskBillingDelivery
			require.NoError(t, db.Find(&logs).Error)
			require.NoError(t, db.Find(&deliveries).Error)
			if stage == "conflicting audit" {
				assert.Equal(t, 900, user.Quota)
				assert.Equal(t, 900, token.RemainQuota)
				assert.EqualValues(t, 100, channel.UsedQuota)
				assert.Empty(t, logs)
				assert.Empty(t, deliveries)
			} else {
				assert.Equal(t, 40, saved.Quota)
				assert.Equal(t, model.TaskBillingStateSettled, saved.BillingState)
				assert.Equal(t, 960, user.Quota)
				assert.EqualValues(t, 40, user.UsedQuota)
				assert.Equal(t, 960, token.RemainQuota)
				assert.Equal(t, 40, token.UsedQuota)
				assert.EqualValues(t, 40, channel.UsedQuota)
				require.Len(t, logs, 1)
				assert.Equal(t, 60, logs[0].Quota)
				require.Len(t, deliveries, 1)
				assert.NotZero(t, deliveries[0].DeliveredAt)
			}
		})
	}
}
