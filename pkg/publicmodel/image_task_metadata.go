package publicmodel

import (
	"net/http"

	"github.com/QuantumNous/new-api/relaykit/dto"
)

// publishImageTaskMetadata describes an existing task executor; it does not
// enable execution or guarantee that deployment storage is currently available.
func publishImageTaskMetadata(api *dto.PublicModelAPI) {
	api.Image.Async = &dto.PublicImageAsync{
		RequestHeader: "Prefer", RequestValue: "respond-async",
		QueryPath: "/v1/tasks/{task_id}", StreamPriority: false,
	}
	api.Image.Operations = append(api.Image.Operations, dto.PublicAPIOperation{
		Operation: "query_image", Method: http.MethodGet, Path: "/v1/tasks/{task_id}", Supported: true,
	})
}
