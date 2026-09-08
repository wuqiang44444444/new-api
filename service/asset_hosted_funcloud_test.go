package service

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image"
	"image/png"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	assetadapter "github.com/QuantumNous/new-api/relay/channel/task/seedance/assets"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

type fakeHostedImageStore struct {
	puts      map[string][]byte
	mimeTypes map[string]string
	presigned map[string]string
	failPut   bool
	failHead  bool
	location  model.FunCloudHostedStorageLocation
}

func newFakeHostedImageStore() *fakeHostedImageStore {
	return &fakeHostedImageStore{
		location:  model.FunCloudHostedStorageLocation{Backend: "s3", Endpoint: "https://store.example", Bucket: "images"},
		puts:      map[string][]byte{},
		mimeTypes: map[string]string{},
		presigned: map[string]string{},
	}
}

func (f *fakeHostedImageStore) putImageObject(_ context.Context, objectKey, mimeType string, data []byte) (*ImageObjectRef, error) {
	if f.failPut {
		return nil, errors.New("put failed")
	}
	f.puts[objectKey] = data
	f.mimeTypes[objectKey] = mimeType
	return &ImageObjectRef{ObjectKey: objectKey, MimeType: mimeType, Size: int64(len(data))}, nil
}

func (f *fakeHostedImageStore) presignImageObjectURL(objectKey string) (string, int64, error) {
	return "https://store.example/" + objectKey, 0, nil
}

func (f *fakeHostedImageStore) headImageObject(_ context.Context, objectKey string) (bool, error) {
	if f.failHead {
		return false, errors.New("head unavailable")
	}
	_, ok := f.puts[objectKey]
	return ok, nil
}

func (f *fakeHostedImageStore) fetchImageObjectBytes(_ context.Context, objectKey string) ([]byte, error) {
	data, ok := f.puts[objectKey]
	if !ok {
		return nil, errors.New("missing object")
	}
	return data, nil
}

func (f *fakeHostedImageStore) presignHostedAssetURL(objectKey string, _ time.Duration) (string, error) {
	if url, ok := f.presigned[objectKey]; ok {
		return url, nil
	}
	return "https://store.example/signed/" + objectKey, nil
}

func (f *fakeHostedImageStore) hostedAssetLocation() model.FunCloudHostedStorageLocation {
	return f.location
}

func withHostedAssetDB(t *testing.T) *gorm.DB {
	t.Helper()
	previousDB := model.DB
	previousType := common.MainDatabaseType()
	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "_"))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.FunCloudHostedAssetGroup{}, &model.FunCloudHostedAsset{}))
	model.DB = db
	common.SetMainDatabaseType(common.DatabaseTypeSQLite)
	t.Cleanup(func() {
		model.DB = previousDB
		common.SetMainDatabaseType(previousType)
	})
	return db
}

func withHostedImageSession(ctx context.Context, store *fakeHostedImageStore) context.Context {
	session := &imageObjectSession{store: store}
	return context.WithValue(ctx, imageObjectSessionKey{}, session)
}

func hostedImageCreateRequest(t *testing.T, groupID string) assetadapter.AssetRequest {
	t.Helper()
	var data bytes.Buffer
	require.NoError(t, png.Encode(&data, image.NewRGBA(image.Rect(0, 0, 2, 2))))
	return assetadapter.AssetRequest{
		GroupResourceID: groupID,
		URL:             "https://source.example/image.png",
		Name:            "hosted-image",
		MediaType:       "image",
		Source:          bytes.NewReader(data.Bytes()),
		SourceType:      "image/png",
		SourceMaxBytes:  funCloudMaterialMaxBytes,
	}
}

