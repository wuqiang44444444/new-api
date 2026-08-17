package controller

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
)

func validateNewChannelAssetCredential(channel *model.Channel, input *dto.ChannelAssetCredentialInput, mode string) error {
	if !channelUsesOfficialAssetCredential(channel) {
		if input != nil {
			return errors.New("asset credential is only accepted for official_action_assets")
		}
		return nil
	}
	if mode != "single" || channel.ChannelInfo.IsMultiKey {
		return errors.New("official_action_assets only supports single-key channel creation")
	}
	if strings.TrimSpace(channel.Key) == "" {
		return errors.New("official video API key is required")
	}
	credential, err := model.NormalizeChannelAssetCredential(input)
	if err != nil {
		return err
	}
	if credential == nil {
		return errors.New("official_action_assets requires an asset Access Key ID and Secret Access Key")
	}
	return nil
}

func validateUpdatedChannelAssetCredential(channel, origin *model.Channel, requestData map[string]any, input *dto.ChannelAssetCredentialInput) error {
	effective := *origin
	if _, present := requestData["type"]; present && channel.Type != 0 {
		effective.Type = channel.Type
	}
	if _, present := requestData["settings"]; present {
		effective.OtherSettings = channel.OtherSettings
	}
	if strings.TrimSpace(channel.Key) != "" {
		effective.Key = channel.Key
		effective.Keys = nil
	}
	effective.ChannelInfo = channel.ChannelInfo

	if !channelUsesOfficialAssetCredential(&effective) {
		if input != nil {
			return errors.New("asset credential is only accepted for official_action_assets")
		}
		return nil
	}
	if effective.ChannelInfo.IsMultiKey {
		return errors.New("official_action_assets requires a single-key channel")
	}
	if strings.TrimSpace(effective.Key) == "" {
		return errors.New("official video API key is required")
	}
	if input != nil {
		_, err := model.NormalizeChannelAssetCredential(input)
		return err
	}
	credential, err := model.GetChannelAssetCredential(origin.Id)
	if err != nil {
		return err
	}
	if credential == nil {
		return errors.New("official_action_assets requires an asset Access Key ID and Secret Access Key")
	}
	return nil
}

func channelUsesOfficialAssetCredential(channel *model.Channel) bool {
	if channel == nil || channel.Type != constant.ChannelTypeSeedanceLink {
		return false
	}
	settings := channel.GetOtherSettings()
	return settings.AssetUpstreamProtocol.TransportProfile() == dto.AssetUpstreamProfileOfficial
}

func DeleteChannelAssetCredential(c *gin.Context) {
	channelID, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if err := model.DeleteChannelAssetCredential(channelID); err != nil {
		if errors.Is(err, model.ErrAssetCredentialProfileActive) {
			c.JSON(http.StatusConflict, gin.H{
				"success":    false,
				"message":    err.Error(),
				"error_code": "asset_credential_profile_active",
			})
			return
		}
		common.ApiError(c, err)
		return
	}
	recordManageAudit(c, "channel.asset_credential.remove", map[string]interface{}{
		"id":               channelID,
		"asset_credential": "removed",
	})
	c.JSON(http.StatusOK, gin.H{"success": true, "message": ""})
}
