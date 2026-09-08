package publicmodel

import (
	"github.com/QuantumNous/new-api/relaykit/dto"
	"net/http"
	"reflect"
)

// NativeAsyncImageAPI is a projection only: it never grants runtime eligibility.
// The caller has already selected an OpenAI/Azure channel and an image model.
func NativeAsyncImageAPI(customerModel, providerModel string) *dto.PublicModelAPI {
	api := NativeImageAPI(providerModel)
	api.Image.Creation.Model = customerModel
	api.Image.Creation.Parameters[0] = fixedParameter("model", "string", true, customerModel)
	api.Image.Async = &dto.PublicImageAsync{RequestHeader: "Prefer", RequestValue: "respond-async", QueryPath: "/v1/tasks/{task_id}", StreamPriority: true}
	api.Image.Operations = append(api.Image.Operations, dto.PublicAPIOperation{Operation: "query_image", Method: http.MethodGet, Path: "/v1/tasks/{task_id}", Supported: true})
	// DALL-E 3 has no native edit operation. Other native profiles publish the
	// gateway edit surface; provider-specific support remains provider-owned.
	if providerModel != "dall-e-3" {
		edit := api.Image.Creation
		edit.Path = "/v1/images/edits"
		edit.ContentType = "multipart/form-data"
		edit.RequiredFields = []string{"model", "prompt", "image"}
		edit.Parameters = append([]dto.PublicAPIParameter(nil), edit.Parameters...)
		edit.Parameters = append(edit.Parameters, dto.PublicAPIParameter{Name: "image", Type: "file", Required: true}, dto.PublicAPIParameter{Name: "mask", Type: "file"})
		api.Image.Edit = &edit
		api.Image.Operations = append(api.Image.Operations, dto.PublicAPIOperation{Operation: "edit_image", Method: http.MethodPost, Path: edit.Path, Supported: true})
	}
	return api
}

// CommonNativeImageAPI preserves an identical native creation contract while
// publishing optional operations only when every selectable channel agrees.
// Callers must establish that both projections came from native image paths;
// strict provider contracts still require complete equality.
func CommonNativeImageAPI(left, right *dto.PublicModelAPI) *dto.PublicModelAPI {
	leftBase, rightBase := *left, *right
	leftImage, rightImage := *left.Image, *right.Image
	leftBase.Image, rightBase.Image = &leftImage, &rightImage
	leftImage.Edit, rightImage.Edit = nil, nil
	leftImage.Async, rightImage.Async = nil, nil
	leftImage.Operations, rightImage.Operations = nil, nil
	if !reflect.DeepEqual(leftBase, rightBase) {
		return nil
	}
	if reflect.DeepEqual(left.Image.Edit, right.Image.Edit) {
		leftImage.Edit = left.Image.Edit
	}
	if reflect.DeepEqual(left.Image.Async, right.Image.Async) {
		leftImage.Async = left.Image.Async
	}
	for _, operation := range left.Image.Operations {
		if (operation.Operation == "edit_image" && leftImage.Edit == nil) ||
			(operation.Operation == "query_image" && leftImage.Async == nil) {
			continue
		}
		for _, other := range right.Image.Operations {
			if operation == other {
				leftImage.Operations = append(leftImage.Operations, operation)
				break
			}
		}
	}
	return &leftBase
}
