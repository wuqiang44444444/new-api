package publicmodel

import (
	"net/http"

	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/relaykit/dto"
)

func imageRelayEditAPI(api *dto.PublicModelAPI, protocol dto.ImageUpstreamProtocol, model string) {
	count, _, ok := constant.ImageRelayInputLimits(protocol, model)
	if !ok {
		return
	}
	edit := api.Image.Creation
	edit.Path = "/v1/images/edits"
	edit.RequiredOneOf = []string{"image", "images"}
	edit.Parameters = append([]dto.PublicAPIParameter(nil), edit.Parameters...)
	edit.Parameters = append(edit.Parameters,
		dto.PublicAPIParameter{Name: "image", Type: "string"},
		dto.PublicAPIParameter{Name: "images", Type: "array", ItemType: "string", MinItems: intPointer(1), MaxItems: intPointer(count)},
	)
	api.Image.Edit = &edit
	api.Image.Operations = append(api.Image.Operations, dto.PublicAPIOperation{Operation: "edit_image", Method: http.MethodPost, Path: edit.Path, Supported: true})
}
