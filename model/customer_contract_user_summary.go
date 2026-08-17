package model

func populateUserContractRuleCounts(users []*User) error {
	if len(users) == 0 {
		return nil
	}
	ids := make([]int, 0, len(users))
	for _, user := range users {
		ids = append(ids, user.Id)
	}
	type ruleCount struct {
		UserId int
		Count  int
	}
	var counts []ruleCount
	if err := DB.Model(&CustomerModelContract{}).
		Select("user_id, COUNT(*) AS count").
		Where("user_id IN ?", ids).
		Group("user_id").
		Scan(&counts).Error; err != nil {
		return err
	}
	byUser := make(map[int]int, len(counts))
	for _, item := range counts {
		byUser[item.UserId] = item.Count
	}
	for _, user := range users {
		user.ContractRuleCount = byUser[user.Id]
	}
	return nil
}
