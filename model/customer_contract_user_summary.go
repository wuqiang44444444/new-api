package model

// populateUserContractRuleCounts fills User.ContractRuleCount with the total
// number of rules across all contract entities owned by each user.
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
	if err := DB.Model(&CustomerContractEntityRule{}).
		Select("customer_contracts.user_id AS user_id, COUNT(*) AS count").
		Joins("JOIN customer_contracts ON customer_contracts.id = customer_contract_entity_rules.contract_id").
		Where("customer_contracts.user_id IN ?", ids).
		Group("customer_contracts.user_id").
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
