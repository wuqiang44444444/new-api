package controller

import (
	"encoding/base64"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/system_setting"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestImageFormatDeliveryPreservesBillingAndNeverRegenerates(t *testing.T) {
	// Valid 1x1 PNG, deterministic bytes rather than a generated asset.
	const pngB64 = "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+jRZkAAAAASUVORK5CYII="
	for _, tc := range []struct {
		name, option string
		failStore    bool
		status       int
	}{
		{"default", "", false, 200}, {"base64", `,"response_format":"b64_json"`, false, 200},
		{"hosted", `,"response_format":"url"`, false, 200},
		{"hosted_edits", `,"response_format":"url"`, false, 200}, {"delivery_failure", `,"response_format":"url"`, true, 502},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newNativeDiagFixture(t, 200, `{"created":123,"data":[{"b64_json":"`+pngB64+`","revised_prompt":"kept"}],"usage":{"input_tokens":1,"output_tokens":1}}`, nil)
			common.LogConsumeEnabled = true
			var saved []byte
			writes := 0
			if tc.failStore {
				store := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					if strings.Contains(r.URL.Path, "/object-storage-health/") {
						if r.Method == http.MethodPut {
							saved, _ = io.ReadAll(r.Body)
						} else {
							_, _ = w.Write(saved)
						}
						return
					}
					writes++
					w.WriteHeader(503)
				}))
				t.Cleanup(store.Close)
				config, err := common.Marshal(system_setting.ObjectStorageConfig{Backend: "s3", Endpoint: store.URL, Bucket: "images", AccountName: "fixture", Region: "us-east-1", Credential: "fixture", Revision: t.Name() + "-failure"})
				require.NoError(t, err)
				model.NotifyObjectStorageSettingUpdate(string(config))
			}
			path, input := "/v1/images/generations", ""
			if tc.name == "hosted_edits" {
				path, input = "/v1/images/edits", `,"images":[{"image_url":"https://example.com/reference.png"}]`
			}
			w := submitNativeImageCreate(t, f.engine, path, `{"model":"gpt-image-2","prompt":"fixture"`+input+tc.option+`}`, false)
			require.Equal(t, tc.status, w.Code)
			assert.EqualValues(t, 1, f.calls.Load())
			var result struct {
				Data []struct {
					URL     string `json:"url"`
					B64     string `json:"b64_json"`
					Revised string `json:"revised_prompt"`
					Expires int64  `json:"url_expires_at"`
				}
				Error struct {
					Code string `json:"code"`
				}
			}
			require.NoError(t, common.Unmarshal(w.Body.Bytes(), &result))
			if tc.failStore {
				assert.Equal(t, "image_delivery_failed", result.Error.Code)
				assert.Equal(t, 3, writes, "retry only the same result upload")
				require.Len(t, result.Data, 1)
				assert.Equal(t, pngB64, result.Data[0].B64, "failed delivery must preserve the generated image")
			} else {
				require.Len(t, result.Data, 1)
				assert.Equal(t, "kept", result.Data[0].Revised)
				if strings.HasPrefix(tc.name, "hosted") {
					assert.Empty(t, result.Data[0].B64)
					assert.NotEmpty(t, result.Data[0].URL)
					assert.Positive(t, result.Data[0].Expires)
					resp, err := http.Get(result.Data[0].URL)
					require.NoError(t, err)
					defer resp.Body.Close()
					actual, err := io.ReadAll(resp.Body)
					require.NoError(t, err)
					expected, err := base64.StdEncoding.DecodeString(pngB64)
					require.NoError(t, err)
					assert.Equal(t, expected, actual)
				} else {
					assert.Equal(t, pngB64, result.Data[0].B64)
					assert.Empty(t, result.Data[0].URL)
				}
			}
			var logs []model.Log
			require.NoError(t, model.LOG_DB.Where("type = ?", model.LogTypeConsume).Find(&logs).Error)
			require.Len(t, logs, 1, "delivery must settle generation exactly once")
			assert.Positive(t, logs[0].Quota)
			var user model.User
			require.NoError(t, f.db.First(&user, f.user.Id).Error)
			assert.Equal(t, 100000-logs[0].Quota, user.Quota, "storage failure must not refund generated images")
		})
	}
}

func TestImageFormatAsyncPreferenceIsFrozenAndPartOfIdempotency(t *testing.T) {
	f := newNativeDiagFixture(t, 200, `{"data":[]}`, nil)
	submit := func(option bool) *httptest.ResponseRecorder {
		body := `{"model":"gpt-image-2","prompt":"fixture","response_format":"b64_json"}`
		if option {
			body = strings.Replace(body, `"response_format":"b64_json"`, `"response_format":"url"`, 1)
		}
		req := httptest.NewRequest(http.MethodPost, "/v1/images/generations", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Prefer", "respond-async")
		req.Header.Set("Idempotency-Key", "oss-delivery-fixture")
		w := httptest.NewRecorder()
		f.engine.ServeHTTP(w, req)
		return w
	}
	w := submit(true)
	require.Equal(t, 202, w.Code)
	var tasks []model.Task
	require.NoError(t, f.db.Find(&tasks).Error)
	require.Len(t, tasks, 1)
	require.NotNil(t, tasks[0].PrivateData.ImageTask)
	assert.Equal(t, "url", tasks[0].PrivateData.ImageTask.ResponseFormat)
	assert.Equal(t, 202, submit(true).Code, "same delivery preference replays acceptance")
	assert.Equal(t, 409, submit(false).Code, "changing delivery preference cannot reuse an accepted key")
	assert.Zero(t, f.calls.Load(), "acceptance does not send provider bytes")
}

func TestImageFormatURLResultsUseProtectedFetchForBase64(t *testing.T) {
	imageBytes, err := base64.StdEncoding.DecodeString("iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+jRZkAAAAASUVORK5CYII=")
	require.NoError(t, err)
	for _, blocked := range []bool{false, true} {
		t.Run(map[bool]string{false: "host_url_result", true: "reject_private_result_url"}[blocked], func(t *testing.T) {
			sourceCalls := 0
			source := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { sourceCalls++; _, _ = w.Write(imageBytes) }))
			t.Cleanup(source.Close)
			body, err := common.Marshal(map[string]any{"data": []map[string]string{{"url": source.URL + "/image.png"}}})
			require.NoError(t, err)
			f := newNativeDiagFixture(t, 200, string(body), nil)
			fetch := system_setting.GetFetchSetting()
			original := *fetch
			t.Cleanup(func() { *fetch = original })
			// The successful local fixture explicitly allows its loopback server;
			// the protected case must reject it without reaching that server.
			fetch.EnableSSRFProtection = blocked
			fetch.AllowPrivateIp = false
			w := submitNativeImageCreate(t, f.engine, "/v1/images/generations", `{"model":"gpt-image-2","prompt":"fixture","response_format":"b64_json"}`, false)
			assert.EqualValues(t, 1, f.calls.Load())
			if blocked {
				assert.Equal(t, 502, w.Code)
				assert.Zero(t, sourceCalls)
				return
			}
			require.Equal(t, 200, w.Code)
			assert.Equal(t, 1, sourceCalls)
			var result struct {
				Data []struct {
					B64 string `json:"b64_json"`
				}
			}
			require.NoError(t, common.Unmarshal(w.Body.Bytes(), &result))
			require.Len(t, result.Data, 1)
			assert.Equal(t, base64.StdEncoding.EncodeToString(imageBytes), result.Data[0].B64)
		})
	}
}
