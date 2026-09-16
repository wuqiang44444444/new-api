package service

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/clienterrlog"
	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	assetadapter "github.com/QuantumNous/new-api/relay/channel/task/seedance/assets"
	"github.com/QuantumNous/new-api/relaykit/dto"

	_ "golang.org/x/image/bmp"
	_ "golang.org/x/image/tiff"
	_ "golang.org/x/image/webp"
)

// FunCloud 托管素材 adapter（funcloud_material_hosted）。它不访问 FunCloud
// 上游：素材组与素材是平台持久化事实，源图片在创建调用内复制到本站私有
// 对象存储。素材按 user_id 隔离与共享；删除只停止新引用，不删除对象。

// funCloudHostedMediaURLTTL 是视频履约时为本站对象签发的内部访问期限。
// 必须覆盖正常排队与 Provider 取图窗口；不提供自动续签，取图失败按业务
// 失败返回，不自动重发。
const funCloudHostedMediaURLTTL = 24 * time.Hour

type funCloudHostedMaterialAdapter struct {
	userID    int
	modelName string
}

func newFunCloudHostedMaterialAdapter(userID int, modelName string) *funCloudHostedMaterialAdapter {
	return &funCloudHostedMaterialAdapter{userID: userID, modelName: strings.TrimSpace(modelName)}
}

func (*funCloudHostedMaterialAdapter) Profile() dto.AssetUpstreamProfile {
	return dto.AssetUpstreamProfileFunCloudHosted
}

// GeneralAssetGroupPolicy 固定表达宿主托管语义；工厂入口已核对声明中的
// groupPolicy 与本能力一致，运行时不再从旧协议表推导。
func (*funCloudHostedMaterialAdapter) GeneralAssetGroupPolicy() dto.GeneralAssetGroupPolicy {
	return dto.GeneralAssetGroupPolicyHosted
}

// Supports 首期只登记普通图片素材；视频与音频引用仍由调用方直接提供 URL。
func (*funCloudHostedMaterialAdapter) Supports(kind, mediaType string) bool {
	return kind == model.AssetKindGeneral && mediaType == "image"
}

func (a *funCloudHostedMaterialAdapter) CheckConnectivity(ctx context.Context) error {
	if err := CheckImageObjectStoreReady(ctx); err != nil {
		return fmt.Errorf("hosted asset object storage is unavailable: %w", err)
	}
	return nil
}

func (a *funCloudHostedMaterialAdapter) CreateGroup(ctx context.Context, req assetadapter.GroupRequest) (assetadapter.GroupResult, error) {
	groupID, err := model.NewFunCloudHostedAssetGroupID()
	if err != nil {
		return assetadapter.GroupResult{}, err
	}
	record := &model.FunCloudHostedAssetGroup{
		ID:          groupID,
		UserID:      a.userID,
		Model:       a.modelName,
		Name:        strings.TrimSpace(req.Name),
		Description: strings.TrimSpace(req.Description),
	}
	if err := model.CreateFunCloudHostedAssetGroup(record); err != nil {
		return assetadapter.GroupResult{}, err
	}
	return assetadapter.GroupResult{ResourceID: record.ID, BusinessID: record.ID, Name: record.Name, Status: "active"}, nil
}

func (a *funCloudHostedMaterialAdapter) GetGroup(_ context.Context, resourceID string) (assetadapter.GroupResult, error) {
	record, err := model.GetFunCloudHostedAssetGroup(a.userID, resourceID)
	if err != nil {
		return assetadapter.GroupResult{}, err
	}
	if record == nil {
		return assetadapter.GroupResult{}, assetadapter.ErrAssetResourceNotFound
	}
	return assetadapter.GroupResult{ResourceID: record.ID, BusinessID: record.ID, Name: record.Name, Status: "active"}, nil
}

