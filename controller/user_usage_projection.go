package controller

import "github.com/QuantumNous/new-api/model"

// Keep the existing user response, overriding only an unresolved cumulative
// amount. The underlying user/model is never mutated by a read response.
type userUsageView struct {
	*model.User
	UsedQuota *int `json:"used_quota"`
}

func projectUserUsage(user *model.User) userUsageView {
	return userUsageView{User: user, UsedQuota: model.UserUsedQuotaForDisplay(user)}
}

func projectUsersUsage(users []*model.User) []userUsageView {
	result := make([]userUsageView, 0, len(users))
	for _, user := range users {
		result = append(result, projectUserUsage(user))
	}
	return result
}
