package common

import (
	"errors"
	"fmt"
	"strings"

	"github.com/QuantumNous/new-api/dto"
	"github.com/gin-gonic/gin"
)

const videoContractRequestContextKey = "video_contract_request"

type VideoContractError struct {
	Code    string
	Message string
}

func (e *VideoContractError) Error() string {
	if e == nil {
		return ""
	}
	return e.Message
}

func NewVideoContractError(code, message string) error {
	return &VideoContractError{Code: code, Message: message}
}

func AsVideoContractError(err error) (*VideoContractError, bool) {
	var contractErr *VideoContractError
	ok := errors.As(err, &contractErr)
	return contractErr, ok
}

func SetVideoContractRequest(c *gin.Context, request dto.VideoContractRequest) {
	c.Set(videoContractRequestContextKey, request)
}

func GetVideoContractRequest(c *gin.Context) (dto.VideoContractRequest, bool) {
	if c == nil {
		return dto.VideoContractRequest{}, false
	}
	value, exists := c.Get(videoContractRequestContextKey)
	if !exists {
		return dto.VideoContractRequest{}, false
	}
	request, ok := value.(dto.VideoContractRequest)
	return request, ok
}

func VideoContractModel(c *gin.Context) (string, bool) {
	request, ok := GetVideoContractRequest(c)
	if !ok {
		return "", false
	}
	switch {
	case request.ModelArk != nil:
		return strings.TrimSpace(request.ModelArk.Model), true
	case request.Kling != nil:
		return videoOptionalString(request.Kling.ModelName), true
	case request.Jimeng != nil:
		return strings.TrimSpace(request.Jimeng.ReqKey), true
	default:
		return "", false
	}
}

func VideoContractAssetReferences(c *gin.Context) ([]string, bool) {
	request, ok := GetVideoContractRequest(c)
	if !ok {
		return nil, false
	}
	values := make([]string, 0)
	switch {
	case request.ModelArk != nil:
		for _, item := range request.ModelArk.Content {
			switch {
			case item.ImageURL != nil:
				values = append(values, item.ImageURL.URL)
			case item.VideoURL != nil:
				values = append(values, item.VideoURL.URL)
			case item.AudioURL != nil:
				values = append(values, item.AudioURL.URL)
			}
		}
	case request.Kling != nil:
		values = append(values, videoOptionalString(request.Kling.Image), videoOptionalString(request.Kling.ImageTail), videoOptionalString(request.Kling.StaticMask))
		for _, mask := range request.Kling.DynamicMasks {
			values = append(values, videoOptionalString(mask.Mask))
		}
	case request.Jimeng != nil:
		values = append(values, request.Jimeng.ImageURLs...)
	}
	return values, true
}

func RewriteVideoContractAssetReferences(c *gin.Context, replacements map[string]string) error {
	request, ok := GetVideoContractRequest(c)
	if !ok {
		return nil
	}
	replace := func(value string) string {
		if replacement, exists := replacements[strings.TrimSpace(value)]; exists {
			return replacement
		}
		return value
	}
	switch {
	case request.ModelArk != nil:
		for index := range request.ModelArk.Content {
			item := &request.ModelArk.Content[index]
			if item.ImageURL != nil {
				item.ImageURL.URL = replace(item.ImageURL.URL)
			}
			if item.VideoURL != nil {
				item.VideoURL.URL = replace(item.VideoURL.URL)
			}
			if item.AudioURL != nil {
				item.AudioURL.URL = replace(item.AudioURL.URL)
			}
		}
	case request.Kling != nil:
		rewriteOptionalVideoString(request.Kling.Image, replace)
		rewriteOptionalVideoString(request.Kling.ImageTail, replace)
		rewriteOptionalVideoString(request.Kling.StaticMask, replace)
		for index := range request.Kling.DynamicMasks {
			rewriteOptionalVideoString(request.Kling.DynamicMasks[index].Mask, replace)
		}
	case request.Jimeng != nil:
		for index := range request.Jimeng.ImageURLs {
			request.Jimeng.ImageURLs[index] = replace(request.Jimeng.ImageURLs[index])
		}
	default:
		return fmt.Errorf("video contract %q has no typed request", request.ContractID)
	}
	SetVideoContractRequest(c, request)
	return nil
}

func videoOptionalString(value *string) string {
	if value == nil {
		return ""
	}
	return strings.TrimSpace(*value)
}

func rewriteOptionalVideoString(value *string, replace func(string) string) {
	if value != nil {
		*value = replace(*value)
	}
}