// CreateAsset 校验显式素材组归属（跨用户组一律按不存在处理），把源图片
// 字节复制到本站对象存储，成功后持久化素材记录。复制成功但记录写入失败
// 时不返回可用素材，也不做删除补偿。
func (a *funCloudHostedMaterialAdapter) CreateAsset(ctx context.Context, req assetadapter.AssetRequest) (assetadapter.AssetResult, error) {
	groupID := strings.TrimSpace(req.GroupResourceID)
	if groupID != "" {
		if !strings.HasPrefix(groupID, model.FunCloudHostedGroupIDPrefix) {
			return assetadapter.AssetResult{}, assetadapter.ErrAssetResourceNotFound
		}
		group, err := model.GetFunCloudHostedAssetGroup(a.userID, groupID)
		if err != nil {
			return assetadapter.AssetResult{}, err
		}
		if group == nil {
			return assetadapter.AssetResult{}, assetadapter.ErrAssetResourceNotFound
		}
	}
	if req.Source == nil {
		return assetadapter.AssetResult{}, errors.New("hosted asset source is required")
	}
	data, err := readHostedAssetSource(req.Source, req.SourceMaxBytes)
	if err != nil {
		reason := "source_read_failed"
		if errors.Is(err, errHostedSourceTooLarge) {
			reason = "source_too_large"
		}
		clienterrlog.Attach(ctx, clienterrlog.Report{Stage: "content_validation", Reason: reason})
		return assetadapter.AssetResult{}, err
	}
	contentType := strings.TrimSpace(req.SourceType)
	cfg, format, err := image.DecodeConfig(bytes.NewReader(data))
	expectedType := map[string]string{"jpeg": "image/jpeg", "png": "image/png", "gif": "image/gif", "webp": "image/webp", "bmp": "image/bmp", "tiff": "image/tiff"}[format]
	if err != nil || expectedType == "" {
		clienterrlog.Attach(ctx, clienterrlog.Report{Stage: "content_validation", Reason: "invalid_image"})
		return assetadapter.AssetResult{}, fmt.Errorf("%w: invalid hosted image content or dimensions", ErrInvalidAssetRequest)
	}
	if contentType != expectedType {
		clienterrlog.Attach(ctx, clienterrlog.Report{
			Stage: "content_validation", Reason: "content_type_mismatch",
			Detail: map[string]string{"source_content_type": contentType},
		})
		return assetadapter.AssetResult{}, fmt.Errorf("%w: invalid hosted image content or dimensions", ErrInvalidAssetRequest)
	}
	if cfg.Width <= 0 || cfg.Height <= 0 || int64(cfg.Width) > dto.PublicAssetHostedMaxPixels/int64(cfg.Height) {
		clienterrlog.Attach(ctx, clienterrlog.Report{
			Stage: "content_validation", Reason: "image_dimensions_exceeded",
			Detail: map[string]string{"decoded_width": strconv.Itoa(cfg.Width), "decoded_height": strconv.Itoa(cfg.Height)},
		})
		return assetadapter.AssetResult{}, fmt.Errorf("%w: invalid hosted image content or dimensions", ErrInvalidAssetRequest)
	}
	if _, _, err := image.Decode(bytes.NewReader(data)); err != nil {
		clienterrlog.Attach(ctx, clienterrlog.Report{Stage: "content_validation", Reason: "invalid_image"})
		return assetadapter.AssetResult{}, fmt.Errorf("%w: invalid hosted image content", ErrInvalidAssetRequest)
	}
	storeCtx, err := WithImageObjectStore(ctx)
	if err != nil {
		return assetadapter.AssetResult{}, err
	}
	location, err := funCloudHostedStorageLocation(storeCtx)
	if err != nil {
		return assetadapter.AssetResult{}, err
	}
	objectKey, err := buildFunCloudHostedAssetObjectKey(contentType)
	if err != nil {
		return assetadapter.AssetResult{}, err
	}
	if _, err := PutImageObject(storeCtx, objectKey, contentType, data); err != nil {
		return assetadapter.AssetResult{}, err
	}
	assetID, err := model.NewFunCloudHostedAssetID()
	if err != nil {
		return assetadapter.AssetResult{}, err
	}
	record := &model.FunCloudHostedAsset{
		ID:              assetID,
		UserID:          a.userID,
		GroupID:         groupID,
		Model:           a.modelName,
		Name:            strings.TrimSpace(req.Name),
		ObjectKey:       objectKey,
		StorageLocation: location,
		MimeType:        contentType,
		SizeBytes:       int64(len(data)),
		Status:          model.FunCloudHostedAssetStatusReady,
	}
	if err := model.CreateFunCloudHostedAsset(record); err != nil {
		// 不返回可用素材；留下的无引用对象不做删除补偿。
		return assetadapter.AssetResult{}, err
	}
	return hostedAssetResult(record), nil
}

