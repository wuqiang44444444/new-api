package controller

import (
	"bytes"
	"context"
	"encoding/base64"
	"github.com/QuantumNous/new-api/service"
	"image"
	"image/png"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/QuantumNous/new-api/setting/system_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The real Controller owns validation, pre-consume, response conversion and
// settlement. Provider and storage transports are the only simulated boundaries.
func imageFormatChannelEngine(t *testing.T, channel model.Channel, name string) *gin.Engine {
	t.Helper()
	db := nativeImageTestDB(t)
	user := model.User{Id: 8961, Username: "format-contract", Quota: 1000000, Group: "default", Status: common.UserStatusEnabled}
	token := model.Token{Id: 8961, UserId: user.Id, Key: strings.Repeat("f", 32), RemainQuota: 1000000, Status: common.TokenStatusEnabled, ExpiredTime: -1}
	require.NoError(t, db.Create(&user).Error)
	require.NoError(t, db.Create(&token).Error)
	require.NoError(t, db.Create(&channel).Error)
	prices, err := common.Marshal(map[string]float64{name: 0.01})
	require.NoError(t, err)
	require.NoError(t, ratio_setting.UpdateModelPriceByJSONString(string(prices)))
	engine := gin.New()
	engine.Use(func(c *gin.Context) {
		common.SetContextKey(c, constant.ContextKeyUserId, user.Id)
		common.SetContextKey(c, constant.ContextKeyUserQuota, user.Quota)
		common.SetContextKey(c, constant.ContextKeyUserGroup, "default")
		common.SetContextKey(c, constant.ContextKeyUsingGroup, "default")
		common.SetContextKey(c, constant.ContextKeyUserSetting, dto.UserSetting{BillingPreference: "wallet_only"})
		common.SetContextKey(c, constant.ContextKeyTokenId, token.Id)
		common.SetContextKey(c, constant.ContextKeyTokenKey, token.Key)
		require.Nil(t, middleware.SetupContextForSelectedChannel(c, &channel, name))
		c.Next()
	})
	for _, path := range []string{"/v1/images/generations", "/v1/images/edits"} {
		engine.POST(path, middleware.ImageCreateIdempotency(), func(c *gin.Context) { Relay(c, types.RelayFormatOpenAIImage) })
	}
	return engine
}

func TestImageFormatChannelContracts(t *testing.T) {
	var pngData bytes.Buffer
	require.NoError(t, png.Encode(&pngData, image.NewRGBA(image.Rect(0, 0, 2, 2))))
	encoded := base64.StdEncoding.EncodeToString(pngData.Bytes())
	for _, tc := range []struct {
		name, model, protocol string
		channelType           int
	}{
		{"gemini", "gemini-3.1-flash-image", "", constant.ChannelTypeGemini},
		{"vertex", "gemini-3.1-flash-image", "", constant.ChannelTypeVertexAi},
		{"imagen", "imagen-4.0-generate-001", "", constant.ChannelTypeGemini},
		{"funcloud", constant.FunCloudImageProviderModelNanoBanana2Lite, "funcloud_aigc_v2", constant.ChannelTypeAsyncImage},
		{"moxing", constant.MoxingImageProviderModelSeedream5Lite, "moxing_images_v1", constant.ChannelTypeAsyncImage},
	} {
		for _, operation := range []string{"generations", "edits"} {
			if tc.name == "imagen" && operation == "edits" {
				continue
			}
			for _, format := range []string{"", "url", "b64_json"} {
				t.Run(tc.name+"/"+operation+"/"+format, func(t *testing.T) {
					calls, downloads := 0, 0
					source := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						downloads++
						w.Header().Set("Content-Type", "image/png")
						_, _ = w.Write(pngData.Bytes())
					}))
					defer source.Close()
					sourceURL := source.URL + "/result.png"
					provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						calls++
						_, _ = io.Copy(io.Discard, r.Body)
						w.Header().Set("Content-Type", "application/json")
						var response any
						switch tc.name {
						case "gemini", "vertex":
							response = map[string]any{"candidates": []any{map[string]any{"content": map[string]any{"parts": []any{map[string]any{"inlineData": map[string]any{"mimeType": "image/png", "data": encoded}}}}}}, "usageMetadata": map[string]any{"promptTokenCount": 1, "candidatesTokenCount": 1, "totalTokenCount": 2}}
						case "imagen":
							response = map[string]any{"predictions": []any{map[string]any{"bytesBase64Encoded": encoded, "mimeType": "image/png"}}}
						case "funcloud":
							response = map[string]any{"code": 0, "data": map[string]any{"status": "success", "result": []string{sourceURL}}}
						default:
							response = map[string]any{"data": []any{map[string]any{"url": sourceURL}}}
						}
						data, err := common.Marshal(response)
						assert.NoError(t, err)
						_, _ = w.Write(data)
					}))
					defer provider.Close()
					settings := `{"vertex_key_type":"api_key","image_upstream_protocol":"` + tc.protocol + `"}`
					channel := model.Channel{Id: 8961, Type: tc.channelType, Key: "fixture", BaseURL: &provider.URL, Status: common.ChannelStatusEnabled, OtherSettings: settings}
					engine := imageFormatChannelEngine(t, channel, tc.model)
					if format != "url" || tc.channelType == constant.ChannelTypeAsyncImage {
						model.NotifyObjectStorageSettingUpdate("")
					}
					fetch := system_setting.GetFetchSetting()
					original := *fetch
					t.Cleanup(func() { *fetch = original })
					fetch.EnableSSRFProtection = false
					payload := map[string]any{"model": tc.model, "prompt": "draw a square"}
					if tc.name == "gemini" || tc.name == "vertex" {
						payload["size"] = "auto"
					}
					if format != "" {
						payload["response_format"] = format
					}
					if operation == "edits" {
						payload["images"] = []string{"https://example.com/reference.png"}
					}
					body, err := common.Marshal(payload)
					require.NoError(t, err)
					w := submitNativeImageCreate(t, engine, "/v1/images/"+operation, string(body), false)
					require.Equal(t, 200, w.Code, w.Body.String())
					var result dto.ImageResponse
					require.NoError(t, common.Unmarshal(w.Body.Bytes(), &result))
					require.Len(t, result.Data, 1)
					wantURL := format == "url" || (format == "" && tc.channelType == constant.ChannelTypeAsyncImage)
					if wantURL {
						assert.NotEmpty(t, result.Data[0].Url)
						assert.Empty(t, result.Data[0].B64Json)
						if tc.channelType == constant.ChannelTypeAsyncImage {
							assert.Equal(t, sourceURL, result.Data[0].Url)
						}
					} else {
						assert.NotEmpty(t, result.Data[0].B64Json)
						assert.Empty(t, result.Data[0].Url)
					}
					assert.Equal(t, 1, calls)
					if tc.channelType == constant.ChannelTypeAsyncImage && format == "b64_json" {
						assert.Equal(t, 1, downloads)
					} else {
						assert.Zero(t, downloads, "matching URL must not be downloaded again")
					}
				})
			}
		}
	}
}

