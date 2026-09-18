package relay

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/system_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestImageFormatPreflightRechecksSelectedChannel(t *testing.T) {
	model.NotifyObjectStorageSettingUpdate("")
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/images/generations", nil)
	common.SetContextKey(c, constant.ContextKeyOriginalModel, "customer-image")
	common.SetContextKey(c, constant.ContextKeyChannelType, constant.ChannelTypeOpenAI)
	c.Set("model_mapping", `{"customer-image":"dall-e-3"}`)
	info := relaycommon.GenRelayInfoImage(c, &dto.ImageRequest{ResponseFormat: "url"})
	require.Nil(t, PrepareImageResponseFormat(c, info), "native URL channel must not need storage")
	require.Nil(t, info.ChannelMeta, "preflight must not alter live per-attempt or billing state")
	c.Set("model_mapping", `{"customer-image":"gpt-image-2"}`)
	// Exercise the real per-attempt entry after the native router changes mapping.
	err := ImageHelper(c, info)
	require.NotNil(t, err)
	assert.Equal(t, http.StatusServiceUnavailable, err.StatusCode)
}

func TestImageDeliveryLargeConversionAndRetryPreservesOriginalBytes(t *testing.T) {
	// A valid PNG with trailing padding crosses the former 50 MiB image bound.
	// The test protects actual byte delivery, rather than timing or heap counts.
	png, err := base64.StdEncoding.DecodeString("iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+jRZkAAAAASUVORK5CYII=")
	require.NoError(t, err)
	media, err := os.CreateTemp(t.TempDir(), "large-image-*")
	require.NoError(t, err)
	defer media.Close()
	_, err = media.Write(png)
	require.NoError(t, err)
	const size = int64(50<<20) + 1
	require.NoError(t, media.Truncate(size))
	digest := sha256.New()
	_, err = io.Copy(digest, io.NewSectionReader(media, 0, size))
	require.NoError(t, err)
	var savedDigest []byte
	attempts := 0
	store := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		if attempts == 1 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		hash := sha256.New()
		n, err := io.Copy(hash, r.Body)
		assert.NoError(t, err)
		assert.Equal(t, size, n)
		savedDigest = hash.Sum(nil)
	}))
	defer store.Close()
	config, err := common.Marshal(system_setting.ObjectStorageConfig{Backend: "s3", Endpoint: store.URL, Bucket: "images", AccountName: "fixture", Credential: "fixture", Region: "us-east-1", Revision: t.Name()})
	require.NoError(t, err)
	model.NotifyObjectStorageSettingUpdate(string(config))
	t.Cleanup(func() { model.NotifyObjectStorageSettingUpdate("") })
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/images/generations", nil)
	delivery, deliveryErr := beginImageDelivery(c, &relaycommon.RelayInfo{Request: &dto.ImageRequest{ResponseFormat: "url"}})
	require.Nil(t, deliveryErr)
	defer delivery.cleanup(c)
	require.NoError(t, delivery.err)
	_, err = io.WriteString(delivery, `{"usage":{"total_tokens":7},"data":[{"revised_prompt":"kept","b64_json":"`)
	require.NoError(t, err)
	encoder := base64.NewEncoder(base64.StdEncoding, delivery)
	_, err = io.Copy(encoder, io.NewSectionReader(media, 0, size))
	require.NoError(t, err)
	require.NoError(t, encoder.Close())
	_, err = io.WriteString(delivery, `"}]}`)
	require.NoError(t, err)
	delivery.prepare(c)
	require.NoError(t, delivery.err)
	assert.Equal(t, 2, attempts)
	assert.Equal(t, digest.Sum(nil), savedDigest)
	var response struct {
		Usage map[string]int   `json:"usage"`
		Data  []map[string]any `json:"data"`
	}
	_, err = delivery.output.Seek(0, io.SeekStart)
	require.NoError(t, err)
	require.NoError(t, common.DecodeJson(delivery.output, &response))
	require.Len(t, response.Data, 1)
	assert.Equal(t, "kept", response.Data[0]["revised_prompt"])
	assert.NotEmpty(t, response.Data[0]["url"])
	assert.NotContains(t, response.Data[0], "b64_json")
	assert.Equal(t, 7, response.Usage["total_tokens"])
}

