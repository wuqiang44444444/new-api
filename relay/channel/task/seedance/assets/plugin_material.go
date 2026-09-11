package assets

import (
	"context"
	"fmt"
	"github.com/QuantumNous/new-api/common"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"strings"
)

const funCloudUploadResponseMaxBytes = 1 << 20

// Streaming and pagination limits belong to the host. The artifact supplies
// only the multipart fields and normalized observations, never media bytes.
func (a *PluginAssetAdapter) uploadMaterial(ctx context.Context, path string, descriptor any, req AssetRequest) (*http.Response, error) {
	body, ok := descriptor.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("invalid upload descriptor")
	}
	multipartBody, ok := body["multipart"].(map[string]any)
	if !ok {
		return nil, fmt.Errorf("invalid multipart descriptor")
	}
	fileField, ok := multipartBody["fileField"].(string)
	if !ok || fileField != "file" {
		return nil, fmt.Errorf("invalid upload file slot")
	}
	fields, ok := multipartBody["fields"].(map[string]any)
	if !ok || len(fields) != 2 {
		return nil, fmt.Errorf("invalid upload fields")
	}
	groupID, groupOK := fields["groupId"].(string)
	materialName, nameOK := fields["materialName"].(string)
	if !groupOK || !nameOK || groupID != req.GroupResourceID || materialName != req.Name {
		return nil, fmt.Errorf("upload descriptor changed the accepted material")
	}
	if req.Source == nil || req.SourceMaxBytes <= 0 || strings.TrimSpace(req.GroupResourceID) == "" ||
		strings.TrimSpace(req.SourceType) == "" || strings.TrimSpace(req.SourceFilename) == "" {
		return nil, fmt.Errorf("FunCloud virtual material upload source and group are required")
	}
	pipeReader, pipeWriter := io.Pipe()
	defer pipeReader.Close()
	multipartWriter := multipart.NewWriter(pipeWriter)
	contentType := multipartWriter.FormDataContentType()
	sourceErrCh := make(chan error, 1)
	go func() {
		disposition := mime.FormatMediaType("form-data", map[string]string{
			"name": fileField, "filename": req.SourceFilename,
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
			err = multipartWriter.WriteField("groupId", groupID)
		}
		if err == nil {
			err = multipartWriter.WriteField("materialName", materialName)
		}
		if closeErr := multipartWriter.Close(); err == nil {
			err = closeErr
		}
		sourceErrCh <- err
		_ = pipeWriter.CloseWithError(err)
	}()

	httpRequest, err := http.NewRequestWithContext(ctx, http.MethodPost, a.bearer.baseURL+path, pipeReader)
	if err != nil {
		_ = pipeReader.CloseWithError(err)
		return nil, err
	}
	httpRequest.Header.Set("Authorization", "Bearer "+a.bearer.apiKey)
	httpRequest.Header.Set("Accept", "application/json")
	httpRequest.Header.Set("Content-Type", contentType)
	response, err := a.bearer.http.Do(httpRequest)
	if err != nil {
		_ = pipeReader.CloseWithError(err)
		select {
		case srcErr := <-sourceErrCh:
			if srcErr != nil {
				// 阶段由 source producer 的失败确定；类别仍取 Do 返回的 transport
				// 错误，以保留 timeout/connect/reset 等可操作信号。
				return nil, classifyTransportError(AssetStageUploadBody, err)
			}
			return nil, classifyTransportError(AssetStageWaitResponse, err)
		default:
			// Do 可能在消费请求体前就失败。不得等待可能仍阻塞在调用方 Source.Read
			// 的生产协程；上层会在本函数返回后关闭 source body，使该协程退出。
			return nil, classifyTransportError(AssetStageUploadBody, err)
		}
	}
	return response, nil
}

func (a *PluginAssetAdapter) findMaterial(ctx context.Context, operation, id string, target any) error {
	id = strings.TrimSpace(id)
	if id == "" {
		return fmt.Errorf("material resource id is required")
	}
	var matched map[string]any
	complete := false
	for page := 1; page <= 100; page++ {
		var result struct {
			Items []map[string]any
			Count int
		}
		if err := a.execute(ctx, operation, map[string]any{"id": id, "page": page}, &result); err != nil {
			return err
		}
		if result.Count < 0 || result.Count > 100 || len(result.Items) > result.Count {
			return invalidUpstreamResponse(fmt.Errorf("invalid material page"))
		}
		for _, item := range result.Items {
			if item["ResourceID"] != id || matched != nil {
				return invalidUpstreamResponse(fmt.Errorf("material list contains conflicting ids"))
			}
			matched = item
		}
		if result.Count < 100 {
			complete = true
			break
		}
	}
	if !complete {
		return invalidUpstreamResponse(fmt.Errorf("material pagination exceeds the verified bound"))
	}
	if matched == nil {
		return &upstreamHTTPError{StatusCode: http.StatusNotFound}
	}
	data, err := common.Marshal(matched)
	if err != nil {
		return err
	}
	return common.Unmarshal(data, target)
}
