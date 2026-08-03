package middleware

import (
	"bytes"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	relaycommon "github.com/QuantumNous/new-api/relay/common"

	"github.com/gin-gonic/gin"
)

func KlingRequestConvert() func(c *gin.Context) {
	return func(c *gin.Context) {
		if c.Request.Method != http.MethodPost {
			c.Next()
			return
		}
		var originalReq map[string]interface{}
		if err := common.UnmarshalBodyReusable(c, &originalReq); err != nil {
			abortKlingVideo(c, http.StatusBadRequest, "invalid request body")
			return
		}
		if err := rejectUnknownVideoFields(
			originalReq,
			"prompt", "image", "image_tail", "negative_prompt", "mode", "duration",
			"aspect_ratio", "model_name", "cfg_scale", "static_mask",
			"dynamic_masks", "camera_control", "callback_url", "external_task_id",
		); err != nil {
			abortKlingVideo(c, http.StatusBadRequest, err.Error())
			return
		}
		if err := rejectUnknownKlingVideoFields(originalReq); err != nil {
			abortKlingVideo(c, http.StatusBadRequest, err.Error())
			return
		}
		var official dto.KlingVideoCreateRequest
		if err := decodeTypedVideoRequest(originalReq, &official); err != nil {
			abortKlingVideo(c, http.StatusBadRequest, "invalid request body")
			return
		}
		modelName := strings.TrimSpace(videoStringValue(official.ModelName))
		isImageToVideo := strings.HasSuffix(c.Request.URL.Path, "/image2video")
		if isImageToVideo && strings.TrimSpace(videoStringValue(official.Image)) == "" {
			abortKlingVideo(c, http.StatusBadRequest, "image is required for image2video")
			return
		}
		if !isImageToVideo && strings.TrimSpace(videoStringValue(official.Image)) != "" {
			abortKlingVideo(c, http.StatusBadRequest, "image is not supported for text2video")
			return
		}
		if !isImageToVideo && strings.TrimSpace(videoStringValue(official.ImageTail)) != "" {
			abortKlingVideo(c, http.StatusBadRequest, "image_tail is not supported for text2video")
			return
		}
		internal := relaycommon.TaskSubmitReq{
			Model:  modelName,
			Prompt: videoStringValue(official.Prompt),
			Mode:   videoStringValue(official.Mode),
			Image:  videoStringValue(official.Image),
		}
		if durationValue := videoStringValue(official.Duration); durationValue != "" {
			duration, err := strconv.Atoi(durationValue)
			if err != nil {
				abortKlingVideo(c, http.StatusBadRequest, "duration must be an integer string")
				return
			}
			internal.Duration = duration
		}
		if image := videoStringValue(official.Image); image != "" {
			internal.Images = append(internal.Images, image)
		}
		if imageTail := videoStringValue(official.ImageTail); imageTail != "" {
			internal.Images = append(internal.Images, imageTail)
		}
		relaycommon.SetVideoContractRequest(c, dto.VideoContractRequest{
			ContractID: dto.VideoContractKlingV1,
			Kling:      &official,
		})
		jsonData, err := common.Marshal(internal)
		if err != nil {
			abortKlingVideo(c, http.StatusInternalServerError, "failed to prepare request")
			return
		}

		c.Request.Body = io.NopCloser(bytes.NewBuffer(jsonData))
		c.Request.ContentLength = int64(len(jsonData))
		originalPath := c.Request.URL.Path
		c.Request.URL.Path = "/v1/video/generations"
		if videoStringValue(official.Image) == "" {
			c.Set("action", constant.TaskActionTextGenerate)
		}
		c.Set(common.KeyRequestBody, jsonData)
		c.Next()
		c.Request.URL.Path = originalPath
	}
}

func rejectUnknownKlingVideoFields(body map[string]any) error {
	if dynamicMasks, exists := body["dynamic_masks"]; exists {
		if err := rejectUnknownNestedVideoArrayFields(dynamicMasks, "dynamic_masks", "mask", "trajectories"); err != nil {
			return err
		}
		for index, rawMask := range dynamicMasks.([]any) {
			mask := rawMask.(map[string]any)
			if trajectories, ok := mask["trajectories"]; ok {
				if err := rejectUnknownNestedVideoArrayFields(
					trajectories,
					"dynamic_masks["+strconv.Itoa(index)+"].trajectories",
					"x", "y",
				); err != nil {
					return err
				}
			}
		}
	}
	if cameraControl, exists := body["camera_control"]; exists {
		if err := rejectUnknownNestedVideoFields(cameraControl, "camera_control", "type", "config"); err != nil {
			return err
		}
		camera := cameraControl.(map[string]any)
		if config, ok := camera["config"]; ok {
			if err := rejectUnknownNestedVideoFields(
				config,
				"camera_control.config",
				"horizontal", "vertical", "pan", "tilt", "roll", "zoom",
			); err != nil {
				return err
			}
		}
	}
	return nil
}