// 同账号创建→查询→删除的完整生命周期：删除只停止新引用，不删除对象。
func TestFunCloudHostedAssetLifecycleAndCrossUserIsolation(t *testing.T) {
	withHostedAssetDB(t)
	store := newFakeHostedImageStore()
	ctx := withHostedImageSession(context.Background(), store)
	owner := newFunCloudHostedMaterialAdapter(7, "customer-model")
	other := newFunCloudHostedMaterialAdapter(8, "customer-model")

	created, err := owner.CreateAsset(ctx, hostedImageCreateRequest(t, ""))
	require.NoError(t, err)
	assert.True(t, strings.HasPrefix(created.ResourceID, model.FunCloudHostedAssetIDPrefix))
	assert.Equal(t, "asset_uri_id", created.ReferenceType)
	assert.Equal(t, created.ResourceID, created.ReferenceValue)
	assert.Equal(t, "ready", created.Status)
	require.Len(t, store.puts, 1)
	_, err = png.Decode(bytes.NewReader(firstHostedStoredBytes(t, store)))
	require.NoError(t, err)
	assert.Equal(t, "image/png", onlyHostedMimeType(t, store))

	fetched, err := owner.GetAsset(ctx, created.ResourceID)
	require.NoError(t, err)
	assert.Equal(t, "ready", fetched.Status)

	_, err = other.GetAsset(ctx, created.ResourceID)
	assert.ErrorIs(t, err, assetadapter.ErrAssetResourceNotFound)
	assert.Error(t, other.DeleteAsset(ctx, created.ResourceID))

	require.NoError(t, owner.DeleteAsset(ctx, created.ResourceID))
	_, err = owner.GetAsset(ctx, created.ResourceID)
	assert.ErrorIs(t, err, assetadapter.ErrAssetResourceNotFound)
	assert.Len(t, store.puts, 1)
	assert.True(t, hostedAssetSupportsOnlyGeneralImages(owner))
}

// 显式素材组按 user_id 校验归属；跨用户的组一律按不存在处理。
func TestFunCloudHostedAssetGroupOwnership(t *testing.T) {
	withHostedAssetDB(t)
	store := newFakeHostedImageStore()
	ctx := withHostedImageSession(context.Background(), store)
	owner := newFunCloudHostedMaterialAdapter(11, "customer-model")
	other := newFunCloudHostedMaterialAdapter(12, "customer-model")

	group, err := owner.CreateGroup(ctx, assetadapter.GroupRequest{Name: "my-group"})
	require.NoError(t, err)
	assert.True(t, strings.HasPrefix(group.ResourceID, model.FunCloudHostedGroupIDPrefix))

	_, err = owner.CreateAsset(ctx, hostedImageCreateRequest(t, group.ResourceID))
	require.NoError(t, err)

	fetched, err := owner.GetGroup(ctx, group.ResourceID)
	require.NoError(t, err)
	assert.Equal(t, "my-group", fetched.Name)

	_, err = other.GetGroup(ctx, group.ResourceID)
	assert.ErrorIs(t, err, assetadapter.ErrAssetResourceNotFound)
	_, err = other.CreateAsset(ctx, hostedImageCreateRequest(t, group.ResourceID))
	assert.ErrorIs(t, err, assetadapter.ErrAssetResourceNotFound)

	_, err = owner.CreateAsset(ctx, hostedImageCreateRequest(t, "not-hosted-group"))
	assert.ErrorIs(t, err, assetadapter.ErrAssetResourceNotFound)

	_, err = owner.CreateAsset(ctx, hostedImageCreateRequest(t, ""))
	require.NoError(t, err)
}

func firstHostedStoredBytes(t *testing.T, store *fakeHostedImageStore) []byte {
	t.Helper()
	for _, data := range store.puts {
		return data
	}
	return nil
}

func onlyHostedMimeType(t *testing.T, store *fakeHostedImageStore) string {
	t.Helper()
	for _, mimeType := range store.mimeTypes {
		return mimeType
	}
	return ""
}

func hostedAssetSupportsOnlyGeneralImages(adapter assetadapter.Adapter) bool {
	return adapter.Supports(model.AssetKindGeneral, "image") &&
		!adapter.Supports(model.AssetKindGeneral, "video") &&
		!adapter.Supports(model.AssetKindRealPerson, "image")
}

