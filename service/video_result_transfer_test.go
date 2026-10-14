package service

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/setting/system_setting"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// fakeVideoResultStore 同时实现 imageObjectStore 与 videoResultObjectStore，
// 以内存对象承载上传，便于在不访问真实对象存储的情况下验证转存契约。
type fakeVideoResultStore struct {
	mu      sync.Mutex
	objects map[string][]byte
	mimes   map[string]string
}

func newFakeVideoResultStore() *fakeVideoResultStore {
	return &fakeVideoResultStore{objects: map[string][]byte{}, mimes: map[string]string{}}
}

func (f *fakeVideoResultStore) putImageObject(_ context.Context, objectKey, mimeType string, data []byte) (*ImageObjectRef, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.objects[objectKey] = data
	f.mimes[objectKey] = mimeType
	return &ImageObjectRef{ObjectKey: objectKey, MimeType: mimeType, Size: int64(len(data))}, nil
}

func (f *fakeVideoResultStore) presignImageObjectURL(objectKey string) (string, int64, error) {
	return f.presignVideoResultObjectURL(objectKey, http.MethodGet, time.Minute)
}

func (f *fakeVideoResultStore) headImageObject(_ context.Context, objectKey string) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	_, ok := f.objects[objectKey]
	return ok, nil
}

func (f *fakeVideoResultStore) fetchImageObjectBytes(_ context.Context, objectKey string) ([]byte, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	data, ok := f.objects[objectKey]
	if !ok {
		return nil, errors.New("object missing")
	}
	return data, nil
}

