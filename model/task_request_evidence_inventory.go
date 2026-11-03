package model

func ExistingTaskRequestEvidenceIDs(ids []int64) ([]int64, error) {
	var existing []int64
	if len(ids) == 0 {
		return existing, nil
	}
	err := DB.Model(&TaskRequestEvidence{}).Where("id IN ?", ids).Pluck("id", &existing).Error
	return existing, err
}
