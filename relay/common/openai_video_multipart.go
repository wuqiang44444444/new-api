package common

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/gin-gonic/gin"
)

const maxOpenAIVideoInputReferenceBytes int64 = 20 << 20

func parseOpenAIVideoMultipartRequest(c *gin.Context) (TaskSubmitReq, error) {
	form, err := common.ParseMultipartFormReusable(c)
	if err != nil {
		return TaskSubmitReq{}, err
	}
	defer form.RemoveAll()

	req := TaskSubmitReq{
		Prompt:  firstMultipartValue(form.Value, "prompt"),
		Model:   firstMultipartValue(form.Value, "model"),
		Seconds: firstMultipartValue(form.Value, "seconds"),
		Size:    firstMultipartValue(form.Value, "size"),
	}
	if duration := firstMultipartValue(form.Value, "duration"); duration != "" {
		parsed, err := strconv.Atoi(duration)
		if err != nil {
			return TaskSubmitReq{}, fmt.Errorf("duration must be an integer")
		}
		req.Duration = parsed
	}
	files := form.File["input_reference"]
	if len(files) > 1 {
		return TaskSubmitReq{}, fmt.Errorf("input_reference accepts one file")
	}
	if len(files) == 1 {
		if files[0].Size < 0 || files[0].Size > maxOpenAIVideoInputReferenceBytes {
			return TaskSubmitReq{}, fmt.Errorf("input_reference must not exceed 20 MB")
		}
		req.InputReference = "multipart://input_reference"
		req.Images = []string{req.InputReference}
	}
	if rawReference := strings.TrimSpace(firstMultipartValue(form.Value, "input_reference")); rawReference != "" {
		if len(files) != 0 {
			return TaskSubmitReq{}, fmt.Errorf("input_reference cannot contain both a file and a value")
		}
		req.InputReference = rawReference
		req.Images = []string{rawReference}
	}
	return req, nil
}

func firstMultipartValue(values map[string][]string, key string) string {
	if len(values[key]) == 0 {
		return ""
	}
	return values[key][0]
}
