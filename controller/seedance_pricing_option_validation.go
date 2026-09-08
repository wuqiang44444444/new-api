package controller

import (
	"encoding/json"
	"sort"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
)

func validateSeedancePricingOption(c *gin.Context, option OptionUpdateRequest) bool {
	switch option.Key {
	case "ModelPrice", "ModelRatio", "CompletionRatio", "CacheRatio", "CreateCacheRatio", "ImageRatio", "AudioRatio", "AudioCompletionRatio",
		"billing_setting.billing_expr", "billing_setting.billing_mode", "task_billing_setting.preconsume_tokens":
	default:
		return true
	}
	if model.DB == nil {
		return true
	}
	var values map[string]json.RawMessage
	if err := common.UnmarshalJsonStr(option.Value.(string), &values); err != nil {
		common.ApiErrorMsg(c, err.Error())
		return false
	}
	names := make([]string, 0, len(values))
	for name := range values {
		names = append(names, name)
	}
	sort.Strings(names)
	if err := model.ValidateSeedancePricingModelNames(names); err != nil {
		common.ApiErrorMsg(c, err.Error())
		return false
	}
	return true
}
