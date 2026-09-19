package model

import "github.com/QuantumNous/new-api/constant"

// CustomerContractBatchSourceIDs identifies management sources for price
// previews, including disabled channels. It never grants routing eligibility.
func CustomerContractBatchSourceIDs(ids []int) (map[int]bool, error) {
	result := make(map[int]bool)
	if len(ids) == 0 {
		return result, nil
	}
	unique := make([]int, 0, len(ids))
	seen := make(map[int]bool)
	for _, id := range ids {
		if !seen[id] {
			unique = append(unique, id)
			seen[id] = true
		}
	}
	for start := 0; start < len(unique); start += 200 {
		var batchIDs []int
		if err := DB.Model(&Channel{}).Where("id IN ? AND type = ?", unique[start:min(start+200, len(unique))], constant.ChannelTypeAzureBatch).Pluck("id", &batchIDs).Error; err != nil {
			return nil, err
		}
		for _, id := range batchIDs {
			result[id] = true
		}
	}
	return result, nil
}
