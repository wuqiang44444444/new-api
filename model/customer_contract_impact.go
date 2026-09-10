package model

// ActiveCustomerContractRule describes one rule of an enabled contract entity.
// Impact previews must reflect what new requests will actually use.
type ActiveCustomerContractRule struct {
	UserId     int    `gorm:"column:user_id"`
	UserGroup  string `gorm:"column:user_group"`
	RouteGroup string `gorm:"column:route_group"`
}

func ListActiveCustomerContractRules() ([]ActiveCustomerContractRule, error) {
	var rows []CustomerContractEntityRule
	activeContracts := DB.Model(&CustomerContract{}).Select("id").Where("enabled = ?", true)
	if err := DB.Where("contract_id IN (?)", activeContracts).Find(&rows).Error; err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return []ActiveCustomerContractRule{}, nil
	}
	ids := make([]int, 0, len(rows))
	for _, row := range rows {
		ids = append(ids, row.ContractId)
	}
	var contracts []CustomerContract
	if err := DB.Select("id", "user_id").Where("id IN ?", ids).Find(&contracts).Error; err != nil {
		return nil, err
	}
	userByContract := make(map[int]int, len(contracts))
	for _, contract := range contracts {
		userByContract[contract.Id] = contract.UserId
	}
	userIds := make([]int, 0, len(contracts))
	for _, userId := range userByContract {
		userIds = append(userIds, userId)
	}
	var users []User
	if err := DB.Select("id", "group").Where("id IN ?", userIds).Find(&users).Error; err != nil {
		return nil, err
	}
	groups := make(map[int]string, len(users))
	for _, user := range users {
		groups[user.Id] = user.Group
	}
	rules := make([]ActiveCustomerContractRule, 0, len(rows))
	for _, row := range rows {
		userId := userByContract[row.ContractId]
		rules = append(rules, ActiveCustomerContractRule{
			UserId: userId, UserGroup: groups[userId], RouteGroup: row.RouteGroup,
		})
	}
	return rules, nil
}
