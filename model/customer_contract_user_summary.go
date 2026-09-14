package model

// UserContractSummary describes owned entities, not permissions of every API key.
type UserContractSummary struct {
	Total   int `json:"total"`
	Enabled int `json:"enabled"`
}

// populateUserContractSummaries uses only current entities. Legacy user flags
// remain untouched as migration input and never determine this projection.
func populateUserContractSummaries(users []*User) error {
	if len(users) == 0 {
		return nil
	}
	ids := make([]int, 0, len(users))
	for _, user := range users {
		ids = append(ids, user.Id)
	}
	type contractCount struct {
		UserId  int
		Enabled bool
		Count   int
	}
	var counts []contractCount
	if err := DB.Model(&CustomerContract{}).
		Select("user_id, enabled, COUNT(*) AS count").
		Where("user_id IN ?", ids).
		Group("user_id, enabled").
		Scan(&counts).Error; err != nil {
		return err
	}
	byUser := make(map[int]UserContractSummary, len(users))
	for _, item := range counts {
		summary := byUser[item.UserId]
		summary.Total += item.Count
		if item.Enabled {
			summary.Enabled += item.Count
		}
		byUser[item.UserId] = summary
	}
	for _, user := range users {
		summary := byUser[user.Id]
		user.ContractSummary = &summary
	}
	return nil
}
