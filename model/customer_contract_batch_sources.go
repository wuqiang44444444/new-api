package model

import "github.com/QuantumNous/new-api/constant"

// CustomerContractBatchSourceIDs identifies management sources for price
// previews, including disabled channels. It never grants routing eligibility.
func CustomerContractBatchSourceIDs(ids []int) (map[int]bool, error) {
	result := make(map[int]bool)
	if len(ids) == 0 {
		return result, nil
	}
	var batchIDs []int
	if err := DB.Model(&Channel{}).Where("id IN ? AND type = ?", ids, constant.ChannelTypeAzureBatch).Pluck("id", &batchIDs).Error; err != nil {
		return nil, err
	}
	for _, id := range batchIDs {
		result[id] = true
	}
	return result, nil
}
