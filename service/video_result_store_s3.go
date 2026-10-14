package service

import (
	"bytes"
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/model"
)

// s3ArtifactStore 的视频结果托管能力（本地扩展，独立文件零改动既有实现）。
// 视频采用分片上传：单片缓冲有界（videoResultUploadPartSize），任一时刻不
// 保存完整视频正文；失败时中止分片上传，部分上传不成为可交付对象。

const s3VideoRequestTimeout = 5 * time.Minute

type s3MultipartPart struct {
	XMLName    xml.Name `xml:"Part"`
	PartNumber int      `xml:"PartNumber"`
	ETag       string   `xml:"ETag"`
}

func (s *s3ArtifactStore) putVideoResultObject(ctx context.Context, objectKey, mimeType string, content io.Reader, maxBytes int64) (int64, error) {
	bounded := newBoundVideoResultReader(content, maxBytes)
	part, eof, err := readVideoResultPart(bounded, videoResultUploadPartSize)
	if err != nil {
		return 0, err
	}
	if len(part) == 0 && eof {
		return 0, errors.New("upstream video result is empty")
	}
	if eof {
		// 单片即完整：直接 PUT，不为小文件建立分片上传。
		resp, err := s.videoResultRequest(ctx, http.MethodPut, s.objectURL(objectKey), "", mimeType, part)
		if err != nil {
			return 0, err
		}
		_ = resp.Body.Close()
		return int64(len(part)), nil
	}
	return s.putVideoResultMultipart(ctx, objectKey, mimeType, bounded, part)
}

// putVideoResultMultipart 继续读取余下分片并完成分片上传；任何失败都中止
// 上传，已传分片不产生可见对象。
func (s *s3ArtifactStore) putVideoResultMultipart(ctx context.Context, objectKey, mimeType string, bounded *boundVideoResultReader, firstPart []byte) (int64, error) {
	uploadID, err := s.initiateVideoResultMultipart(ctx, objectKey, mimeType)
	if err != nil {
		return 0, err
	}
	completed := false
	defer func() {
		if !completed {
			s.abortVideoResultMultipart(context.Background(), objectKey, uploadID)
		}
	}()
	parts := make([]s3MultipartPart, 0, 2)
	total := int64(0)
	buffer := firstPart
	for partNumber := 1; len(buffer) > 0; partNumber++ {
		etag, uploadErr := s.uploadVideoResultPart(ctx, objectKey, uploadID, partNumber, mimeType, buffer)
		if uploadErr != nil {
			return 0, uploadErr
		}
		total += int64(len(buffer))
		parts = append(parts, s3MultipartPart{PartNumber: partNumber, ETag: etag})
		if len(buffer) < int(videoResultUploadPartSize) {
			break
		}
		buffer, _, uploadErr = readVideoResultPart(bounded, videoResultUploadPartSize)
		if uploadErr != nil {
			return 0, uploadErr
		}
	}
	if len(parts) == 0 {
		return 0, errors.New("upstream video result is empty")
	}
	completion, err := xml.Marshal(struct {
		XMLName xml.Name          `xml:"CompleteMultipartUpload"`
		Parts   []s3MultipartPart `xml:"Parts"`
	}{Parts: parts})
	if err != nil {
		return 0, err
	}
	var completionBody []byte
	completionBody, err = s.videoResultRequestForBody(ctx, http.MethodPost, s.objectURL(objectKey), "uploadId="+s3QueryEscape(uploadID), "application/xml", completion)
	if err != nil {
		return 0, err
	}
	// HTTP 200 仅表示开始处理；必须取得完整的成功 XML 才能发布对象。
	var outcome struct {
		XMLName xml.Name
		ETag    string `xml:"ETag"`
	}
	decoder := xml.NewDecoder(bytes.NewReader(bytes.TrimSpace(completionBody)))
	if err := decoder.Decode(&outcome); err != nil {
		return 0, errors.New("object store returned invalid multipart completion")
	}
	if _, err := decoder.Token(); err != io.EOF {
		return 0, errors.New("object store returned trailing multipart completion data")
	}
	if outcome.XMLName.Local != "CompleteMultipartUploadResult" || strings.TrimSpace(outcome.ETag) == "" {
		return 0, errors.New("object store did not confirm multipart completion")
	}
	completed = true
	return total, nil
}

func (s *s3ArtifactStore) initiateVideoResultMultipart(ctx context.Context, objectKey, mimeType string) (string, error) {
	body, err := s.videoResultRequestForBody(ctx, http.MethodPost, s.objectURL(objectKey), "uploads", mimeType, nil)
	if err != nil {
		return "", err
	}
	var initiation struct {
		UploadID string `xml:"UploadId"`
	}
	if err := xml.Unmarshal(body, &initiation); err != nil || initiation.UploadID == "" {
		return "", errors.New("object store did not return a multipart upload id")
	}
	return initiation.UploadID, nil
}

