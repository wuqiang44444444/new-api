package controller

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/system_setting"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNativeImageDeliverySharesOnlyOriginalSuccessfulResponseEvidence(t *testing.T) {
	const image = "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+jRZkAAAAASUVORK5CYII="
	upstreamBody := `{"data":[{"b64_json":"` + image + `","revised_prompt":"` + strings.Repeat("x", 96<<10) + `"}],"usage":{"input_tokens":1,"output_tokens":1}}`
	writeFailure := errors.New("fixture response writer failed")
	for _, tc := range []struct {
		name, option string
		limit        int
		writeErr     error
		observedErr  error
		shared       bool
	}{
		{name: "unchanged", shared: true},
		{name: "converted", option: `,"response_format":"url"`},
		{name: "short_write", limit: 16, observedErr: io.ErrShortWrite},
		{name: "write_failed_after_accepting_all_bytes", writeErr: writeFailure, observedErr: writeFailure},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fixture := newNativeDiagFixture(t, http.StatusOK, upstreamBody, map[string]string{common.RequestIdKey: "fixture-upstream"}, service.APIRequestEvidence())
			require.NoError(t, fixture.db.AutoMigrate(&model.TaskRequestEvidence{}, &model.TaskRequestEvidenceEvent{}, &model.EvidenceTaskLink{}))
			old := system_setting.GetTaskRequestEvidenceConfig()
			config := system_setting.TaskRequestEvidenceConfig{Enabled: true, RetentionDays: 2, StorageDir: t.TempDir(), EncryptionKeyHex: strings.Repeat("01", 32)}
			system_setting.SetTaskRequestEvidenceConfig(config)
			require.NoError(t, service.InitTaskRequestEvidenceStore(config))
			t.Cleanup(func() {
				flushControllerEvidence(t)
				system_setting.SetTaskRequestEvidenceConfig(old)
				require.NoError(t, service.InitTaskRequestEvidenceStore(old))
			})

			req := httptest.NewRequest(http.MethodPost, "/v1/images/generations", strings.NewReader(`{"model":"gpt-image-2","prompt":"fixture"`+tc.option+`}`))
			req.Header.Set("Content-Type", "application/json")
			response := &imageEvidenceTestResponse{ResponseRecorder: httptest.NewRecorder(), limit: tc.limit, err: tc.writeErr}
			fixture.engine.ServeHTTP(response, req)
			require.Equal(t, http.StatusOK, response.Code)
			assert.EqualValues(t, 1, fixture.calls.Load())
			if tc.option != "" {
				assert.Contains(t, response.Body.String(), `"url":`)
				assert.NotContains(t, response.Body.String(), `"b64_json":`)
			} else if tc.limit > 0 {
				assert.Equal(t, upstreamBody[:tc.limit], response.Body.String())
			} else {
				assert.Equal(t, upstreamBody, response.Body.String())
			}

			flushControllerEvidence(t)
			indexes, _, err := model.QueryTaskRequestEvidence(model.TaskRequestEvidenceQueryParams{Num: 10})
			require.NoError(t, err)
			require.Len(t, indexes, 1)
			events, err := model.ListTaskRequestEvidenceEvents(indexes[0].Id)
			require.NoError(t, err)
			byStage := map[string]*model.TaskRequestEvidenceEvent{}
			for _, event := range events {
				byStage[event.Stage] = event
			}
			upstream := byStage[model.TaskRequestEvidenceStageUpstreamResponse]
			delivery := byStage[model.TaskRequestEvidenceStageClientDelivery]
			require.NotNil(t, upstream)
			require.NotNil(t, delivery)
			assert.NotEqual(t, upstream.Id, delivery.Id)
			upstreamContent, status := service.ReadEvidenceEventContent(upstream, false)
			require.Equal(t, "available", status)
			deliveryContent, status := service.ReadEvidenceEventContent(delivery, false)
			require.Equal(t, "available", status)
			assert.Equal(t, "fixture-upstream", http.Header(upstreamContent.Meta.Headers).Get(common.RequestIdKey))
			assert.Empty(t, http.Header(deliveryContent.Meta.Headers).Get(common.RequestIdKey))
			if tc.observedErr != nil {
				assert.Equal(t, tc.observedErr.Error(), deliveryContent.Meta.Error)
				assert.NotNil(t, byStage["client_write"])
			}

			reader, status := service.OpenEvidenceEventBody(delivery, false)
			require.Equal(t, "available", status)
			download, err := io.ReadAll(reader)
			require.NoError(t, err)
			require.NoError(t, reader.Close())
			assert.EqualValues(t, len(download), reader.(interface{ Size() int64 }).Size())
			_, envelopeAndBody, ok := strings.Cut(string(download), "\n")
			require.True(t, ok)
			_, deliveredBody, ok := strings.Cut(envelopeAndBody, "\n")
			require.True(t, ok)
			assert.Equal(t, response.Body.String(), deliveredBody)

			upstreamKey := service.EvidenceObjectKey(upstream.EvidenceId, upstream.Id)
			deliveryKey := service.EvidenceObjectKey(delivery.EvidenceId, delivery.Id)
			stat, err := os.Stat(filepath.Join(config.StorageDir, deliveryKey))
			require.NoError(t, err)
			if tc.shared {
				assert.Less(t, stat.Size(), int64(len(upstreamBody)), "delivery stores metadata and a reference")
			} else {
				assert.Greater(t, stat.Size(), int64(response.Body.Len()), "delivery stores an independent body")
			}
			_, err = service.GetTaskRequestEvidenceStore().Delete(upstreamKey)
			require.NoError(t, err)
			_, status = service.ReadEvidenceEventContent(delivery, false)
			if tc.shared {
				assert.Equal(t, "missing", status)
			} else {
				assert.Equal(t, "available", status)
			}
		})
	}
}

type imageEvidenceTestResponse struct {
	*httptest.ResponseRecorder
	limit int
	err   error
}

func (w *imageEvidenceTestResponse) Write(p []byte) (int, error) {
	if w.limit > 0 {
		p = p[:min(len(p), w.limit)]
	}
	n, err := w.ResponseRecorder.Write(p)
	if err != nil {
		return n, err
	}
	return n, w.err
}
