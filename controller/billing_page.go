package controller

import (
	"github.com/QuantumNous/new-api/common"
	"github.com/gin-gonic/gin"
	"strconv"
)

// Validate raw parameters instead of silently turning malformed or oversized
// values into the first page. The bound also prevents offset overflow.
func parseBillingPage(c *gin.Context) (int, int, bool) {
	page, err := strconv.Atoi(c.DefaultQuery("p", "1"))
	size, sizeErr := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	if err != nil || sizeErr != nil || page < 1 || page > 1000000 || size < 1 || size > 100 {
		common.ApiErrorMsg(c, "invalid pagination")
		return 0, 0, false
	}
	return page, size, true
}
