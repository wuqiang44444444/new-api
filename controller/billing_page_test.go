package controller

import (
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"net/http/httptest"
	"testing"
)

func TestBillingPaginationRejectsMalformedAndUnboundedQueries(t *testing.T) {
	for _, query := range []string{"p=0", "p=-1", "p=oops", "p=1000001", "p=9223372036854775807", "page_size=0", "page_size=-1", "page_size=101", "page_size=oops"} {
		t.Run(query, func(t *testing.T) {
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest("GET", "/?"+query, nil)
			_, _, ok := parseBillingPage(c)
			assert.False(t, ok)
		})
	}
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("GET", "/?p=2&page_size=10", nil)
	page, size, ok := parseBillingPage(c)
	assert.True(t, ok)
	assert.Equal(t, 2, page)
	assert.Equal(t, 10, size)
}
