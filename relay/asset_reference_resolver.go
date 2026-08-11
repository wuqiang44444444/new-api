package relay

import (
	"errors"
	"fmt"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/asset_setting"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

func resolveAssetReferencesForAttempt(c *gin.Context, info *relaycommon.RelayInfo) error {
	contract, ok := relaycommon.GetVideoContractRequest(c)
	if !ok || contract.ModelArk == nil {
		return nil
	}
	privateIDs, replacements, err := collectModelArkAssetReferences(contract.ModelArk.Content, info)
	if err != nil {
		return err
	}
	if len(privateIDs) == 0 && len(replacements) == 0 {
		return nil
	}
	assetPublicIDs := make([]string, 0, len(privateIDs))
	if len(privateIDs) > 0 {
		privateReplacements, resolvedPublicIDs, appID, err := resolvePrivateAssetReferences(c, info, privateIDs)
		if err != nil {
			return err
		}
		for source, target := range privateReplacements {
			replacements[source] = target
		}
		assetPublicIDs = resolvedPublicIDs
		if info.TaskRelayInfo != nil {
			info.AssetPublicIDs = assetPublicIDs
			info.AppID = appID
		}
	}
	if err := relaycommon.RewriteVideoContractAssetReferences(c, replacements); err != nil {
		return err
	}
	req, err := relaycommon.GetTaskRequest(c)
	if err != nil {
		return err
	}
	if req.Metadata != nil {
		encoded, err := common.Marshal(req.Metadata)
		if err != nil {
			return err
		}
		var cloned map[string]any
		if err := common.Unmarshal(encoded, &cloned); err != nil {
			return err
		}
		rewriteMetadataAssetReferences(cloned, replacements)
		req.Metadata = cloned
	}
	c.Set("task_request", req)
	return nil
}

func collectModelArkAssetReferences(content []dto.ModelArkVideoContent, info *relaycommon.RelayInfo) ([]string, map[string]string, error) {
	privateIDs := make([]string, 0)
	replacements := make(map[string]string)
	seen := make(map[string]struct{})
	for _, item := range content {
		for _, media := range []*dto.VideoMediaURL{item.ImageURL, item.VideoURL, item.AudioURL} {
			if media == nil {
				continue
			}
			value := strings.TrimSpace(media.URL)
			if !strings.HasPrefix(value, "asset://") {
				continue
			}
			switch {
			case strings.HasPrefix(value, "asset://ast_"):
				publicID := strings.TrimPrefix(value, "asset://")
				if _, exists := seen[publicID]; !exists {
					seen[publicID] = struct{}{}
					privateIDs = append(privateIDs, publicID)
				}
			case strings.HasPrefix(value, "asset://pubref_"):
				if info == nil || info.ChannelMeta == nil ||
					(info.ChannelOtherSettings.VideoUpstreamProtocol != dto.VideoUpstreamProtocolModelArkV3Volcengine &&
						info.ChannelOtherSettings.VideoUpstreamProtocol != dto.VideoUpstreamProtocolModelArkV3BytePlus) {
					return nil, nil, service.ErrAssetReferenceUnresolvable
				}
				providerID := strings.TrimPrefix(value, "asset://pubref_")
				if !validPublicProviderAssetID(providerID) {
					return nil, nil, service.ErrInvalidAssetRequest
				}
				replacements[value] = "asset://" + providerID
			default:
				return nil, nil, service.ErrInvalidAssetRequest
			}
		}
	}
	return privateIDs, replacements, nil
}

func validPublicProviderAssetID(value string) bool {
	if len(value) == 0 || len(value) > 256 {
		return false
	}
	for _, character := range value {
		if character >= 'a' && character <= 'z' || character >= 'A' && character <= 'Z' ||
			character >= '0' && character <= '9' || character == '-' || character == '_' || character == '.' {
			continue
		}
		return false
	}
	return true
}

func resolvePrivateAssetReferences(c *gin.Context, info *relaycommon.RelayInfo, publicIDs []string) (map[string]string, []string, int, error) {
	if info != nil && info.ChannelMeta != nil &&
		info.ChannelOtherSettings.AssetUpstreamProtocol == dto.AssetUpstreamProtocolNone {
		return nil, nil, 0, service.ErrAssetReferenceUnresolvable
	}
	if !asset_setting.Current().Enabled || info == nil || info.ChannelMeta == nil ||
		info.ChannelType != constant.ChannelTypeSeedanceLink {
		return nil, nil, 0, service.ErrAssetLibraryUnavailable
	}
	userID := c.GetInt("id")
	appID := common.GetContextKeyInt(c, constant.ContextKeyAssetAppID)
	assets, err := model.LoadAssetsForReferenceForApp(userID, appID, publicIDs)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil, 0, service.ErrAssetNotFound
		}
		return nil, nil, 0, err
	}
	channel, err := model.GetChannelById(info.ChannelId, true)
	if err != nil || channel == nil {
		return nil, nil, 0, service.ErrAssetReferenceUnresolvable
	}
	_, fingerprint, err := model.ResolveAssetChannelCredential(channel)
	if err != nil {
		return nil, nil, 0, service.ErrAssetScopeConflict
	}
	protocol := string(info.ChannelOtherSettings.AssetUpstreamProtocol)
	replacements := make(map[string]string, len(assets))
	assetPublicIDs := make([]string, 0, len(assets))
	for i := range assets {
		asset := &assets[i]
		if asset.Status != model.AssetStatusReady {
			return nil, nil, 0, fmt.Errorf("%w: %s", service.ErrAssetNotReady, asset.PublicID)
		}
		if asset.RequestedModel != info.OriginModelName || asset.ChannelID != info.ChannelId {
			return nil, nil, 0, fmt.Errorf("%w: %s", service.ErrAssetChannelMismatch, asset.PublicID)
		}
		if asset.CredentialFingerprint != fingerprint || asset.UpstreamProtocol != protocol {
			return nil, nil, 0, fmt.Errorf("%w: %s", service.ErrAssetScopeConflict, asset.PublicID)
		}
		if asset.UpstreamReferenceType != "asset_uri_id" || strings.TrimSpace(asset.UpstreamReferenceValue) == "" {
			return nil, nil, 0, fmt.Errorf("%w: %s", service.ErrAssetReferenceUnresolvable, asset.PublicID)
		}
		if asset.AssetKind == model.AssetKindRealPerson {
			if asset.AssetGroupID == nil {
				return nil, nil, 0, fmt.Errorf("%w: %s", service.ErrAssetNotReady, asset.PublicID)
			}
			var group model.AssetGroup
			if err := model.DB.First(&group, "id = ? AND user_id = ? AND app_id = ? AND status = ? AND deleted_at = ?",
				*asset.AssetGroupID, userID, appID, model.AssetStatusReady, 0).Error; err != nil {
				return nil, nil, 0, fmt.Errorf("%w: %s", service.ErrAssetNotReady, asset.PublicID)
			}
			if group.RequestedModel != info.OriginModelName || group.ChannelID != info.ChannelId {
				return nil, nil, 0, fmt.Errorf("%w: %s", service.ErrAssetChannelMismatch, asset.PublicID)
			}
			if group.CredentialFingerprint != fingerprint || group.UpstreamProtocol != protocol {
				return nil, nil, 0, fmt.Errorf("%w: %s", service.ErrAssetScopeConflict, asset.PublicID)
			}
		}
		replacements["asset://"+asset.PublicID] = "asset://" + asset.UpstreamReferenceValue
		assetPublicIDs = append(assetPublicIDs, asset.PublicID)
	}
	return replacements, assetPublicIDs, appID, nil
}

func rewriteMetadataAssetReferences(metadata map[string]any, replacements map[string]string) {
	content, ok := metadata["content"].([]any)
	if !ok {
		return
	}
	for _, rawItem := range content {
		item, ok := rawItem.(map[string]any)
		if !ok {
			continue
		}
		for _, field := range []string{"image_url", "video_url", "audio_url"} {
			media, ok := item[field].(map[string]any)
			if !ok {
				continue
			}
			if value, ok := media["url"].(string); ok {
				if replacement, found := replacements[strings.TrimSpace(value)]; found {
					media["url"] = replacement
				}
			}
		}
	}
}