func TestImageDeliveryFailureReturnsRecoverableOriginalResults(t *testing.T) {
	for _, original := range []string{
		`{"data":[{"b64_json":"aW52YWxpZC1pbWFnZQ==","revised_prompt":"keep"}]}`,
		`{"data":[{"url":"https://example.invalid/result.png","revised_prompt":"keep"}]}`,
	} {
		t.Run(original[:10], func(t *testing.T) {
			recorder := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(recorder)
			ctx, cancel := context.WithCancel(context.Background())
			cancel()
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/images/generations", nil).WithContext(ctx)
			format := "url"
			if strings.Contains(original, `"url"`) {
				format = "b64_json"
			}
			delivery, deliveryErr := beginImageDelivery(c, &relaycommon.RelayInfo{Request: &dto.ImageRequest{ResponseFormat: format}})
			require.Nil(t, deliveryErr)
			defer delivery.cleanup(c)
			_, err := io.WriteString(delivery, original)
			require.NoError(t, err)
			delivery.prepare(c)
			require.Error(t, delivery.err)
			delivery.send(c)
			assert.Equal(t, 502, recorder.Code)
			var expected, actual map[string]any
			require.NoError(t, common.UnmarshalJsonStr(original, &expected))
			require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &actual))
			assert.Equal(t, expected["data"], actual["data"])
			assert.Equal(t, format, actual["requested_response_format"])
			assert.NotNil(t, actual["error"])
		})
	}
}

func TestImageDeliveryJSONEscapesAndMetadataRemainValid(t *testing.T) {
	file, err := os.CreateTemp(t.TempDir(), "escaped-*")
	require.NoError(t, err)
	defer file.Close()
	raw := `{"data":[{"b64_json":"\u002b\/8=","nested":{"items":["a}b",2]},"revised_prompt":"hello \\\" world"}],"usage":{"total_tokens":4}}`
	_, err = io.WriteString(file, raw)
	require.NoError(t, err)
	fields, err := (imageJSONValue{source: file, size: int64(len(raw))}).object()
	require.NoError(t, err)
	images, err := fields["data"].array()
	require.NoError(t, err)
	require.Len(t, images, 1)
	item, err := images[0].object()
	require.NoError(t, err)
	reader, err := item["b64_json"].base64Reader()
	require.NoError(t, err)
	decoded, err := io.ReadAll(base64.NewDecoder(base64.StdEncoding, reader))
	require.NoError(t, err)
	assert.Equal(t, []byte{251, 255}, decoded)
	var output bytes.Buffer
	require.NoError(t, writeImageJSONObject(&output, fields))
	assert.JSONEq(t, raw, output.String())
}

func TestImageDeliveryJSONAboveFormerResponseLimitIsForwardedWhole(t *testing.T) {
	// Cross the former 512 MiB envelope boundary with one file-backed result.
	// This guards against paid, already-generated images being discarded.
	media, err := os.CreateTemp(t.TempDir(), "large-result-*")
	require.NoError(t, err)
	defer media.Close()
	const mediaSize = int64(384<<20) + 3
	require.NoError(t, media.Truncate(mediaSize))
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/images/generations", nil)
	delivery, deliveryErr := beginImageDelivery(c, &relaycommon.RelayInfo{Request: &dto.ImageRequest{ResponseFormat: "b64_json"}})
	require.Nil(t, deliveryErr)
	defer delivery.cleanup(c)
	require.NoError(t, delivery.err)
	_, err = io.WriteString(delivery, `{"data":[{"b64_json":"`)
	require.NoError(t, err)
	encoder := base64.NewEncoder(base64.StdEncoding, delivery)
	_, err = io.Copy(encoder, io.NewSectionReader(media, 0, mediaSize))
	require.NoError(t, err)
	require.NoError(t, encoder.Close())
	_, err = io.WriteString(delivery, `"}],"created":123}`)
	require.NoError(t, err)
	delivery.prepare(c)
	require.NoError(t, delivery.err)
	stat, err := delivery.output.Stat()
	require.NoError(t, err)
	fields, err := (imageJSONValue{source: delivery.output, size: stat.Size()}).object()
	require.NoError(t, err)
	items, err := fields["data"].array()
	require.NoError(t, err)
	require.Len(t, items, 1)
	item, err := items[0].object()
	require.NoError(t, err)
	reader, err := item["b64_json"].base64Reader()
	require.NoError(t, err)
	digest := sha256.New()
	n, err := io.Copy(digest, base64.NewDecoder(base64.StdEncoding, reader))
	require.NoError(t, err)
	expected := sha256.New()
	_, err = io.Copy(expected, io.NewSectionReader(media, 0, mediaSize))
	require.NoError(t, err)
	assert.Equal(t, mediaSize, n)
	assert.Equal(t, expected.Sum(nil), digest.Sum(nil))
}

