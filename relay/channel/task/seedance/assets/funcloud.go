package assets

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"strconv"
	"strings"
	"unicode"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
)

const (
	funCloudMaterialRoot           = "/api/v2/open/material"
	funCloudUploadResponseMaxBytes = 1 << 20
)

type FunCloudMaterialAdapter struct{ client }

func NewFunCloudMaterialAdapter(baseURL, apiKey string, httpClient HTTPDoer) *FunCloudMaterialAdapter {
	return &FunCloudMaterialAdapter{client: newClient(baseURL, apiKey, httpClient)}
}

func (*FunCloudMaterialAdapter) Profile() dto.AssetUpstreamProfile {
	return dto.AssetUpstreamProfileFunCloudMaterial
}

func (*FunCloudMaterialAdapter) Supports(kind, mediaType string) bool {
	return kind == "general" && (mediaType == "image" || mediaType == "video" || mediaType == "audio")
}

type funCloudMaterialEnvelope struct {
	Code int             `json:"code"`
	Msg  string          `json:"msg"`
	Data json.RawMessage `json:"data"`
}

type funCloudGroup struct {
	GroupID     string `json:"groupId"`
	GroupName   string `json:"groupName"`
	Description string `json:"description"`
}

type funCloudMaterial struct {
	MaterialID       string `json:"materialId"`
	MaterialName     string `json:"materialName"`
	MaterialType     int    `json:"materialType"`
	MaterialCategory int    `json:"materialCategory"`
	GroupID          string `json:"groupId"`
	AssetURL         string `json:"assetUrl"`
	AssetStatus      string `json:"assetStatus"`
	IsAsset          *bool  `json:"isAsset"`
}

func (a *FunCloudMaterialAdapter) requestEnvelope(ctx context.Context, method, path string, body any) (funCloudMaterialEnvelope, error) {
	var envelope funCloudMaterialEnvelope
	if err := a.request(ctx, method, path, body, &envelope); err != nil {
		return envelope, err
	}
	if envelope.Code != 0 {
		return envelope, &upstreamApplicationError{
			provider: "FunCloud material", code: envelope.Code,
			definitive: envelope.Code == 10002 || envelope.Code == 10005 || envelope.Code == 10006 || envelope.Code == 30003,
		}
	}
	return envelope, nil
}

func (a *FunCloudMaterialAdapter) CheckConnectivity(ctx context.Context) error {
	_, err := a.requestEnvelope(ctx, http.MethodGet, funCloudMaterialRoot+"/list?page=1&pageSize=1", nil)
	return err
}

func (a *FunCloudMaterialAdapter) CreateGroup(ctx context.Context, req GroupRequest) (GroupResult, error) {
	envelope, err := a.requestEnvelope(ctx, http.MethodPost, funCloudMaterialRoot+"/group/create", map[string]string{
		"groupName": req.Name, "description": req.Description,
	})
	if err != nil {
		return GroupResult{}, err
	}
	var group funCloudGroup
	if common.Unmarshal(envelope.Data, &group) != nil || strings.TrimSpace(group.GroupID) == "" {
		return GroupResult{}, invalidUpstreamResponse(fmt.Errorf("FunCloud material group response is invalid"))
	}
	return GroupResult{ResourceID: strings.TrimSpace(group.GroupID), BusinessID: strings.TrimSpace(group.GroupID), Status: "active"}, nil
}

func (a *FunCloudMaterialAdapter) GetGroup(ctx context.Context, resourceID string) (GroupResult, error) {
	group, err := a.getGroup(ctx, resourceID)
	if err != nil {
		return GroupResult{}, err
	}
	return GroupResult{ResourceID: group.GroupID, BusinessID: group.GroupID, Status: "active"}, nil
}

func (a *FunCloudMaterialAdapter) getGroup(ctx context.Context, resourceID string) (funCloudGroup, error) {
	resourceID = strings.TrimSpace(resourceID)
	if resourceID == "" {
		return funCloudGroup{}, fmt.Errorf("FunCloud material group id is required")
	}
	var matched *funCloudGroup
	complete := false
	for page := 1; page <= 100; page++ {
		envelope, err := a.requestEnvelope(ctx, http.MethodGet, funCloudMaterialRoot+"/group/list?page="+strconv.Itoa(page)+"&pageSize=100", nil)
		if err != nil {
			return funCloudGroup{}, err
		}
		groups, err := decodeFunCloudList[funCloudGroup](envelope.Data)
		if err != nil {
			return funCloudGroup{}, invalidUpstreamResponse(err)
		}
		for i := range groups {
			if strings.TrimSpace(groups[i].GroupID) != resourceID {
				continue
			}
			if matched != nil {
				return funCloudGroup{}, invalidUpstreamResponse(fmt.Errorf("FunCloud material group list contains conflicting ids"))
			}
			value := groups[i]
			value.GroupID = resourceID
			matched = &value
		}
		if len(groups) < 100 {
			complete = true
			break
		}
	}
	if !complete {
		return funCloudGroup{}, invalidUpstreamResponse(fmt.Errorf("FunCloud material group pagination exceeds the verified bound"))
	}
	if matched == nil {
		return funCloudGroup{}, &upstreamHTTPError{StatusCode: http.StatusNotFound}
	}
	return *matched, nil
}

