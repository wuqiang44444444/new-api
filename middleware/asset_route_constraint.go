package middleware

import (
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/asset_setting"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

var platformAssetReferencePattern = regexp.MustCompile(`^asset://(ast_[0-9A-Za-z]{32})$`)

func AssetRouteConstraint() gin.HandlerFunc {
	return func(c *gin.Context) {
		if c.Request.Method != http.MethodPost ||
			(c.Request.URL.Path != "/v1/video/generations" && c.Request.URL.Path != "/v1/videos") {
			c.Next()
			return
		}
		var body map[string]any
		if err := common.UnmarshalBodyReusable(c, &body); err != nil {
			abortAssetRoute(c, http.StatusBadRequest, "invalid_request", "invalid video request")
			return
		}
		if content, exists := body["content"]; exists && content != nil {
			abortAssetRoute(c, http.StatusBadRequest, "invalid_request", "top-level content is not supported; use metadata.content")
			return
		}
		references, err := collectPlatformAssetReferences(body)
		if err != nil {
			abortAssetRoute(c, http.StatusBadRequest, "invalid_asset_reference", err.Error())
			return
		}
		if len(references) == 0 {
			c.Next()
			return
		}
		config := asset_setting.Current()
		if !config.Enabled {
			abortAssetRoute(c, http.StatusServiceUnavailable, "asset_library_disabled", "asset library is disabled")
			return
		}
		publicIDs := make([]string, 0, len(references))
		for id := range references {
			publicIDs = append(publicIDs, id)
		}
		assets, err := model.LoadAssetsForReference(c.GetInt("id"), publicIDs)
		if err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				abortAssetRoute(c, http.StatusNotFound, "asset_not_found", "one or more assets were not found")
			} else {
				abortAssetRoute(c, http.StatusInternalServerError, "database_error", "failed to load referenced assets")
			}
			return
		}
		assetIDs := make([]int64, 0, len(assets))
		requestModel, _ := body["model"].(string)
		requestModel = strings.TrimSpace(requestModel)
		for i := range assets {
			asset := &assets[i]
			if asset.Status != model.AssetStatusReady {
				abortAssetRoute(c, http.StatusConflict, "asset_not_ready", "one or more assets are not ready")
				return
			}
			if asset.AssetKind == model.AssetKindRealPerson {
				if !config.RealPersonEnabled {
					abortAssetRoute(c, http.StatusServiceUnavailable, "real_person_asset_disabled", "real-person asset service is disabled")
					return
				}
				if asset.AuthorizationID == nil {
					abortAssetRoute(c, http.StatusForbidden, "real_person_authorization_revoked", "real-person authorization is not active")
					return
				}
				active, err := activeRealPersonAuthorization(asset.UserID, *asset.AuthorizationID)
				if err != nil {
					abortAssetRoute(c, http.StatusInternalServerError, "database_error", "failed to validate real-person authorization")
					return
				}
				if !active {
					abortAssetRoute(c, http.StatusForbidden, "real_person_authorization_revoked", "real-person authorization is not active")
					return
				}
			}
			assetIDs = append(assetIDs, asset.ID)
		}
		bindings, err := model.ActiveBindingsForAssets(assetIDs)
		if err != nil {
			abortAssetRoute(c, http.StatusInternalServerError, "database_error", "failed to resolve asset bindings")
			return
		}
		byAsset := make(map[int64]map[int]struct{}, len(assetIDs))
		for _, binding := range bindings {
			if binding.RequestedModel != "" && binding.RequestedModel != requestModel {
				continue
			}
			current, err := bindingCredentialIsCurrent(&binding)
			if err != nil {
				abortAssetRoute(c, http.StatusInternalServerError, "database_error", "failed to validate asset binding credentials")
				return
			}
			if !current {
				continue
			}
			if byAsset[binding.AssetID] == nil {
				byAsset[binding.AssetID] = map[int]struct{}{}
			}
			byAsset[binding.AssetID][binding.ChannelID] = struct{}{}
		}
		var allowed map[int]struct{}
		for _, assetID := range assetIDs {
			channels := byAsset[assetID]
			if len(channels) == 0 {
				abortAssetRoute(c, http.StatusConflict, "asset_binding_required", "active upstream bindings are required for all referenced assets")
				return
			}
			if allowed == nil {
				allowed = cloneChannelSet(channels)
				continue
			}
			for id := range allowed {
				if _, ok := channels[id]; !ok {
					delete(allowed, id)
				}
			}
		}
		if len(allowed) == 0 {
			abortAssetRoute(c, http.StatusConflict, "asset_binding_required", "referenced assets do not share a compatible upstream channel")
			return
		}
		common.SetContextKey(c, constant.ContextKeyAssetAllowedChannelIDs, allowed)
		common.SetContextKey(c, constant.ContextKeyAssetReferenceIDs, publicIDs)
		c.Next()
	}
}