func TestImageFormatGoogleDeliveryFailsWithoutRegeneratingOrRefunding(t *testing.T) {
	for _, fail := range []bool{false, true} {
		t.Run(map[bool]string{false: "single_upload", true: "failed_upload"}[fail], func(t *testing.T) {
			var data bytes.Buffer
			require.NoError(t, png.Encode(&data, image.NewRGBA(image.Rect(0, 0, 2, 2))))
			encoded := base64.StdEncoding.EncodeToString(data.Bytes())
			calls := 0
			provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				w.Header().Set("Content-Type", "application/json")
				_, _ = io.WriteString(w, `{"candidates":[{"content":{"parts":[{"inlineData":{"mimeType":"image/png","data":"`+encoded+`"}}]}}],"usageMetadata":{"promptTokenCount":1,"candidatesTokenCount":1,"totalTokenCount":2}}`)
			}))
			defer provider.Close()
			channel := model.Channel{Id: 8961, Type: constant.ChannelTypeGemini, Key: "fixture", BaseURL: &provider.URL, Status: common.ChannelStatusEnabled}
			engine := imageFormatChannelEngine(t, channel, "gemini-3.1-flash-image")
			common.LogConsumeEnabled = true
			writes, reads := 0, 0
			var probe []byte
			store := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if strings.Contains(r.URL.Path, "object-storage-health") {
					if r.Method == http.MethodPut {
						probe, _ = io.ReadAll(r.Body)
					} else {
						_, _ = w.Write(probe)
					}
					return
				}
				if r.Method == http.MethodPut {
					writes++
					if fail {
						w.WriteHeader(503)
					}
				} else {
					reads++
					w.WriteHeader(404)
				}
			}))
			defer store.Close()
			config, err := common.Marshal(system_setting.ObjectStorageConfig{Backend: "s3", Endpoint: store.URL, Bucket: "images", AccountName: "fixture", Credential: "fixture", Region: "us-east-1", Revision: t.Name() + "-delivery"})
			require.NoError(t, err)
			model.NotifyObjectStorageSettingUpdate(string(config))
			w := submitNativeImageCreate(t, engine, "/v1/images/generations", `{"model":"gemini-3.1-flash-image","prompt":"draw","size":"auto","response_format":"url"}`, false)
			if fail {
				assert.Equal(t, 502, w.Code)
				assert.Contains(t, w.Body.String(), "image_delivery_failed")
			} else {
				assert.Equal(t, 200, w.Code)
				assert.Contains(t, w.Body.String(), `"url"`)
			}
			if fail {
				assert.Equal(t, 3, writes, "only upload is retried")
				var recovered dto.ImageResponse
				require.NoError(t, common.Unmarshal(w.Body.Bytes(), &recovered))
				require.Len(t, recovered.Data, 1)
				assert.Equal(t, encoded, recovered.Data[0].B64Json)
			} else {
				assert.Equal(t, 1, writes)
			}
			assert.Zero(t, reads)
			assert.Equal(t, 1, calls)
			var logs []model.Log
			require.NoError(t, model.LOG_DB.Where("type = ?", model.LogTypeConsume).Find(&logs).Error)
			require.Len(t, logs, 1)
			var user model.User
			require.NoError(t, model.DB.First(&user, 8961).Error)
			assert.Equal(t, 1000000-logs[0].Quota, user.Quota)
			assert.Positive(t, logs[0].Quota)
		})
	}
}

