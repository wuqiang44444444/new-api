package relay

import (
	"errors"
	"fmt"
	"slices"
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
	appID := common.GetContextKeyInt(c, constant.ContextKeyAssetAppID)
	subjectHash := common.GetContextKeyString(c, constant.ContextKeyEndUserSubjectHash)
	channelID := common.GetContextKeyInt(c, constant.ContextKeyChannelId)
	channel, err := model.GetChannelById(channelID, true)
	if err != nil {
		return err
	}
	implementation, ok := model.ResolveChannelLinkImplementation(channel)
	if info == nil || info.PublishedLinkContractSKU == "" || !ok ||
		model.ValidateChannelLinkExecution(channel, info.OriginModelName, model.LinkRouteFamily(info.LinkRouteFamily), info.PublishedLinkContractSKU) != nil {
		return fmt.Errorf("%w: selected channel has no registered Link implementation", service.ErrAssetReferenceUnresolvable)
	}
	var fingerprint string
	if implementation.AssetCapability.Supports(model.LinkAssetResolutionUpstreamBinding) {
		_, fingerprint, err = model.ResolveAssetChannelCredential(channel)
		if err != nil {
			return fmt.Errorf("%w: selected asset credential is unavailable", service.ErrAssetCredentialChanged)
		}
	}

	value, _ := common.GetContextKey(c, constant.ContextKeyAssetReferenceIDs)
	publicIDs, _ := value.([]string)
	replacements := map[string]string{}
	assetPublicIDs := make([]string, 0, len(publicIDs))
	bindingIDs := make([]int64, 0, len(publicIDs))
	assets, err := model.LoadAssetsForReferenceForApp(userID, appID, publicIDs)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return fmt.Errorf("%w: referenced asset no longer exists", service.ErrAssetNotFound)
		}
		return err
	}
	assetIDs := make([]int64, 0, len(assets))
	for i := range assets {
		assetIDs = append(assetIDs, assets[i].ID)
	}
	sources, err := model.LoadAssetSources(assetIDs)
	if err != nil {
		return err
	}
	imageCount, videoCount, audioCount := 0, 0, 0
	for i := range assets {
		asset := &assets[i]
		if asset.RequestedModel != info.OriginModelName ||
			asset.LinkContractNamespace != info.LinkContractNamespace ||
			asset.LinkRouteFamily != info.LinkRouteFamily ||
			asset.PublishedLinkContractSKU != info.PublishedLinkContractSKU ||
			asset.LinkPublicationVersion != info.LinkPublicationVersion {
			return fmt.Errorf("%w: asset %s belongs to a different Link publication", service.ErrAssetBindingRequired, asset.PublicID)
		}
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
			if subjectHash == "" || asset.AppID != appID || asset.EndUserSubjectHash == "" || asset.EndUserSubjectHash != subjectHash {
				return fmt.Errorf("%w: end-user subject mismatch for %s", service.ErrRealPersonAuthorizationNotReady, asset.PublicID)
			}
			var activeCount int64
			if err := model.DB.Model(&model.RealPersonAuthorization{}).Where("id = ? AND user_id = ? AND app_id = ? AND end_user_subject_hash = ? AND status = ?", *asset.AuthorizationID, userID, appID, subjectHash, model.RealPersonAuthorizationAuthorized).Count(&activeCount).Error; err != nil {
				return err
			}
			if activeCount != 1 {
				return fmt.Errorf("%w: real-person authorization is unavailable for %s", service.ErrRealPersonAuthorizationNotReady, asset.PublicID)
			}
		}
		bindingResolved := false
		if implementation.AssetCapability.Supports(model.LinkAssetResolutionUpstreamBinding) {
			var binding model.AssetBinding
			err := model.DB.Where("asset_id = ? AND user_id = ? AND channel_id = ? AND credential_fingerprint = ? AND link_implementation_id = ? AND link_implementation_version = ? AND link_implementation_hash = ? AND status = ? AND upstream_reference_type = ?", asset.ID, userID, channelID, fingerprint, implementation.ID, implementation.Version, implementation.ContentHash, model.AssetBindingStatusActive, "asset_uri_id").First(&binding).Error
			if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
				return err
			}
			if err == nil && strings.TrimSpace(binding.UpstreamReferenceValue) != "" {
				if info == nil || info.ChannelMeta == nil || string(info.ChannelMeta.ChannelOtherSettings.AssetUpstreamProfile) != binding.UpstreamProfile {
					return fmt.Errorf("%w: asset binding profile changed before request submission", service.ErrAssetCredentialChanged)
				}
				if binding.RequestedModel != "" && binding.RequestedModel != req.Model {
					return fmt.Errorf("%w: asset binding model does not match the video request", service.ErrAssetBindingRequired)
				}
				if binding.LinkContractNamespace != info.LinkContractNamespace ||
					binding.LinkRouteFamily != info.LinkRouteFamily ||
					binding.PublishedLinkContractSKU != info.PublishedLinkContractSKU ||
					binding.LinkPublicationVersion != info.LinkPublicationVersion {
					return fmt.Errorf("%w: asset binding publication does not match the video request", service.ErrAssetBindingRequired)
				}
				replacements["asset://"+asset.PublicID] = "asset://" + binding.UpstreamReferenceValue
				bindingIDs = append(bindingIDs, binding.ID)
				bindingResolved = true
			}
		}
		if !bindingResolved {
			if !implementation.AssetCapability.Supports(model.LinkAssetResolutionSourceURL) ||
				asset.AssetKind == model.AssetKindRealPerson ||
				!slices.Contains(implementation.AssetCapability.AssetKinds, asset.AssetKind) ||
				!slices.Contains(implementation.AssetCapability.MediaTypes, asset.MediaType) {
				return fmt.Errorf("%w: no compatible Link asset resolution is available for %s", service.ErrAssetReferenceUnresolvable, asset.PublicID)
			}
			source, exists := sources[asset.ID]
			if !exists || (source.ExpiresAt != 0 && source.ExpiresAt-common.GetTimestamp() < implementation.AssetCapability.SourceMinTTLSeconds) {
				return fmt.Errorf("%w: source URL is unavailable or expires too soon for %s", service.ErrAssetSourceExpired, asset.PublicID)
			}
			sourceURL, err := model.DecryptAssetSourceURL(asset, &source)
			if err != nil {
				return fmt.Errorf("%w: source URL is unavailable for %s", service.ErrAssetReferenceUnresolvable, asset.PublicID)
			}
			replacements["asset://"+asset.PublicID] = sourceURL
		}
		switch asset.MediaType {
		case "image":
			imageCount++
		case "video":
			videoCount++
		case "audio":
			audioCount++
		}
		assetPublicIDs = append(assetPublicIDs, asset.PublicID)
	}
	// Public capability validation protects the customer contract; these
	// implementation-specific source limits are rechecked immediately before
	// dispatch because a different eligible implementation may have been chosen.
	if implementation.AssetCapability.Supports(model.LinkAssetResolutionSourceURL) &&
		((implementation.AssetCapability.MaxImages > 0 && imageCount > implementation.AssetCapability.MaxImages) ||
			(implementation.AssetCapability.MaxVideos > 0 && videoCount > implementation.AssetCapability.MaxVideos) ||
			(implementation.AssetCapability.MaxAudio > 0 && audioCount > implementation.AssetCapability.MaxAudio)) {
		return fmt.Errorf("%w: referenced assets exceed the Link implementation media limits", service.ErrAssetReferenceUnresolvable)
	}
	if info != nil && info.TaskRelayInfo != nil {
		info.AssetPublicIDs = assetPublicIDs
		info.AssetBindingIDs = bindingIDs
		info.AppID = appID
		info.EndUserSubjectHash = subjectHash
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
	if err := relaycommon.RewriteVideoContractAssetReferences(c, replacements); err != nil {
		return err
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
