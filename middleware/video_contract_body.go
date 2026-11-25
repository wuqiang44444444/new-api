package middleware

import (
	"io"

	"github.com/QuantumNous/new-api/common"
	"github.com/gin-gonic/gin"
)

// The decoder consumes the public body. Distribution and retry must read the
// same normalized body as the task handler.
func replaceVideoContractBody(c *gin.Context, body []byte) error {
	storage, err := common.CreateBodyStorage(body)
	if err != nil {
		return err
	}
	common.CleanupBodyStorage(c)
	c.Set(common.KeyRequestBody, nil)
	c.Set(common.KeyBodyStorage, storage)
	c.Request.Body = io.NopCloser(readerOnlyOf(storage))
	c.Request.ContentLength = int64(len(body))
	return nil
}
