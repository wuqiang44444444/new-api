package relay

import (
	"errors"
	"fmt"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/asset_setting"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

func resolveAssetReferencesForAttempt(c *gin.Context, info *relaycommon.RelayInfo) error {
	if _, ok := common.GetContextKey(c, constant.ContextKeyAssetReferenceIDs); !ok {
		return nil
	}
	config := asset_setting.Current()
	if !config.Enabled {
		return fmt.Errorf("%w: asset library is disabled", service.ErrAssetLibraryUnavailable)
	}
	req, err := relaycommon.GetTaskRequest(c)
	if err != nil {
		return err
	}
	userID := c.GetInt("id")
	channelID := common.GetContextKeyInt(c, constant.ContextKeyChannelId)
	channel, err := model.GetChannelById(channelID, true)
	if err != nil {
		return err
	}
	_, fingerprint, err := model.ResolveAssetChannelCredential(channel)
	if err != nil {
		return fmt.Errorf("%w: selected asset credential is unavailable", service.ErrAssetCredentialChanged)
	}

	value, _ := common.GetContextKey(c, constant.ContextKeyAssetReferenceIDs)
	publicIDs, _ := value.([]string)
	replacements := map[string]string{}
	assetPublicIDs := make([]string, 0, len(publicIDs))
	bindingIDs := make([]int64, 0, len(publicIDs))
	assets, err := model.LoadAssetsForReference(userID, publicIDs)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return fmt.Errorf("%w: referenced asset no longer exists", service.ErrAssetNotFound)
		}
		return err
	}
	for i := range assets {
		asset := &assets[i]
		if asset.Status != model.AssetStatusReady {
			return fmt.Errorf("%w: asset %s is no longer ready", service.ErrAssetNotReady, asset.PublicID)
		}
		if asset.AssetKind == model.AssetKindRealPerson {
			if !config.RealPersonEnabled {
				return fmt.Errorf("%w: real-person asset service is disabled", service.ErrAssetLibraryUnavailable)
			}
			if asset.AuthorizationID == nil {
				return fmt.Errorf("%w: real-person authorization is unavailable for %s", service.ErrRealPersonAuthorizationNotReady, asset.PublicID)
			}
			var activeCount int64
			if err := model.DB.Model(&model.RealPersonAuthorization{}).Where("id = ? AND user_id = ? AND status = ?", *asset.AuthorizationID, userID, model.RealPersonAuthorizationAuthorized).Count(&activeCount).Error; err != nil {
				return err
			}
			if activeCount != 1 {
				return fmt.Errorf("%w: real-person authorization is unavailable for %s", service.ErrRealPersonAuthorizationNotReady, asset.PublicID)
			}
		}
		var binding model.AssetBinding
		err := model.DB.Where("asset_id = ? AND user_id = ? AND channel_id = ? AND credential_fingerprint = ? AND status = ? AND upstream_reference_type = ?", asset.ID, userID, channelID, fingerprint, model.AssetBindingStatusActive, "asset_uri_id").First(&binding).Error
		if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		if errors.Is(err, gorm.ErrRecordNotFound) || strings.TrimSpace(binding.UpstreamReferenceValue) == "" {
			return fmt.Errorf("%w: active asset binding is unavailable for %s on the selected channel", service.ErrAssetBindingRequired, asset.PublicID)
		}
		if info == nil || info.ChannelMeta == nil || string(info.ChannelMeta.ChannelOtherSettings.AssetUpstreamProfile) != binding.UpstreamProfile {
			return fmt.Errorf("%w: asset binding profile changed before request submission", service.ErrAssetCredentialChanged)
		}
		if binding.RequestedModel != "" && binding.RequestedModel != req.Model {
			return fmt.Errorf("%w: asset binding model does not match the video request", service.ErrAssetBindingRequired)
		}
		replacements["asset://"+asset.PublicID] = "asset://" + binding.UpstreamReferenceValue
		assetPublicIDs = append(assetPublicIDs, asset.PublicID)
		bindingIDs = append(bindingIDs, binding.ID)
	}
	if info != nil && info.TaskRelayInfo != nil {
		info.AssetPublicIDs = assetPublicIDs
		info.AssetBindingIDs = bindingIDs
	}

	req.Image = replaceAssetReference(req.Image, replacements)
	req.InputReference = replaceAssetReference(req.InputReference, replacements)
	req.InputReferenceImageURL = replaceAssetReference(req.InputReferenceImageURL, replacements)
	for i := range req.Images {
		req.Images[i] = replaceAssetReference(req.Images[i], replacements)
	}
	if req.Metadata != nil {
		clonedBytes, err := common.Marshal(req.Metadata)
		if err != nil {
			return err
		}
		var cloned map[string]any
		if err := common.Unmarshal(clonedBytes, &cloned); err != nil {
			return err
		}
		rewriteMetadataAssetReferences(cloned, replacements)
		req.Metadata = cloned
	}
	c.Set("task_request", req)
	return nil
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
			if url, ok := media["url"].(string); ok {
				media["url"] = replaceAssetReference(url, replacements)
			}
		}
	}
}

func replaceAssetReference(value string, replacements map[string]string) string {
	if replacement, ok := replacements[strings.TrimSpace(value)]; ok {
		return replacement
	}
	return value
}
