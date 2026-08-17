package common

import (
	"errors"
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

func videoOptionalString(value *string) string {
	if value == nil {
		return ""
	}
	return strings.TrimSpace(*value)
}