func (f *fakeVideoResultStore) putVideoResultObject(_ context.Context, objectKey, mimeType string, content io.Reader, maxBytes int64) (int64, error) {
	data, err := io.ReadAll(io.LimitReader(content, maxBytes+1))
	if err != nil {
		return 0, err
	}
	if int64(len(data)) > maxBytes {
		return 0, errVideoResultTooLarge
	}
	if len(data) == 0 {
		return 0, errors.New("upstream video result is empty")
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.objects[objectKey] = data
	f.mimes[objectKey] = mimeType
	return int64(len(data)), nil
}

func (f *fakeVideoResultStore) presignVideoResultObjectURL(objectKey, _ string, ttl time.Duration) (string, int64, error) {
	expiresAt := time.Now().Add(ttl).Unix()
	return "https://oss.example.test/presigned/" + objectKey + "?expires=" + fmt.Sprintf("%d", expiresAt), expiresAt, nil
}

func (f *fakeVideoResultStore) videoResultLocation() model.VideoResultStorageLocation {
	return model.VideoResultStorageLocation{Backend: "fake", Endpoint: "https://oss.example.test", Bucket: "videos", Prefix: ""}
}

func videoResultTestContext(store *fakeVideoResultStore) context.Context {
	if store == nil {
		return context.Background()
	}
	return context.WithValue(context.Background(), imageObjectSessionKey{}, &imageObjectSession{store: store})
}

func allowVideoResultPrivateFetch(t *testing.T) {
	t.Helper()
	original := *system_setting.GetFetchSetting()
	system_setting.GetFetchSetting().EnableSSRFProtection = true
	system_setting.GetFetchSetting().AllowPrivateIp = true
	system_setting.GetFetchSetting().AllowedPorts = []string{"1-65535"}
	t.Cleanup(func() { *system_setting.GetFetchSetting() = original })
	InitHttpClient()
}

func newVideoResultTaskForTest(t *testing.T, taskID string, baseURL string) *model.Task {
	t.Helper()
	task := &model.Task{
		TaskID:         taskID,
		Platform:       constant.TaskPlatform(strconv.Itoa(constant.ChannelTypeSeedanceLink)),
		UserId:         71,
		AppID:          900,
		ChannelId:      1,
		ClientProtocol: model.TaskClientProtocolModelArkV3,
		Status:         model.TaskStatusSuccess,
		FinishTime:     common.GetTimestamp(),
		SubmitTime:     common.GetTimestamp(),
		PrivateData: model.TaskPrivateData{
			ResultURL:                 baseURL + "/video.mp4",
			Key:                       "frozen-key",
			UpstreamTaskID:            "provider-task-1",
			VideoUpstreamQueryBaseURL: baseURL,
		},
	}
	require.NoError(t, model.DB.Create(task).Error)
	t.Cleanup(func() {
		_ = model.DB.Where("task_id = ?", task.TaskID).Delete(&model.TaskVideoResultObject{}).Error
		_ = model.DB.Unscoped().Delete(&model.Task{}, "task_id = ?", task.TaskID).Error
	})
	return task
}

func videoResultRowForTest(t *testing.T, taskID, part string) *model.TaskVideoResultObject {
	t.Helper()
	rows, err := model.ListVideoResultObjects(taskID)
	require.NoError(t, err)
	for i := range rows {
		if rows[i].Part == part {
			return &rows[i]
		}
	}
	t.Fatalf("video result row %s not found", part)
	return nil
}

// runTransferPassesUntil 反复执行转存 pass 直到目标行离开 pending 或次数用
// 尽：共享测试库中其它用例的成功 Link 任务会被补扫登记并按 id 先行领取，
// 单轮 pass 的汇总计数不可作断言依据，只有行状态是确定的。
func runTransferPassesUntil(t *testing.T, ctx context.Context, taskID, part string, wantStatus string, maxPasses int) {
	t.Helper()
	for i := 0; i < maxPasses; i++ {
		row := videoResultRowForTest(t, taskID, part)
		if row.Status != model.TaskVideoResultStatusPending {
			return
		}
		RunVideoResultTransferPass(ctx, nil)
	}
}

func TestVideoResultTransferScopedTable(t *testing.T) {
	seedance := &model.Task{Platform: constant.TaskPlatform(strconv.Itoa(constant.ChannelTypeSeedanceLink)), ClientProtocol: model.TaskClientProtocolModelArkV3}
	minimax := &model.Task{Platform: constant.TaskPlatform(strconv.Itoa(constant.ChannelTypeMiniMaxLink)), ClientProtocol: model.TaskClientProtocolModelArkV3}
	assert.True(t, VideoResultTransferScoped(seedance))
	assert.True(t, VideoResultTransferScoped(minimax))
	assert.False(t, VideoResultTransferScoped(&model.Task{ClientProtocol: model.TaskClientProtocolKlingV1}))
	assert.False(t, VideoResultTransferScoped(&model.Task{Platform: seedance.Platform}))
	assert.False(t, VideoResultTransferScoped(nil))
}

func TestRunVideoResultTransferPassTransfersAndSigns(t *testing.T) {
	allowVideoResultPrivateFetch(t)
	store := newFakeVideoResultStore()
	var authHeader string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authHeader = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "video/mp4")
		_, _ = w.Write([]byte("video-bytes-123"))
	}))
	t.Cleanup(upstream.Close)
	task := newVideoResultTaskForTest(t, "task_transfer_ready", upstream.URL)
	ctx := videoResultTestContext(store)

	RegisterVideoResultTransfer(ctx, task)
	row := videoResultRowForTest(t, task.TaskID, model.TaskVideoResultPartVideo)
	require.Equal(t, model.TaskVideoResultStatusPending, row.Status)

	runTransferPassesUntil(t, ctx, task.TaskID, model.TaskVideoResultPartVideo, model.TaskVideoResultStatusReady, 20)

	row = videoResultRowForTest(t, task.TaskID, model.TaskVideoResultPartVideo)
	require.Equal(t, model.TaskVideoResultStatusReady, row.Status)
	assert.Equal(t, "videos/tasks/"+task.TaskID+"/video", row.ObjectKey)
	assert.Equal(t, "video/mp4", row.MimeType)
	assert.Equal(t, int64(len("video-bytes-123")), row.SizeBytes)
	assert.Equal(t, "fake", row.Location.Backend)
	assert.Empty(t, authHeader, "plain lines must not receive frozen credentials")

	objectKey := row.ObjectKey
	store.mu.Lock()
	uploaded, ok := store.objects[objectKey]
	store.mu.Unlock()
	require.True(t, ok)
	assert.Equal(t, "video-bytes-123", string(uploaded))

	url, expiresAt, err := PresignVideoResultDownload(ctx, row, http.MethodGet)
	require.NoError(t, err)
	assert.Contains(t, url, objectKey)
	assert.InDelta(t, time.Now().Add(24*time.Hour).Unix(), expiresAt, 90)
}