func (s *s3ArtifactStore) uploadVideoResultPart(ctx context.Context, objectKey, uploadID string, partNumber int, mimeType string, data []byte) (string, error) {
	query := "partNumber=" + s3QueryEscape(fmt.Sprintf("%d", partNumber)) + "&uploadId=" + s3QueryEscape(uploadID)
	resp, err := s.videoResultRequest(ctx, http.MethodPut, s.objectURL(objectKey), query, mimeType, data)
	if err != nil {
		return "", err
	}
	defer func() { _ = resp.Body.Close() }()
	etag := resp.Header.Get("ETag")
	if etag == "" {
		return "", errors.New("object store did not return a part etag")
	}
	return etag, nil
}

func (s *s3ArtifactStore) abortVideoResultMultipart(ctx context.Context, objectKey, uploadID string) {
	_, _ = s.videoResultRequestForBody(ctx, http.MethodDelete, s.objectURL(objectKey), "uploadId="+s3QueryEscape(uploadID), "", nil)
}

// videoResultRequest 签名并执行一次对象存储请求；payload 为 nil 表示空正文。
// 调用方负责关闭返回响应的 Body。
func (s *s3ArtifactStore) videoResultRequest(ctx context.Context, method, rawURL, query, mimeType string, payload []byte) (*http.Response, error) {
	requestCtx, cancel := context.WithTimeout(ctx, s3VideoRequestTimeout)
	requestURL := rawURL
	if query != "" {
		requestURL += "?" + query
	}
	req, err := http.NewRequestWithContext(requestCtx, method, requestURL, nil)
	if err != nil {
		cancel()
		return nil, err
	}
	req.Host = req.URL.Host
	if mimeType != "" {
		req.Header.Set("Content-Type", mimeType)
	}
	req.ContentLength = int64(len(payload))
	req.Body = newBytesReader(payload)
	req.GetBody = func() (io.ReadCloser, error) { return newBytesReader(payload), nil }
	SigV4SignRequest(req, s.credentials, s.region(), hexSHA256(payload), time.Now())
	resp, err := s.httpClient.Do(req)
	ObserveImageHTTPExchange(ctx, req, resp, err, "video_result_store")
	if err != nil {
		cancel()
		return nil, errors.New("video result store request failed")
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		defer cancel()
		defer func() { _ = resp.Body.Close() }()
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4<<10))
		return nil, fmt.Errorf("video result store returned HTTP %d", resp.StatusCode)
	}
	resp.Body = &videoResultResponseBody{ReadCloser: resp.Body, cancel: cancel}
	return resp, nil
}

// 取消随响应正文关闭，避免返回响应头后提前中断完成证据的读取。
type videoResultResponseBody struct {
	io.ReadCloser
	cancel context.CancelFunc
}

func (b *videoResultResponseBody) Close() error {
	defer b.cancel()
	return b.ReadCloser.Close()
}

func (s *s3ArtifactStore) videoResultRequestForBody(ctx context.Context, method, rawURL, query, mimeType string, payload []byte) ([]byte, error) {
	resp, err := s.videoResultRequest(ctx, method, rawURL, query, mimeType, payload)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(io.LimitReader(resp.Body, (64<<10)+1))
	if err != nil || len(body) > 64<<10 {
		return nil, errors.New("video result store response could not be read")
	}
	return body, nil
}

func (s *s3ArtifactStore) presignVideoResultObjectURL(objectKey, method string, ttl time.Duration) (string, int64, error) {
	issuedAt := time.Now()
	url, err := SigV4PresignURL(method, s.objectURL(objectKey), s.credentials, s.region(), ttl, issuedAt)
	return url, issuedAt.Add(ttl).Unix(), err
}

func (s *s3ArtifactStore) videoResultLocation() model.VideoResultStorageLocation {
	return model.VideoResultStorageLocation{
		Backend:  "s3",
		Endpoint: strings.TrimRight(s.config.S3Endpoint, "/"),
		Bucket:   s.config.S3Bucket,
		Prefix:   strings.Trim(s.config.S3Prefix, "/"),
	}
}

// readVideoResultPart 读取至多 size 字节；eof 表示上游内容已经读完。
func readVideoResultPart(reader io.Reader, size int64) ([]byte, bool, error) {
	buffer := make([]byte, size)
	total := 0
	for total < len(buffer) {
		n, err := reader.Read(buffer[total:])
		total += n
		if err == io.EOF {
			return buffer[:total], true, nil
		}
		if err != nil {
			return nil, false, err
		}
	}
	return buffer[:total], false, nil
}

// s3QueryEscape 以 RFC 3986 编码 query 值（+ 会被部分网关按空格解释）。
func s3QueryEscape(value string) string {
	return strings.ReplaceAll(url.QueryEscape(value), "+", "%20")
}