func (a *FunCloudMaterialAdapter) CreateAsset(ctx context.Context, req AssetRequest) (AssetResult, error) {
	if req.Source == nil || req.SourceMaxBytes <= 0 || strings.TrimSpace(req.GroupResourceID) == "" ||
		strings.TrimSpace(req.SourceType) == "" || strings.TrimSpace(req.SourceFilename) == "" {
		return AssetResult{}, fmt.Errorf("FunCloud virtual material upload source and group are required")
	}
	pipeReader, pipeWriter := io.Pipe()
	defer pipeReader.Close()
	multipartWriter := multipart.NewWriter(pipeWriter)
	contentType := multipartWriter.FormDataContentType()
	sourceErrCh := make(chan error, 1)
	go func() {
		disposition := mime.FormatMediaType("form-data", map[string]string{
			"name": "file", "filename": req.SourceFilename,
		})
		part, err := multipartWriter.CreatePart(textproto.MIMEHeader{
			"Content-Disposition": []string{disposition},
			"Content-Type":        []string{req.SourceType},
		})
		if err == nil {
			var copied int64
			copied, err = io.Copy(part, io.LimitReader(req.Source, req.SourceMaxBytes+1))
			if err == nil && copied > req.SourceMaxBytes {
				err = fmt.Errorf("FunCloud material source exceeds upload limit")
			}
		}
		if err == nil {
			err = multipartWriter.WriteField("groupId", req.GroupResourceID)
		}
		if err == nil {
			err = multipartWriter.WriteField("materialName", req.Name)
		}
		if closeErr := multipartWriter.Close(); err == nil {
			err = closeErr
		}
		sourceErrCh <- err
		_ = pipeWriter.CloseWithError(err)
	}()

	httpRequest, err := http.NewRequestWithContext(ctx, http.MethodPost, a.baseURL+funCloudMaterialRoot+"/virtual/upload", pipeReader)
	if err != nil {
		_ = pipeReader.CloseWithError(err)
		return AssetResult{}, err
	}
	httpRequest.Header.Set("Authorization", "Bearer "+a.apiKey)
	httpRequest.Header.Set("Accept", "application/json")
	httpRequest.Header.Set("Content-Type", contentType)
	response, err := a.http.Do(httpRequest)
	if err != nil {
		_ = pipeReader.CloseWithError(err)
		select {
		case srcErr := <-sourceErrCh:
			if srcErr != nil {
				// 阶段由 source producer 的失败确定；类别仍取 Do 返回的 transport
				// 错误，以保留 timeout/connect/reset 等可操作信号。
				return AssetResult{}, classifyTransportError(AssetStageUploadBody, err)
			}
			return AssetResult{}, classifyTransportError(AssetStageWaitResponse, err)
		default:
			// Do 可能在消费请求体前就失败。不得等待可能仍阻塞在调用方 Source.Read
			// 的生产协程；上层会在本函数返回后关闭 source body，使该协程退出。
			return AssetResult{}, classifyTransportError(AssetStageUploadBody, err)
		}
	}
	defer response.Body.Close()
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 4096))
		return AssetResult{}, &upstreamHTTPError{StatusCode: response.StatusCode}
	}
	responseData, err := io.ReadAll(io.LimitReader(response.Body, funCloudUploadResponseMaxBytes+1))
	if err != nil {
		return AssetResult{}, invalidUpstreamResponse(err)
	}
	if len(responseData) > funCloudUploadResponseMaxBytes {
		return AssetResult{}, invalidUpstreamResponse(fmt.Errorf("FunCloud material upload response exceeds the verified bound"))
	}
	var envelope funCloudMaterialEnvelope
	if err := common.Unmarshal(responseData, &envelope); err != nil {
		return AssetResult{}, invalidUpstreamResponse(err)
	}
	if envelope.Code != 0 {
		return AssetResult{}, &upstreamApplicationError{provider: "FunCloud material", code: envelope.Code}
	}
	var material funCloudMaterial
	if common.Unmarshal(envelope.Data, &material) != nil {
		return AssetResult{}, invalidUpstreamResponse(fmt.Errorf("FunCloud material upload response is invalid"))
	}
	result, err := normalizeFunCloudMaterial(material)
	if err != nil {
		return AssetResult{}, invalidUpstreamResponse(err)
	}
	return result, nil
}