func TestRunVideoResultTransferPassFeicaiCredentialAndSameOrigin(t *testing.T) {
	allowVideoResultPrivateFetch(t)
	store := newFakeVideoResultStore()
	var authHeader, requestPath string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authHeader = r.Header.Get("Authorization")
		requestPath = r.URL.Path
		w.Header().Set("Content-Type", "video/mp4")
		_, _ = w.Write([]byte("feicai-video"))
	}))
	t.Cleanup(upstream.Close)
	task := newVideoResultTaskForTest(t, "task_transfer_feicai", upstream.URL)
	task.PrivateData.VideoUpstreamProfile = "third_party_feicai_videos"
	task.PrivateData.SouthboundAdapterVersion = relaycommon.CurrentVideoSouthboundAdapterVersion(constant.ChannelTypeSeedanceLink, "third_party_feicai_videos")
	require.NoError(t, model.DB.Save(task).Error)
	ctx := videoResultTestContext(store)

	RegisterVideoResultTransfer(ctx, task)
	runTransferPassesUntil(t, ctx, task.TaskID, model.TaskVideoResultPartVideo, model.TaskVideoResultStatusReady, 20)
	assert.Equal(t, "Bearer frozen-key", authHeader, "feicai main video must transfer with frozen credential")
	assert.Equal(t, "/video.mp4", requestPath)
}

func TestRunVideoResultTransferPassSynlinkLastFrame(t *testing.T) {
	allowVideoResultPrivateFetch(t)
	store := newFakeVideoResultStore()
	var authHeader, requestPath string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authHeader = r.Header.Get("Authorization")
		requestPath = r.URL.Path
		w.Header().Set("Content-Type", "image/jpeg")
		_, _ = w.Write([]byte("frame-jpeg"))
	}))
	t.Cleanup(upstream.Close)
	task := newVideoResultTaskForTest(t, "task_transfer_frame", upstream.URL)
	task.PrivateData.VideoUpstreamProtocol = "synlink_video_v1"
	task.PrivateData.VideoUpstreamProfile = "third_party_synlink_video_v1"
	task.PrivateData.SouthboundAdapterVersion = relaycommon.CurrentVideoSouthboundAdapterVersion(constant.ChannelTypeSeedanceLink, "third_party_synlink_video_v1")
	returnTrue := true
	task.PrivateData.ClientRequest = &model.TaskClientRequestSnapshot{ReturnLastFrame: &returnTrue}
	task.Data = []byte(`{"content":{"last_frame_url":"https://provider.example/frame"}}`)
	require.NoError(t, model.DB.Save(task).Error)
	ctx := videoResultTestContext(store)

	RegisterVideoResultTransfer(ctx, task)
	runTransferPassesUntil(t, ctx, task.TaskID, model.TaskVideoResultPartVideo, model.TaskVideoResultStatusReady, 20)

	frame := videoResultRowForTest(t, task.TaskID, model.TaskVideoResultPartLastFrame)
	require.Equal(t, model.TaskVideoResultStatusReady, frame.Status, "main video and last frame transfer independently")
	assert.Equal(t, "image/jpeg", frame.MimeType)
	assert.Equal(t, "/v1/video/files/provider-task-1/last-frame", requestPath)
	assert.Equal(t, "Bearer frozen-key", authHeader)
}