func TestImageFormatNativeWirePreservesInputsAndFrozenDelivery(t *testing.T) {
	for _, async := range []bool{false, true} {
		for _, wire := range []string{"json", "passthrough_json", "multipart", "passthrough_multipart", "override"} {
			t.Run(wire+"/async="+strconv.FormatBool(async), func(t *testing.T) {
				calls := 0
				provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					calls++
					if strings.Contains(wire, "multipart") {
						require.NoError(t, r.ParseMultipartForm(1<<20))
						defer r.MultipartForm.RemoveAll()
						assert.NotContains(t, r.MultipartForm.Value, "response_format")
						assert.Equal(t, []string{"first", "second"}, r.MultipartForm.Value["user"])
						require.Len(t, r.MultipartForm.File["image"], 1)
						require.Len(t, r.MultipartForm.File["mask"], 1)
					} else {
						var body map[string]any
						require.NoError(t, common.DecodeJson(r.Body, &body))
						assert.NotContains(t, body, "response_format")
						assert.NotEmpty(t, body["images"])
						assert.NotEmpty(t, body["mask"])
					}
					w.Header().Set("Content-Type", "application/json")
					_, _ = io.WriteString(w, `{"data":[{"b64_json":"iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+jRZkAAAAASUVORK5CYII="}],"usage":{"input_tokens":1,"output_tokens":1}}`)
				}))
				defer provider.Close()
				mapping := `{"gpt-image-2":"opaque-deployment"}`
				channel := model.Channel{Id: 8961, Type: constant.ChannelTypeAzure, Key: "fixture", BaseURL: &provider.URL, Status: common.ChannelStatusEnabled, ModelMapping: &mapping}
				if strings.HasPrefix(wire, "passthrough") {
					setting := `{"pass_through_body_enabled":true}`
					channel.Setting = &setting
				}
				if wire == "override" {
					override := `{"response_format":"url"}`
					channel.ParamOverride = &override
				}
				engine := imageFormatChannelEngine(t, channel, "gpt-image-2")
				body := bytes.NewBufferString(`{"model":"gpt-image-2","prompt":"edit","response_format":"b64_json","images":[{"image_url":"https://example.com/reference.png"}],"mask":{"image_url":"https://example.com/mask.png"}}`)
				contentType := "application/json"
				if strings.Contains(wire, "multipart") {
					body.Reset()
					writer := multipart.NewWriter(body)
					for _, field := range [][2]string{{"model", "gpt-image-2"}, {"prompt", "edit"}, {"response_format", "b64_json"}, {"user", "first"}, {"user", "second"}} {
						require.NoError(t, writer.WriteField(field[0], field[1]))
					}
					for _, field := range []string{"image", "mask"} {
						part, err := writer.CreateFormFile(field, field+".png")
						require.NoError(t, err)
						_, err = part.Write([]byte("preserved-image"))
						require.NoError(t, err)
					}
					require.NoError(t, writer.Close())
					contentType = writer.FormDataContentType()
				}
				req := httptest.NewRequest("POST", "/v1/images/edits", body)
				req.Header.Set("Content-Type", contentType)
				if async {
					req.Header.Set("Prefer", "respond-async")
				}
				w := httptest.NewRecorder()
				engine.ServeHTTP(w, req)
				if async {
					require.Equal(t, 202, w.Code, w.Body.String())
					assert.Zero(t, calls)
					var task model.Task
					require.NoError(t, model.DB.First(&task).Error)
					assert.Equal(t, "b64_json", task.PrivateData.ImageTask.ResponseFormat)
					service.RunImageTaskWorkerOnce(context.Background())
					service.RunImageTaskWorkerOnce(context.Background())
					require.NoError(t, model.DB.First(&task, task.ID).Error)
					assert.EqualValues(t, model.TaskStatusSuccess, task.Status)
				} else {
					require.Equal(t, 200, w.Code, w.Body.String())
					assert.Contains(t, w.Body.String(), `"b64_json"`)
					assert.NotContains(t, w.Body.String(), `"url"`)
				}
				assert.Equal(t, 1, calls)
			})
		}
	}
}