func collectPlatformAssetReferences(body map[string]any) (map[string]struct{}, error) {
	values := make([]string, 0)
	if image, ok := body["image"].(string); ok {
		values = append(values, image)
	}
	if inputReference, ok := body["input_reference"].(map[string]any); ok {
		if imageURL, ok := inputReference["image_url"].(string); ok {
			values = append(values, imageURL)
		}
	}
	if images, ok := body["images"].([]any); ok {
		for _, raw := range images {
			if value, ok := raw.(string); ok {
				values = append(values, value)
			}
		}
	}
	metadata, _ := body["metadata"].(map[string]any)
	if metadata == nil {
		if rawMetadata, ok := body["metadata"].(string); ok && strings.TrimSpace(rawMetadata) != "" {
			if err := common.UnmarshalJsonStr(rawMetadata, &metadata); err != nil {
				return nil, fmt.Errorf("invalid metadata")
			}
		}
	}
	if metadata != nil {
		if content, ok := metadata["content"].([]any); ok {
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
						values = append(values, value)
					}
				}
			}
		}
	}
	result := map[string]struct{}{}
	for _, value := range values {
		value = strings.TrimSpace(value)
		if len(value) < len("asset://") || !strings.EqualFold(value[:len("asset://")], "asset://") {
			continue
		}
		match := platformAssetReferencePattern.FindStringSubmatch(value)
		if len(match) != 2 {
			return nil, fmt.Errorf("invalid platform asset reference")
		}
		result[match[1]] = struct{}{}
	}
	return result, nil
}

func bindingCredentialIsCurrent(binding *model.AssetBinding) (bool, error) {
	profile := dto.AssetUpstreamProfile(binding.UpstreamProfile)
	if !profile.IsRoutable() {
		return false, nil
	}
	channel, err := model.GetChannelById(binding.ChannelID, true)
	if err != nil {
		return false, err
	}
	if channel.Status != common.ChannelStatusEnabled || channel.Type != constant.ChannelTypeDoubaoVideo || channel.ChannelInfo.IsMultiKey {
		return false, nil
	}
	settings := channel.GetOtherSettings()
	if settings.AssetUpstreamProfile != profile {
		return false, nil
	}
	if profile == dto.AssetUpstreamProfileArk && settings.VideoUpstreamProfile != dto.VideoUpstreamProfileThirdPartyReverseProxy ||
		profile == dto.AssetUpstreamProfileRelay && settings.VideoUpstreamProfile != dto.VideoUpstreamProfileThirdPartyRelay ||
		profile == dto.AssetUpstreamProfileOfficial && settings.VideoUpstreamProfile != "" && settings.VideoUpstreamProfile != dto.VideoUpstreamProfileOfficial {
		return false, nil
	}
	_, fingerprint, err := model.ResolveAssetChannelCredential(channel)
	if err != nil {
		return false, nil
	}
	return fingerprint == binding.CredentialFingerprint, nil
}

func activeRealPersonAuthorization(userID int, authorizationID int64) (bool, error) {
	var count int64
	err := model.DB.Model(&model.RealPersonAuthorization{}).Where("id = ? AND user_id = ? AND status = ?", authorizationID, userID, model.RealPersonAuthorizationAuthorized).Count(&count).Error
	return count == 1, err
}

func cloneChannelSet(source map[int]struct{}) map[int]struct{} {
	result := make(map[int]struct{}, len(source))
	for id := range source {
		result[id] = struct{}{}
	}
	return result
}

func assetChannelAllowed(c *gin.Context, channelID int) bool {
	value, ok := common.GetContextKey(c, constant.ContextKeyAssetAllowedChannelIDs)
	if !ok {
		return true
	}
	allowed, ok := value.(map[int]struct{})
	if !ok {
		return false
	}
	_, ok = allowed[channelID]
	return ok
}

func abortAssetRoute(c *gin.Context, status int, code, message string) {
	c.AbortWithStatusJSON(status, gin.H{"error": gin.H{
		"message":    message,
		"type":       "asset_error",
		"code":       code,
		"request_id": c.GetString(common.RequestIdKey),
	}})
}

func IsolateConsentRoutes() gin.HandlerFunc {
	return func(c *gin.Context) {
		removeConsentCORSHeaders(c)
		if c.Request.Method == http.MethodOptions {
			c.AbortWithStatus(http.StatusMethodNotAllowed)
			return
		}
		c.Next()
		removeConsentCORSHeaders(c)
	}
}

func removeConsentCORSHeaders(c *gin.Context) {
	for _, header := range []string{"Access-Control-Allow-Origin", "Access-Control-Allow-Credentials", "Access-Control-Allow-Methods", "Access-Control-Allow-Headers"} {
		c.Writer.Header().Del(header)
	}
}