func (a *FunCloudMaterialAdapter) GetAsset(ctx context.Context, resourceID string) (AssetResult, error) {
	resourceID = strings.TrimSpace(resourceID)
	if resourceID == "" {
		return AssetResult{}, fmt.Errorf("FunCloud material id is required")
	}
	var matched *funCloudMaterial
	complete := false
	for page := 1; page <= 100; page++ {
		envelope, err := a.requestEnvelope(ctx, http.MethodGet, funCloudMaterialRoot+"/list?page="+strconv.Itoa(page)+"&pageSize=100&materialCategory=2", nil)
		if err != nil {
			return AssetResult{}, err
		}
		materials, err := decodeFunCloudList[funCloudMaterial](envelope.Data)
		if err != nil {
			return AssetResult{}, invalidUpstreamResponse(err)
		}
		for i := range materials {
			if strings.TrimSpace(materials[i].MaterialID) != resourceID {
				continue
			}
			if matched != nil {
				return AssetResult{}, invalidUpstreamResponse(fmt.Errorf("FunCloud material list contains conflicting ids"))
			}
			value := materials[i]
			matched = &value
		}
		if len(materials) < 100 {
			complete = true
			break
		}
	}
	if !complete {
		return AssetResult{}, invalidUpstreamResponse(fmt.Errorf("FunCloud material pagination exceeds the verified bound"))
	}
	if matched == nil {
		return AssetResult{}, &upstreamHTTPError{StatusCode: http.StatusNotFound}
	}
	result, err := normalizeFunCloudMaterial(*matched)
	if err != nil {
		return AssetResult{}, invalidUpstreamResponse(err)
	}
	return result, nil
}

func (*FunCloudMaterialAdapter) UpdateAsset(context.Context, string, string) (AssetResult, error) {
	return AssetResult{}, ErrAssetOperationUnsupported
}

func (*FunCloudMaterialAdapter) DeleteAsset(context.Context, string) error {
	return ErrAssetOperationUnsupported
}

func decodeFunCloudList[T any](data []byte) ([]T, error) {
	var direct []T
	if common.Unmarshal(data, &direct) == nil {
		return direct, nil
	}
	var object struct {
		List    []T `json:"list"`
		Items   []T `json:"items"`
		Records []T `json:"records"`
	}
	if common.Unmarshal(data, &object) != nil {
		return nil, fmt.Errorf("FunCloud material list response is invalid")
	}
	populated := 0
	result := object.List
	if object.List != nil {
		populated++
	}
	if object.Items != nil {
		populated++
		result = object.Items
	}
	if object.Records != nil {
		populated++
		result = object.Records
	}
	if populated != 1 {
		return nil, fmt.Errorf("FunCloud material list response has no unique item collection")
	}
	return result, nil
}

func normalizeFunCloudMaterial(material funCloudMaterial) (AssetResult, error) {
	resourceID := strings.TrimSpace(material.MaterialID)
	assetURL := strings.TrimSpace(material.AssetURL)
	if resourceID == "" {
		return AssetResult{}, fmt.Errorf("FunCloud material response has no material id")
	}
	result := AssetResult{ResourceID: resourceID, BusinessID: resourceID, Status: normalizeFunCloudMaterialStatus(material.AssetStatus)}
	if assetURL != "" {
		if !strings.HasPrefix(assetURL, "asset://") {
			return AssetResult{}, fmt.Errorf("FunCloud material response has an invalid asset URL")
		}
		providerID := strings.TrimSpace(strings.TrimPrefix(assetURL, "asset://"))
		if providerID == "" || strings.ContainsAny(providerID, "\\/?#") {
			return AssetResult{}, fmt.Errorf("FunCloud material response has an invalid asset URL")
		}
		for _, character := range providerID {
			if unicode.IsSpace(character) || unicode.IsControl(character) {
				return AssetResult{}, fmt.Errorf("FunCloud material response has an invalid asset URL")
			}
		}
		result.ReferenceType = "asset_uri_id"
		result.ReferenceValue = providerID
	}
	if strings.TrimSpace(material.AssetStatus) == "" && material.IsAsset != nil && *material.IsAsset && result.ReferenceValue != "" {
		result.Status = "active"
	}
	if result.Status == "active" && result.ReferenceValue == "" {
		return AssetResult{}, fmt.Errorf("FunCloud active material has no asset URL")
	}
	return result, nil
}

func normalizeFunCloudMaterialStatus(status string) string {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "active":
		return "active"
	case "failed":
		return "failed"
	case "processing", "pending", "":
		return "processing"
	default:
		return "processing"
	}
}
