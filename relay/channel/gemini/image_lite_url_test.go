package gemini_test

import (
	"bytes"
	"encoding/base64"
	"image"
	"image/png"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relay/channel/gemini"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/model_setting"
	"github.com/QuantumNous/new-api/setting/system_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLiteGenerationsAndEditsDeliverOnlyURLs(t *testing.T) {
	var uploaded []byte
	store := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPut, r.Method)
		body, err := io.ReadAll(r.Body)
		assert.NoError(t, err)
		if r.Header.Get("Content-Type") == "image/png" {
			uploaded = body
		}
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(store.Close)
	config, err := common.Marshal(system_setting.ObjectStorageConfig{Backend: "s3", Endpoint: store.URL, Bucket: "images", AccountName: "test-account", Region: "us-east-1", Credential: "fixture-secret", Revision: "lite-url-test"})
	require.NoError(t, err)
	model.NotifyObjectStorageSettingUpdate(string(config))
	t.Cleanup(func() { model.NotifyObjectStorageSettingUpdate("") })
	var pngData bytes.Buffer
	require.NoError(t, png.Encode(&pngData, image.NewNRGBA(image.Rect(0, 0, 1, 1))))
	body := `{"candidates":[{"content":{"parts":[{"inlineData":{"mimeType":"image/png","data":"` + base64.StdEncoding.EncodeToString(pngData.Bytes()) + `"}}]}}],"usageMetadata":{"promptTokenCount":10,"candidatesTokenCount":1120,"totalTokenCount":1130,"candidatesTokensDetails":[{"modality":"IMAGE","tokenCount":1120}]}}`
	service.InitHttpClient()
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/v1beta/models/gemini-3.1-flash-lite-image:generateContent", r.URL.Path)
		assert.Equal(t, "application/json", r.Header.Get("Content-Type"))
		assert.Equal(t, "fixture-key", r.Header.Get("x-goog-api-key"))
		var upstream dto.GeminiChatRequest
		assert.NoError(t, common.DecodeJson(r.Body, &upstream))
		assert.Equal(t, []string{"TEXT", "IMAGE"}, upstream.GenerationConfig.ResponseModalities)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, body)
	}))
	t.Cleanup(provider.Close)

	for _, encoding := range []string{"generation", "json-edit", "multipart-edit"} {
		path := "/v1/images/generations"
		mode := relayconstant.RelayModeImagesGenerations
		request := dto.ImageRequest{Model: "gemini-3.1-flash-lite-image", Prompt: "cup", Size: "auto", ResponseFormat: "url"}
		if encoding != "generation" {
			path = "/v1/images/edits"
			mode = relayconstant.RelayModeImagesEdits
			require.NoError(t, common.Unmarshal([]byte(`{"model":"gemini-3.1-flash-lite-image","prompt":"green cup","image":"data:image/png;base64,`+base64.StdEncoding.EncodeToString(pngData.Bytes())+`","response_format":"url"}`), &request))
		}
		recorder := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(recorder)
		c.Request = httptest.NewRequest(http.MethodPost, path, nil)
		if encoding == "multipart-edit" {
			var form bytes.Buffer
			writer := multipart.NewWriter(&form)
			file, err := writer.CreateFormFile("image", "fixture.png")
			require.NoError(t, err)
			_, err = file.Write(pngData.Bytes())
			require.NoError(t, err)
			require.NoError(t, writer.Close())
			request.Image = nil
			c.Request = httptest.NewRequest(http.MethodPost, path, &form)
			c.Request.Header.Set("Content-Type", writer.FormDataContentType())
			require.NoError(t, c.Request.ParseMultipartForm(1<<20))
			t.Cleanup(func() { _ = c.Request.MultipartForm.RemoveAll() })
		}
		info := &relaycommon.RelayInfo{UserId: 1, RelayMode: mode, ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: request.Model, ChannelType: constant.ChannelTypeGemini, ApiType: constant.APITypeGemini, ChannelBaseUrl: provider.URL, ApiKey: "fixture-key"}}
		converted, err := (&gemini.Adaptor{}).ConvertImageRequest(c, info, request)
		require.NoError(t, err)
		if encoding != "generation" {
			providerRequest := converted.(*dto.GeminiChatRequest)
			require.Len(t, providerRequest.Contents[0].Parts, 2)
			assert.Equal(t, base64.StdEncoding.EncodeToString(pngData.Bytes()), providerRequest.Contents[0].Parts[1].InlineData.Data)
		}
		encoded, err := common.Marshal(converted)
		require.NoError(t, err)
		responseValue, err := (&gemini.Adaptor{}).DoRequest(c, info, bytes.NewReader(encoded))
		require.NoError(t, err)
		settings := model_setting.GetGeminiSettings()
		previous := settings.SupportedImagineModels
		settings.SupportedImagineModels = []string{"gemini-3.1-flash-image"}
		usageValue, apiErr := (&gemini.Adaptor{}).DoResponse(c, responseValue.(*http.Response), info)
		settings.SupportedImagineModels = previous
		require.Nil(t, apiErr)
		usage := usageValue.(*dto.Usage)
		assert.Equal(t, 1120, usage.CompletionTokenDetails.ImageTokens)
		assert.Equal(t, http.StatusOK, recorder.Code)
		var response dto.ImageResponse
		require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
		require.Len(t, response.Data, 1)
		assert.True(t, strings.HasPrefix(response.Data[0].Url, store.URL+"/"))
		assert.Empty(t, response.Data[0].B64Json)
		assert.Equal(t, pngData.Bytes(), uploaded)
	}
}