func hostedVideoContext(t *testing.T, hostedProtocol bool, refs ...string) (*gin.Context, *relaycommon.RelayInfo) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/api/v3/contents/generations/tasks", nil)
	content := make([]dto.ModelArkVideoContent, 0, len(refs))
	for _, ref := range refs {
		content = append(content, dto.ModelArkVideoContent{
			Type: "image_url", Role: common.GetPointer("reference_image"),
			ImageURL: &dto.VideoMediaURL{URL: ref},
		})
	}
	relaycommon.SetVideoContractRequest(c, dto.VideoContractRequest{
		ContractID: dto.VideoContractModelArkV3,
		ModelArk:   &dto.ModelArkVideoCreateRequest{Model: "customer-model", Content: content},
	})
	settings := dto.ChannelOtherSettings{VideoUpstreamProtocol: dto.VideoUpstreamProtocolFunCloudModelArkV3}
	if hostedProtocol {
		settings.AssetUpstreamProtocol = dto.AssetUpstreamProtocolFunCloudHosted
	} else {
		settings.AssetUpstreamProtocol = dto.AssetUpstreamProtocolFunCloudMaterial
	}
	info := &relaycommon.RelayInfo{
		ChannelMeta:   &relaycommon.ChannelMeta{ChannelType: constant.ChannelTypeSeedanceLink, ChannelOtherSettings: settings},
		TaskRelayInfo: &relaycommon.TaskRelayInfo{},
		UserId:        7,
	}
	return c, info
}

// 托管引用在 hold 之前按 user_id 校验；跨用户或不存在的引用明确拒绝且不
// 泄漏存在性；非托管协议保持 no-op；校验通过后事实进入上下文并可冻结。
func TestValidateFunCloudHostedVideoMediaOwnershipAndFreeze(t *testing.T) {
	db := withHostedAssetDB(t)
	store := newFakeHostedImageStore()
	store.puts["assets/funcloud-hosted/known.png"] = []byte("stored")
	require.NoError(t, db.Create(&model.FunCloudHostedAsset{
		ID: model.FunCloudHostedAssetIDPrefix + "known", UserID: 7, Model: "customer-model",
		Name: "known", ObjectKey: "assets/funcloud-hosted/known.png", MimeType: "image/png",
		StorageLocation: store.location, SizeBytes: 11, Status: model.FunCloudHostedAssetStatusReady,
	}).Error)

	owner, ownerInfo := hostedVideoContext(t, true, "asset://"+model.FunCloudHostedAssetIDPrefix+"known")
	owner.Request = owner.Request.WithContext(withHostedImageSession(owner.Request.Context(), store))
	require.Nil(t, ValidateFunCloudHostedVideoMedia(owner, ownerInfo))
	facts := GetFunCloudHostedMediaFacts(owner)
	require.Len(t, facts, 1)
	fact := facts["asset://"+model.FunCloudHostedAssetIDPrefix+"known"]
	assert.Equal(t, "assets/funcloud-hosted/known.png", fact.ObjectKey)

	task := &model.Task{}
	StageFunCloudHostedMediaSnapshot(owner, task)
	require.Len(t, task.PrivateData.HostedMedia, 1)
	assert.Equal(t, model.FunCloudHostedAssetIDPrefix+"known", task.PrivateData.HostedMedia[0].AssetID)
	require.NotEmpty(t, task.PrivateData.HostedMedia[0].ObjectKey)

	missing, missingInfo := hostedVideoContext(t, true, "asset://"+model.FunCloudHostedAssetIDPrefix+"missing")
	taskErr := ValidateFunCloudHostedVideoMedia(missing, missingInfo)
	require.NotNil(t, taskErr)
	assert.Equal(t, "invalid_video_parameter", taskErr.Code)

	otherCtx, otherInfo := hostedVideoContext(t, true, "asset://"+model.FunCloudHostedAssetIDPrefix+"known")
	otherInfo.UserId = 8
	taskErr = ValidateFunCloudHostedVideoMedia(otherCtx, otherInfo)
	require.NotNil(t, taskErr)
	assert.Equal(t, "invalid_video_parameter", taskErr.Code)

	passthrough, passthroughInfo := hostedVideoContext(t, true, "asset://opaque-upstream-id")
	require.Nil(t, ValidateFunCloudHostedVideoMedia(passthrough, passthroughInfo))
	assert.Empty(t, GetFunCloudHostedMediaFacts(passthrough))

	notHosted, notHostedInfo := hostedVideoContext(t, false, "asset://"+model.FunCloudHostedAssetIDPrefix+"known")
	require.NotNil(t, ValidateFunCloudHostedVideoMedia(notHosted, notHostedInfo))
	assert.Empty(t, GetFunCloudHostedMediaFacts(notHosted))
}