func (a *funCloudHostedMaterialAdapter) GetAsset(ctx context.Context, resourceID string) (assetadapter.AssetResult, error) {
	record, err := model.GetFunCloudHostedAsset(a.userID, resourceID)
	if err != nil {
		return assetadapter.AssetResult{}, err
	}
	if record == nil {
		return assetadapter.AssetResult{}, assetadapter.ErrAssetResourceNotFound
	}
	if err := validateFunCloudHostedObject(ctx, record.StorageLocation, record.ObjectKey); err != nil {
		return assetadapter.AssetResult{}, err
	}
	return hostedAssetResult(record), nil
}

func (*funCloudHostedMaterialAdapter) UpdateAsset(_ context.Context, _, _ string) (assetadapter.AssetResult, error) {
	return assetadapter.AssetResult{}, assetadapter.ErrAssetOperationUnsupported
}

// DeleteAsset 只停止该 asset ID 的新引用；已提交的视频继续，不删除 OSS 文件。
func (a *funCloudHostedMaterialAdapter) DeleteAsset(_ context.Context, resourceID string) error {
	deleted, err := model.MarkFunCloudHostedAssetDeleted(a.userID, resourceID)
	if err != nil {
		return err
	}
	if !deleted {
		return assetadapter.ErrAssetResourceNotFound
	}
	return nil
}

func hostedAssetResult(record *model.FunCloudHostedAsset) assetadapter.AssetResult {
	return assetadapter.AssetResult{
		ResourceID:     record.ID,
		BusinessID:     record.ID,
		ReferenceType:  "asset_uri_id",
		ReferenceValue: record.ID,
		Status:         "ready",
	}
}

var (
	errHostedSourceUnreadable = errors.New("hosted asset source could not be read")
	errHostedSourceTooLarge   = errors.New("hosted asset source exceeds the upload limit")
)

// readHostedAssetSource 读取全部源字节并执行已登记的大小上限。
func readHostedAssetSource(source io.Reader, maxBytes int64) ([]byte, error) {
	if maxBytes <= 0 {
		maxBytes = funCloudMaterialMaxBytes
	}
	data, err := io.ReadAll(io.LimitReader(source, maxBytes+1))
	if err != nil {
		return nil, errHostedSourceUnreadable
	}
	if int64(len(data)) > maxBytes {
		return nil, errHostedSourceTooLarge
	}
	return data, nil
}

// buildFunCloudHostedAssetObjectKey 生成平台自定的目标对象名；不使用客户
// URL 的文件名，也不以来源文件名参与身份。
func buildFunCloudHostedAssetObjectKey(contentType string) (string, error) {
	extension := map[string]string{
		"image/jpeg": ".jpg", "image/png": ".png", "image/webp": ".webp",
		"image/bmp": ".bmp", "image/tiff": ".tiff", "image/gif": ".gif",
	}[strings.ToLower(contentType)]
	random, err := common.GenerateRandomCharsKey(24)
	if err != nil {
		return "", err
	}
	now := common.GetTimestamp()
	return fmt.Sprintf("assets/funcloud-hosted/%d/%d-%s%s", now/86400, now, random, extension), nil
}

// SignFunCloudHostedAssetURL 为一个已保存对象签发内部临时访问 URL。
// 签名 URL 只用于当次 Provider 调用，不进入日志、数据库或公开响应。
func SignFunCloudHostedAssetURL(ctx context.Context, fact model.TaskHostedMediaFact) (string, error) {
	location, err := funCloudHostedStorageLocation(ctx)
	if err != nil || location != fact.StorageLocation {
		return "", ErrTaskArtifactStoreDisabled
	}
	session, err := imageObjectSessionForContext(ctx)
	if err != nil {
		return "", err
	}
	presigner, ok := session.store.(interface {
		presignHostedAssetURL(objectKey string, ttl time.Duration) (string, error)
	})
	if !ok {
		return "", ErrTaskArtifactStoreDisabled
	}
	return presigner.presignHostedAssetURL(fact.ObjectKey, funCloudHostedMediaURLTTL)
}

// presignHostedAssetURL 由 S3 与 Azure Blob 存储方法各自履约；TTL 由调用方
// 指定，与图片结果固定 300 秒的既有语义互不影响。
func (s *s3ArtifactStore) presignHostedAssetURL(objectKey string, ttl time.Duration) (string, error) {
	return s.PresignObjectURL(objectKey, ttl)
}

func (s *azureBlobArtifactStore) presignHostedAssetURL(objectKey string, ttl time.Duration) (string, error) {
	return s.presignObjectURL(objectKey, ttl)
}
