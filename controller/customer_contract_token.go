package controller

import (
	"slices"

	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
)

// An unrelated edit must preserve the native routing configuration saved on a
// contract key. Changed routing still goes through native group validation.
func setUpdatedTokenAutoGroups(c *gin.Context, token *model.Token, previous model.Token, groups []string) bool {
	if previous.ContractId > 0 && previous.Group == "auto" {
		stored, err := previous.GetAutoGroups()
		if err == nil && slices.Equal(stored, groups) {
			return true
		}
	}
	return setTokenAutoGroups(c, token, groups)
}