func TestRunVideoResultTransferPassFailureClassification(t *testing.T) {
	allowVideoResultPrivateFetch(t)
	t.Run("upstream gone marks unavailable", func(t *testing.T) {
		store := newFakeVideoResultStore()
		upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusNotFound)
		}))
		t.Cleanup(upstream.Close)
		task := newVideoResultTaskForTest(t, "task_transfer_gone", upstream.URL)
		ctx := videoResultTestContext(store)
		RegisterVideoResultTransfer(ctx, task)

		runTransferPassesUntil(t, ctx, task.TaskID, model.TaskVideoResultPartVideo, model.TaskVideoResultStatusUnavailable, 20)
		row := videoResultRowForTest(t, task.TaskID, model.TaskVideoResultPartVideo)
		assert.Equal(t, model.TaskVideoResultStatusUnavailable, row.Status)
	})

	t.Run("server error stays retryable", func(t *testing.T) {
		store := newFakeVideoResultStore()
		upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
		}))
		t.Cleanup(upstream.Close)
		task := newVideoResultTaskForTest(t, "task_transfer_500", upstream.URL)
		ctx := videoResultTestContext(store)
		RegisterVideoResultTransfer(ctx, task)

		runTransferPassesUntil(t, ctx, task.TaskID, model.TaskVideoResultPartVideo, model.TaskVideoResultStatusTransferring, 1)
		row := videoResultRowForTest(t, task.TaskID, model.TaskVideoResultPartVideo)
		assert.Equal(t, model.TaskVideoResultStatusPending, row.Status)
		assert.NotEmpty(t, row.NextAttemptAt)
	})

	t.Run("oversize marks unavailable", func(t *testing.T) {
		previous := videoResultMaxBytes
		videoResultMaxBytes = 8
		t.Cleanup(func() { videoResultMaxBytes = previous })
		store := newFakeVideoResultStore()
		upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte("this payload is longer than eight bytes"))
		}))
		t.Cleanup(upstream.Close)
		task := newVideoResultTaskForTest(t, "task_transfer_large", upstream.URL)
		ctx := videoResultTestContext(store)
		RegisterVideoResultTransfer(ctx, task)

		runTransferPassesUntil(t, ctx, task.TaskID, model.TaskVideoResultPartVideo, model.TaskVideoResultStatusUnavailable, 20)
		row := videoResultRowForTest(t, task.TaskID, model.TaskVideoResultPartVideo)
		assert.Equal(t, model.TaskVideoResultStatusUnavailable, row.Status)
		store.mu.Lock()
		_, uploaded := store.objects["videos/tasks/"+task.TaskID+"/video"]
		store.mu.Unlock()
		assert.False(t, uploaded, "oversize content must never become a stored object")
	})
}

func TestRunVideoResultTransferPassWithoutStorageReschedules(t *testing.T) {
	allowVideoResultPrivateFetch(t)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("video-bytes"))
	}))
	t.Cleanup(upstream.Close)
	task := newVideoResultTaskForTest(t, "task_transfer_nostore", upstream.URL)
	ctx := context.Background()
	RegisterVideoResultTransfer(ctx, task)

	summary := RunVideoResultTransferPass(ctx, nil)
	row := videoResultRowForTest(t, task.TaskID, model.TaskVideoResultPartVideo)
	if row.Status != model.TaskVideoResultStatusPending {
		// 行被其它来源挤占时本轮未领取，仍以“未就绪”为准。
		assert.NotEqual(t, model.TaskVideoResultStatusReady, row.Status)
	}
	_ = summary
}

func TestVideoResultSourceRejectsRedirectsForCredentialedLines(t *testing.T) {
	allowVideoResultPrivateFetch(t)
	store := newFakeVideoResultStore()
	var foreignCalls int
	foreign := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		foreignCalls++
		_, _ = w.Write([]byte("redirected"))
	}))
	t.Cleanup(foreign.Close)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, foreign.URL, http.StatusFound)
	}))
	t.Cleanup(upstream.Close)
	task := newVideoResultTaskForTest(t, "task_transfer_redirect", upstream.URL)
	task.PrivateData.VideoUpstreamProfile = "third_party_feicai_videos"
	task.PrivateData.SouthboundAdapterVersion = relaycommon.CurrentVideoSouthboundAdapterVersion(constant.ChannelTypeSeedanceLink, "third_party_feicai_videos")
	require.NoError(t, model.DB.Save(task).Error)
	ctx := videoResultTestContext(store)

	RegisterVideoResultTransfer(ctx, task)
	runTransferPassesUntil(t, ctx, task.TaskID, model.TaskVideoResultPartVideo, model.TaskVideoResultStatusTransferring, 1)
	assert.Zero(t, foreignCalls, "credentialed cross-origin redirects must not be followed")
	row := videoResultRowForTest(t, task.TaskID, model.TaskVideoResultPartVideo)
	assert.NotEqual(t, model.TaskVideoResultStatusReady, row.Status)
}