// 内部临时 URL 由存储实现按托管 TTL 签发；存储禁用时失败关闭。
func TestSignFunCloudHostedAssetURLUsesStorePresigner(t *testing.T) {
	store := newFakeHostedImageStore()
	store.presigned["assets/funcloud-hosted/known.png"] = "https://store.example/signed-url"
	ctx := withHostedImageSession(context.Background(), store)

	url, err := SignFunCloudHostedAssetURL(ctx, model.TaskHostedMediaFact{ObjectKey: "assets/funcloud-hosted/known.png", StorageLocation: store.location})
	require.NoError(t, err)
	assert.Equal(t, "https://store.example/signed-url", url)

	_, err = SignFunCloudHostedAssetURL(context.Background(), model.TaskHostedMediaFact{ObjectKey: "assets/funcloud-hosted/other.png", StorageLocation: store.location})
	assert.Error(t, err)
}

func TestHostedAssetRejectsInvalidImagesBeforeStorage(t *testing.T) {
	db := withHostedAssetDB(t)
	for _, input := range []string{"empty", "fake", "wrong_mime", "truncated"} {
		t.Run(input, func(t *testing.T) {
			store := newFakeHostedImageStore()
			req := hostedImageCreateRequest(t, "")
			switch input {
			case "empty":
				req.Source = strings.NewReader("")
			case "fake":
				req.Source = strings.NewReader("not an image")
			case "wrong_mime":
				req.SourceType = "image/jpeg"
			case "truncated":
				data, err := io.ReadAll(req.Source)
				require.NoError(t, err)
				req.Source = bytes.NewReader(data[:len(data)-12])
			}
			result, err := newFunCloudHostedMaterialAdapter(7, "model").CreateAsset(withHostedImageSession(t.Context(), store), req)
			require.ErrorIs(t, err, ErrInvalidAssetRequest)
			assert.Empty(t, result.ResourceID)
			assert.Empty(t, store.puts)
			var count int64
			require.NoError(t, db.Model(&model.FunCloudHostedAsset{}).Count(&count).Error)
			assert.Zero(t, count)
		})
	}
}

func TestHostedAssetCreationFailuresAndIndependentCopies(t *testing.T) {
	db := withHostedAssetDB(t)
	store := newFakeHostedImageStore()
	ctx := withHostedImageSession(t.Context(), store)
	owner := newFunCloudHostedMaterialAdapter(7, "model")
	store.failPut = true
	failed, err := owner.CreateAsset(ctx, hostedImageCreateRequest(t, ""))
	require.Error(t, err)
	assert.Empty(t, failed.ResourceID)
	store.failPut = false
	first, err := owner.CreateAsset(ctx, hostedImageCreateRequest(t, ""))
	require.NoError(t, err)
	second, err := owner.CreateAsset(ctx, hostedImageCreateRequest(t, ""))
	require.NoError(t, err)
	assert.NotEqual(t, first.ResourceID, second.ResourceID)
	require.Len(t, store.puts, 2)
	require.NoError(t, db.Callback().Create().Before("gorm:create").Register("hosted_failure", func(tx *gorm.DB) {
		if tx.Statement.Table == "fun_cloud_hosted_assets" {
			tx.AddError(errors.New("write failed"))
		}
	}))
	t.Cleanup(func() { require.NoError(t, db.Callback().Create().Remove("hosted_failure")) })
	failed, err = owner.CreateAsset(ctx, hostedImageCreateRequest(t, ""))
	require.Error(t, err)
	assert.Empty(t, failed.ResourceID)
	assert.Len(t, store.puts, 3, "failed database insert never compensates by deleting the object")
	require.NoError(t, owner.DeleteAsset(ctx, first.ResourceID))
	require.NoError(t, owner.DeleteAsset(ctx, first.ResourceID))
	assert.Len(t, store.puts, 3)
}