func TestImageDeliveryLargeURLDownloadEncodesWholeResult(t *testing.T) {
	service.InitHttpClient()
	png, err := base64.StdEncoding.DecodeString("iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+jRZkAAAAASUVORK5CYII=")
	require.NoError(t, err)
	media, err := os.CreateTemp(t.TempDir(), "download-*")
	require.NoError(t, err)
	defer media.Close()
	_, err = media.Write(png)
	require.NoError(t, err)
	const size = int64(50<<20) + 1
	require.NoError(t, media.Truncate(size))
	downloads := 0
	source := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		downloads++
		if downloads == 1 {
			w.WriteHeader(503)
			return
		}
		_, err := io.Copy(w, io.NewSectionReader(media, 0, size))
		assert.NoError(t, err)
	}))
	defer source.Close()
	fetch := system_setting.GetFetchSetting()
	previous := *fetch
	fetch.EnableSSRFProtection = false
	t.Cleanup(func() { *fetch = previous })
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/images/generations", nil)
	delivery, deliveryErr := beginImageDelivery(c, &relaycommon.RelayInfo{Request: &dto.ImageRequest{ResponseFormat: "b64_json"}})
	require.Nil(t, deliveryErr)
	defer delivery.cleanup(c)
	raw, err := common.Marshal(map[string]any{"data": []map[string]any{{"url": source.URL, "revised_prompt": "keep"}}})
	require.NoError(t, err)
	_, err = delivery.Write(raw)
	require.NoError(t, err)
	delivery.prepare(c)
	require.NoError(t, delivery.err)
	stat, err := delivery.output.Stat()
	require.NoError(t, err)
	fields, err := (imageJSONValue{source: delivery.output, size: stat.Size()}).object()
	require.NoError(t, err)
	items, err := fields["data"].array()
	require.NoError(t, err)
	require.Len(t, items, 1)
	item, err := items[0].object()
	require.NoError(t, err)
	assert.NotContains(t, item, "url")
	reader, err := item["b64_json"].base64Reader()
	require.NoError(t, err)
	actual, expected := sha256.New(), sha256.New()
	n, err := io.Copy(actual, base64.NewDecoder(base64.StdEncoding, reader))
	require.NoError(t, err)
	_, err = io.Copy(expected, io.NewSectionReader(media, 0, size))
	require.NoError(t, err)
	assert.Equal(t, size, n)
	assert.Equal(t, expected.Sum(nil), actual.Sum(nil))
	assert.Equal(t, 2, downloads)
}

func TestImageDeliveryMissingImageIsNotReportedAsSuccess(t *testing.T) {
	for _, value := range []string{"null", `""`, "false", "{}"} {
		t.Run(value, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(recorder)
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/images/generations", nil)
			delivery, deliveryErr := beginImageDelivery(c, &relaycommon.RelayInfo{Request: &dto.ImageRequest{ResponseFormat: "b64_json"}})
			require.Nil(t, deliveryErr)
			defer delivery.cleanup(c)
			_, err := io.WriteString(delivery, `{"data":[{"b64_json":`+value+`}]}`)
			require.NoError(t, err)
			delivery.prepare(c)
			require.Error(t, delivery.err)
			delivery.send(c)
			assert.Equal(t, http.StatusBadGateway, recorder.Code)
		})
	}
}