func TestVideoResultSynlinkLastFrameFailClosedWithoutFrozenFacts(t *testing.T) {
	allowVideoResultPrivateFetch(t)
	store := newFakeVideoResultStore()
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("frame"))
	}))
	t.Cleanup(upstream.Close)
	task := newVideoResultTaskForTest(t, "task_transfer_frame_key", upstream.URL)
	task.PrivateData.VideoUpstreamProtocol = "synlink_video_v1"
	task.PrivateData.VideoUpstreamProfile = "third_party_synlink_video_v1"
	task.PrivateData.SouthboundAdapterVersion = relaycommon.CurrentVideoSouthboundAdapterVersion(constant.ChannelTypeSeedanceLink, "third_party_synlink_video_v1")
	// 创建快照未请求尾帧：主视频照常转存，尾帧按冻结事实失败关闭。
	returnFalse := false
	task.PrivateData.ClientRequest = &model.TaskClientRequestSnapshot{ReturnLastFrame: &returnFalse}
	task.Data = []byte(`{"content":{"last_frame_url":"https://provider.example/frame"}}`)
	require.NoError(t, model.DB.Save(task).Error)
	ctx := videoResultTestContext(store)

	RegisterVideoResultTransfer(ctx, task)
	runTransferPassesUntil(t, ctx, task.TaskID, model.TaskVideoResultPartVideo, model.TaskVideoResultStatusReady, 20)
	frame := videoResultRowForTest(t, task.TaskID, model.TaskVideoResultPartLastFrame)
	assert.Equal(t, model.TaskVideoResultStatusUnavailable, frame.Status, "missing frozen last frame facts fail closed")
	videoRow := videoResultRowForTest(t, task.TaskID, model.TaskVideoResultPartVideo)
	assert.Equal(t, model.TaskVideoResultStatusReady, videoRow.Status, "main video transfers without a last frame credential")
}

func TestMiniMaxResolveFailureStaysRetryable(t *testing.T) {
	allowVideoResultPrivateFetch(t)
	store := newFakeVideoResultStore()
	task := newVideoResultTaskForTest(t, "task_transfer_minimax_retry", "https://minimax-upstream.example.invalid")
	task.Platform = constant.TaskPlatform(strconv.Itoa(constant.ChannelTypeMiniMaxLink))
	require.NoError(t, model.DB.Save(task).Error)
	previous := ResolveMiniMaxVideoContentFunc
	ResolveMiniMaxVideoContentFunc = func(_ context.Context, _ *model.Task) (string, error) {
		return "", errors.New("resolve transport failed")
	}
	t.Cleanup(func() { ResolveMiniMaxVideoContentFunc = previous })
	ctx := videoResultTestContext(store)

	RegisterVideoResultTransfer(ctx, task)
	runTransferPassesUntil(t, ctx, task.TaskID, model.TaskVideoResultPartVideo, model.TaskVideoResultStatusTransferring, 20)
	row := videoResultRowForTest(t, task.TaskID, model.TaskVideoResultPartVideo)
	assert.Equal(t, model.TaskVideoResultStatusPending, row.Status,
		"a single resolve failure must stay retryable with backoff, never turn unavailable")
	assert.NotContains(t, row.LastError, "invalid")
}

func TestVideoResultTransfersNonSynlinkLastFrames(t *testing.T) {
	allowVideoResultPrivateFetch(t)
	for _, protocol := range []string{"funcloud_modelark_v3", "vidu_modelark_v3", "modelark_v3_volcengine", "modelark_v3_byteplus", "moxing_modelark_media_v1"} {
		t.Run(protocol, func(t *testing.T) {
			store := newFakeVideoResultStore()
			var auth string
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				auth = r.Header.Get("Authorization")
				w.Header().Set("Content-Type", "image/png")
				_, _ = io.WriteString(w, "frame-bytes")
			}))
			t.Cleanup(server.Close)
			task := newVideoResultTaskForTest(t, "task_tail_"+protocol, server.URL)
			task.PrivateData.VideoUpstreamProtocol = dto.VideoUpstreamProtocol(protocol)
			data, err := common.Marshal(map[string]any{"content": map[string]string{"last_frame_url": server.URL + "/frame.png"}})
			require.NoError(t, err)
			task.Data = data
			require.NoError(t, model.DB.Save(task).Error)
			source, err := resolveVideoResultSource(context.Background(), task, model.TaskVideoResultPartLastFrame)
			require.NoError(t, err)
			defer source.close()
			size, err := store.putVideoResultObject(context.Background(), "videos/tasks/"+task.TaskID+"/last_frame", source.contentType, source.body, videoResultMaxBytes)
			require.NoError(t, err)
			assert.EqualValues(t, len("frame-bytes"), size)
			assert.Empty(t, auth, "Provider credentials must not accompany the CDN reference")
			assert.Equal(t, "image/png", source.contentType)
		})
	}
}