func TestHostedObjectAvailabilityAndLocationFailClosed(t *testing.T) {
	withHostedAssetDB(t)
	store := newFakeHostedImageStore()
	ctx := withHostedImageSession(t.Context(), store)
	owner := newFunCloudHostedMaterialAdapter(7, "model")
	created, err := owner.CreateAsset(ctx, hostedImageCreateRequest(t, ""))
	require.NoError(t, err)
	record, err := model.GetFunCloudHostedAsset(7, created.ResourceID)
	require.NoError(t, err)
	fact := model.TaskHostedMediaFact{ObjectKey: record.ObjectKey, StorageLocation: record.StorageLocation}
	require.NoError(t, validateFunCloudHostedObject(ctx, record.StorageLocation, record.ObjectKey))
	store.location.Bucket = "other-bucket"
	_, err = owner.GetAsset(ctx, created.ResourceID)
	require.Error(t, err)
	_, err = SignFunCloudHostedAssetURL(ctx, fact)
	require.Error(t, err)
	store.location = record.StorageLocation
	store.failHead = true
	_, err = owner.GetAsset(ctx, created.ResourceID)
	require.Error(t, err)
	store.failHead = false
	delete(store.puts, record.ObjectKey)
	_, err = owner.GetAsset(ctx, created.ResourceID)
	require.Error(t, err)
}

func TestHostedAcceptedReferenceSurvivesDeletionAndRuntimeReplacement(t *testing.T) {
	withHostedAssetDB(t)
	store := newFakeHostedImageStore()
	ctx := withHostedImageSession(t.Context(), store)
	owner := newFunCloudHostedMaterialAdapter(7, "model")
	created, err := owner.CreateAsset(ctx, hostedImageCreateRequest(t, ""))
	require.NoError(t, err)
	c, info := hostedVideoContext(t, true, "asset://"+created.ResourceID, "asset://"+created.ResourceID)
	c.Request = c.Request.WithContext(ctx)
	require.Nil(t, ValidateFunCloudHostedVideoMedia(c, info))
	facts := GetFunCloudHostedMediaFacts(c)
	require.Len(t, facts, 1)
	require.NoError(t, owner.DeleteAsset(ctx, created.ResourceID))
	taskArtifactStoreRuntime.mu.Lock()
	previousStore, previousRevision, previousSession := taskArtifactStoreRuntime.store, taskArtifactStoreRuntime.revision, taskArtifactStoreRuntime.imageSession
	taskArtifactStoreRuntime.mu.Unlock()
	taskArtifactStoreRuntime.swap(&disabledArtifactStore{}, "disabled-during-accepted-request")
	t.Cleanup(func() {
		taskArtifactStoreRuntime.mu.Lock()
		defer taskArtifactStoreRuntime.mu.Unlock()
		taskArtifactStoreRuntime.store = previousStore
		taskArtifactStoreRuntime.revision = previousRevision
		taskArtifactStoreRuntime.imageSession = previousSession
	})
	signed, err := SignFunCloudHostedAssetURL(c.Request.Context(), facts["asset://"+created.ResourceID])
	require.NoError(t, err)
	assert.NotEmpty(t, signed)
	next, nextInfo := hostedVideoContext(t, true, "asset://"+created.ResourceID)
	require.NotNil(t, ValidateFunCloudHostedVideoMedia(next, nextInfo))
	task := &model.Task{}
	StageFunCloudHostedMediaSnapshot(c, task)
	StageFunCloudHostedMediaSnapshot(c, task)
	require.Len(t, task.PrivateData.HostedMedia, 1)
}
