package helper

import (
	"net/http"

	"github.com/QuantumNous/new-api/common"
	"github.com/gin-gonic/gin"
	"github.com/gin-gonic/gin/render"
)

type chatCompletionJSON struct {
	render.JSON
	context *gin.Context
}

func (r chatCompletionJSON) Render(w http.ResponseWriter) error {
	r.WriteContentType(w)
	data, err := common.Marshal(r.Data)
	if err != nil {
		return err
	}
	_, err = w.Write(common.ChatCompletionMessageJSON(r.context, data))
	return err
}

func RenderChatCompletionJSON(c *gin.Context, status int, value any) {
	c.Render(status, chatCompletionJSON{JSON: render.JSON{Data: value}, context: c})
}
