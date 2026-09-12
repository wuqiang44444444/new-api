package middleware

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestModelArkReferenceAudioInputForms(t *testing.T) {
	wav := []byte("RIFF\x04\x00\x00\x00WAVE")
	for _, form := range []string{"https", "base64", "data", "file", "missing_file", "unused_file", "json_file"} {
		t.Run(form, func(t *testing.T) {
			ref := "https://source.example/audio.wav?one=1&two=2"
			switch form {
			case "base64":
				ref = base64.StdEncoding.EncodeToString(wav)
			case "data":
				ref = "data:audio/wav;base64," + base64.StdEncoding.EncodeToString(wav)
			case "file", "missing_file", "json_file":
				ref = "file://audio"
			}
			payload := map[string]any{"model": "customer", "content": []any{map[string]any{"type": "image_url", "image_url": map[string]any{"url": "https://source.example/image.png"}, "role": "reference_image"}, map[string]any{"type": "audio_url", "audio_url": map[string]any{"url": ref}, "role": "reference_audio"}}}
			raw, err := common.Marshal(payload)
			require.NoError(t, err)
			var input bytes.Buffer
			ct := "application/json"
			if form == "file" || form == "missing_file" || form == "unused_file" {
				writer := multipart.NewWriter(&input)
				require.NoError(t, writer.WriteField("request", string(raw)))
				if form != "missing_file" {
					part, err := writer.CreateFormFile("audio", "misleading-image.png")
					require.NoError(t, err)
					_, err = part.Write(wav)
					require.NoError(t, err)
				}
				require.NoError(t, writer.Close())
				ct = writer.FormDataContentType()
			} else {
				input.Write(raw)
			}
			engine := gin.New()
			engine.POST("/create", ModelArkVideoCreateConvert(), func(c *gin.Context) {
				contract, ok := relaycommon.GetVideoContractRequest(c)
				require.True(t, ok)
				require.Len(t, contract.ModelArk.Content, 2)
				image, audio := contract.ModelArk.Content[0], contract.ModelArk.Content[1]
				assert.Equal(t, "https://source.example/image.png", image.ImageURL.URL)
				assert.Equal(t, "audio_url", audio.Type)
				assert.Nil(t, audio.ImageURL)
				assert.Equal(t, "reference_audio", *audio.Role)
				if form == "https" || form == "file" {
					assert.Equal(t, ref, audio.AudioURL.URL)
				} else {
					data, mime, err := service.DecodeVideoReferenceAudio(audio.AudioURL.URL)
					require.NoError(t, err)
					assert.Equal(t, wav, data)
					assert.Contains(t, mime, "audio/")
				}
				c.Status(http.StatusNoContent)
			})
			req := httptest.NewRequest(http.MethodPost, "/create", &input)
			req.Header.Set("Content-Type", ct)
			rec := httptest.NewRecorder()
			engine.ServeHTTP(rec, req)
			if form == "missing_file" || form == "unused_file" || form == "json_file" {
				assert.Equal(t, 400, rec.Code, rec.Body.String())
			} else {
				assert.Equal(t, 204, rec.Code, rec.Body.String())
			}
		})
	}
}

func TestReferenceAudioByteLimitAcrossInputForms(t *testing.T) {
	for _, form := range []string{"file", "base64", "data"} {
		for _, delta := range []int{-1, 0, 1} {
			t.Run(fmt.Sprintf("%s/%d", form, delta), func(t *testing.T) {
				data := bytes.Repeat([]byte{0}, service.MaxVideoReferenceAudioBytes+delta)
				ref := "file://audio"
				if form != "file" {
					ref = base64.StdEncoding.EncodeToString(data)
					if form == "data" {
						ref = "data:audio/wav;base64," + ref
					}
				}
				raw, err := common.Marshal(map[string]any{"model": "customer", "content": []any{map[string]any{"type": "audio_url", "audio_url": map[string]any{"url": ref}, "role": "reference_audio"}}})
				require.NoError(t, err)
				var input bytes.Buffer
				ct := "application/json"
				if form == "file" {
					writer := multipart.NewWriter(&input)
					require.NoError(t, writer.WriteField("request", string(raw)))
					part, err := writer.CreateFormFile("audio", "audio.mp3")
					require.NoError(t, err)
					_, err = part.Write(data)
					require.NoError(t, err)
					require.NoError(t, writer.Close())
					ct = writer.FormDataContentType()
				} else {
					input.Write(raw)
				}
				engine := gin.New()
				engine.POST("/create", ModelArkVideoCreateConvert(), func(c *gin.Context) {
					contract, ok := relaycommon.GetVideoContractRequest(c)
					require.True(t, ok)
					assert.Equal(t, ref, contract.ModelArk.Content[0].AudioURL.URL, "ingress must not re-encode the input")
					c.Status(http.StatusNoContent)
				})
				req := httptest.NewRequest("POST", "/create", &input)
				req.Header.Set("Content-Type", ct)
				rec := httptest.NewRecorder()
				engine.ServeHTTP(rec, req)
				if delta > 0 {
					assert.Equal(t, 400, rec.Code)
				} else {
					assert.Equal(t, 204, rec.Code, rec.Body.String())
				}
			})
		}
	}
}

func TestReferenceAudioUnknownMimeDoesNotRejectFileBytes(t *testing.T) {
	for _, data := range [][]byte{{0xff, 0xfb, 0x90, 0x64, 0, 0, 0, 0}, []byte("OggS\x00opaque payload")} {
		for _, form := range []string{"file", "base64"} {
			ref := base64.StdEncoding.EncodeToString(data)
			if form == "file" {
				ref = "file://audio"
			}
			raw, err := common.Marshal(map[string]any{"model": "customer", "content": []any{map[string]any{"type": "audio_url", "audio_url": map[string]any{"url": ref}, "role": "reference_audio"}}})
			require.NoError(t, err)
			var input bytes.Buffer
			ct := "application/json"
			if form == "file" {
				writer := multipart.NewWriter(&input)
				require.NoError(t, writer.WriteField("request", string(raw)))
				part, err := writer.CreateFormFile("audio", "no-id3.mp3")
				require.NoError(t, err)
				_, err = part.Write(data)
				require.NoError(t, err)
				require.NoError(t, writer.Close())
				ct = writer.FormDataContentType()
			} else {
				input.Write(raw)
			}
			engine := gin.New()
			engine.POST("/create", ModelArkVideoCreateConvert(), func(c *gin.Context) { c.Status(http.StatusNoContent) })
			req := httptest.NewRequest("POST", "/create", &input)
			req.Header.Set("Content-Type", ct)
			rec := httptest.NewRecorder()
			engine.ServeHTTP(rec, req)
			assert.Equal(t, 204, rec.Code, rec.Body.String())
		}
	}
}
