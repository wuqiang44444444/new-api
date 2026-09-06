package model

import (
	"reflect"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/pkg/publicmodel"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/setting/model_setting"
)

// GetPublicMediaModelAPIs projects the contracts of every channel that runtime
// routing may select at the caller's first matching group and highest priority.
// A contract is published only when those channels agree. Internal protocol and
// mapped Provider model identities never leave this function.
func GetPublicMediaModelAPIs(modelNames []string, groups []string) (map[string]*dto.PublicModelAPI, error) {
	modelNames = normalizeLookupValues(modelNames)
	groups = normalizeLookupValues(groups)
	result := make(map[string]*dto.PublicModelAPI)
	if len(modelNames) == 0 {
		return result, nil
	}
	allGroups := len(groups) == 0

	type channelCandidate struct {
		Model     string
		GroupName string
		ChannelID int
		Priority  int64
	}
	var rows []channelCandidate
	query := DB.Table("abilities").
		Select("abilities.model as model, abilities."+commonGroupCol+" as group_name, abilities.channel_id as channel_id, COALESCE(abilities.priority, 0) as priority").
		Joins("JOIN channels ON abilities.channel_id = channels.id").
		Where("abilities.model IN ? AND abilities.enabled = ? AND channels.status = ?", modelNames, true, common.ChannelStatusEnabled)
	if !allGroups {
		query = query.Where("abilities."+commonGroupCol+" IN ?", groups)
	}
	if err := query.Scan(&rows).Error; err != nil {
		return nil, err
	}

	channelIDs := make([]int, 0, len(rows))
	seenChannels := make(map[int]struct{}, len(rows))
	for _, row := range rows {
		if _, exists := seenChannels[row.ChannelID]; exists {
			continue
		}
		seenChannels[row.ChannelID] = struct{}{}
		channelIDs = append(channelIDs, row.ChannelID)
	}
	var channels []Channel
	if len(channelIDs) > 0 {
		if err := DB.Where("id IN ?", channelIDs).Find(&channels).Error; err != nil {
			return nil, err
		}
	}
	channelByID := make(map[int]*Channel, len(channels))
	for index := range channels {
		channelByID[channels[index].Id] = &channels[index]
	}

	candidatesByModelAndGroup := make(map[string]map[string][]channelCandidate)
	for _, row := range rows {
		byGroup := candidatesByModelAndGroup[row.Model]
		if byGroup == nil {
			byGroup = make(map[string][]channelCandidate)
			candidatesByModelAndGroup[row.Model] = byGroup
		}
		byGroup[row.GroupName] = append(byGroup[row.GroupName], row)
	}

	for _, modelName := range modelNames {
		selectedGroups := groups
		if allGroups {
			selectedGroups = make([]string, 0, len(candidatesByModelAndGroup[modelName]))
			for group := range candidatesByModelAndGroup[modelName] {
				selectedGroups = append(selectedGroups, group)
			}
		} else {
			for _, group := range groups {
				if len(candidatesByModelAndGroup[modelName][group]) > 0 {
					selectedGroups = []string{group}
					break
				}
			}
		}

		var candidates []channelCandidate
		for _, group := range selectedGroups {
			groupCandidates := candidatesByModelAndGroup[modelName][group]
			if len(groupCandidates) == 0 {
				continue
			}
			maxPriority := groupCandidates[0].Priority
			for _, candidate := range groupCandidates[1:] {
				if candidate.Priority > maxPriority {
					maxPriority = candidate.Priority
				}
			}
			for _, candidate := range groupCandidates {
				if candidate.Priority == maxPriority {
					candidates = append(candidates, candidate)
				}
			}
		}
		if len(candidates) == 0 {
			continue
		}

		var contract *dto.PublicModelAPI
		sawMediaContract := false
		sawMissingContract := false
		consistent := true
		for _, candidate := range candidates {
			channel := channelByID[candidate.ChannelID]
			if channel == nil {
				sawMissingContract = true
				continue
			}

			var api *dto.PublicModelAPI
			if channel.Type == constant.ChannelTypeAsyncImage {
				providerModel, err := mappedCustomerModel(channel, modelName)
				if err != nil {
					common.SysLog("public image model mapping error: " + err.Error())
					sawMissingContract = true
					continue
				}
				api, _ = publicmodel.ImageAPI(modelName, channel.GetOtherSettings().ImageUpstreamProtocol, providerModel)
			} else if channel.Type == constant.ChannelTypeGemini || channel.Type == constant.ChannelTypeVertexAi {
				// gemini_image 族按管理员映射后的 Provider 模型识别
				//（imagine 登记表），不从客户模型名推断。
				providerModel, err := mappedCustomerModel(channel, modelName)
				if err != nil {
					common.SysLog("public image model mapping error: " + err.Error())
					sawMissingContract = true
					continue
				}
				if model_setting.IsGeminiModelSupportImagine(providerModel) {
					api = publicmodel.GeminiImageAPI(modelName)
				} else if common.IsImageGenerationModel(modelName) {
					api = publicmodel.NativeImageAPI(modelName)
				}
			} else if channel.Type == constant.ChannelTypeSora {
				api = publicmodel.NativeVideoAPI(modelName)
			} else if common.IsImageGenerationModel(modelName) {
				api = publicmodel.NativeImageAPI(modelName)
			}
			if api == nil {
				sawMissingContract = true
				continue
			}
			sawMediaContract = true
			if contract == nil {
				contract = api
				continue
			}
			if !reflect.DeepEqual(contract, api) {
				consistent = false
				break
			}
		}
		if sawMediaContract && !sawMissingContract && consistent {
			result[modelName] = contract
		} else if sawMediaContract {
			common.SysLog("public media model contract is inconsistent across selectable channels: model=" + modelName)
		}
	}
	return result, nil
}

func mappedCustomerModel(channel *Channel, customerModel string) (string, error) {
	mapping := make(map[string]string)
	if raw := strings.TrimSpace(channel.GetModelMapping()); raw != "" && raw != "{}" {
		if err := common.UnmarshalJsonStr(raw, &mapping); err != nil {
			return "", err
		}
	}
	providerModel, _, err := ResolveModelMapping(customerModel, mapping)
	return providerModel, err
}
