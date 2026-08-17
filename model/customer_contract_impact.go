package model

type ActiveCustomerContractRule struct {
	UserId     int    `gorm:"column:user_id"`
	UserGroup  string `gorm:"column:user_group"`
	RouteGroup string `gorm:"column:route_group"`
}

func ListActiveCustomerContractRules() ([]ActiveCustomerContractRule, error) {
	var rows []CustomerModelContract
	activeUsers := DB.Model(&User{}).Select("id").Where("contract_mode = ?", true)
	if err := DB.Where("user_id IN (?)", activeUsers).Find(&rows).Error; err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return []ActiveCustomerContractRule{}, nil
	}
	ids := make([]int, 0, len(rows))
	for _, row := range rows {
		ids = append(ids, row.UserId)
	}
	var users []User
	if err := DB.Select("id", "group").Where("id IN ?", ids).Find(&users).Error; err != nil {
		return nil, err
	}
	groups := make(map[int]string, len(users))
	for _, user := range users {
		groups[user.Id] = user.Group
	}
	rules := make([]ActiveCustomerContractRule, 0, len(rows))
	for _, row := range rows {
		rules = append(rules, ActiveCustomerContractRule{
			UserId: row.UserId, UserGroup: groups[row.UserId], RouteGroup: row.RouteGroup,
		})
	}
	return rules, nil
}