func TestVideoResultRegistrationUsesDurableSuccess(t *testing.T) {
	task := newVideoResultTaskForTest(t, "task_uncommitted_success", "https://provider.example")
	require.NoError(t, model.DB.Model(task).Update("status", model.TaskStatusInProgress).Error)
	RegisterVideoResultTransfer(context.Background(), task)
	rows, err := model.ListVideoResultObjects(task.TaskID)
	require.NoError(t, err)
	assert.Empty(t, rows, "an in-memory success must not establish delivery facts")
	require.NoError(t, model.DB.Model(task).Update("status", model.TaskStatusSuccess).Error)
	RegisterVideoResultTransfer(context.Background(), task)
	rows, err = model.ListVideoResultObjects(task.TaskID)
	require.NoError(t, err)
	require.Len(t, rows, 1)
}

func TestVideoResultSourceRejectsPartialFile(t *testing.T) {
	allowVideoResultPrivateFetch(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Range", "bytes 0-2/100")
		w.WriteHeader(http.StatusPartialContent)
		_, _ = io.WriteString(w, "abc")
	}))
	t.Cleanup(server.Close)
	task := newVideoResultTaskForTest(t, "task_partial_file", server.URL)
	source, err := resolveVideoResultSource(context.Background(), task, model.TaskVideoResultPartVideo)
	require.Error(t, err)
	assert.Nil(t, source, "a partial response must not become a complete result")
}

func TestVideoResultSchedulerRecoversWithoutRegisteredWork(t *testing.T) {
	previousDB := model.DB
	require.NoError(t, model.DB.AutoMigrate(&model.Option{}))
	previousConfig, err := model.GetObjectStorageSetting()
	require.NoError(t, err)
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	require.NoError(t, db.AutoMigrate(&model.Task{}, &model.TaskVideoResultObject{}, &model.Option{}))
	model.DB = db
	t.Cleanup(func() { model.DB = previousDB; model.NotifyObjectStorageSettingUpdate(previousConfig); sqlDB.Close() })
	config, err := common.Marshal(system_setting.ObjectStorageConfig{
		Backend: system_setting.ObjectStorageBackendS3, Endpoint: "https://oss.example.invalid",
		Bucket: "videos", Region: "us-east-1", AccountName: "test-key", Credential: "test-secret", Revision: common.GetUUID(),
	})
	require.NoError(t, err)
	require.NoError(t, model.SaveObjectStorageSetting(string(config)))
	task := newVideoResultTaskForTest(t, "task_missing_all_registration", "https://provider.example")
	count, err := model.CountVideoResultTransferWork(common.GetTimestamp())
	require.NoError(t, err)
	require.Zero(t, count)
	require.True(t, (videoResultTransferHandler{}).Enabled(), "the registration gap itself must enable recovery")
	assert.Equal(t, 1, runVideoResultSweep(context.Background()))
	rows, err := model.ListVideoResultObjects(task.TaskID)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	assert.Equal(t, model.TaskVideoResultStatusPending, rows[0].Status)
}

func TestVideoResultPollingRegistersOnlyCommittedSuccess(t *testing.T) {
	for _, lost := range []bool{false, true} {
		t.Run(fmt.Sprint(lost), func(t *testing.T) {
			task := newVideoResultTaskForTest(t, fmt.Sprintf("task_poll_commit_%t", lost), "https://provider.example")
			task.Status = model.TaskStatusInProgress
			task.FinishTime = 0
			require.NoError(t, model.DB.Save(task).Error)
			if lost {
				require.NoError(t, model.DB.Model(task).Update("status", model.TaskStatusFailure).Error)
				task.Status = model.TaskStatusInProgress
			}
			channel, ok := model.FrozenVideoTaskChannel(task)
			require.True(t, ok)
			adaptor := &scriptedPollingAdaptor{parse: &relaycommon.TaskInfo{TaskID: task.GetUpstreamTaskID(), Status: model.TaskStatusSuccess, Url: "https://provider.example/video.mp4"}}
			require.NoError(t, updateVideoSingleTask(context.Background(), adaptor, channel, task.GetUpstreamTaskID(), map[string]*model.Task{task.GetUpstreamTaskID(): task}))
			rows, err := model.ListVideoResultObjects(task.TaskID)
			require.NoError(t, err)
			if lost {
				assert.Empty(t, rows, "a losing success observation must not register objects")
			} else {
				require.Len(t, rows, 1)
				assert.Equal(t, model.TaskVideoResultStatusPending, rows[0].Status)
			}
		})
	}
}
