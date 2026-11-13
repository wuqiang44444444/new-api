package seedance

import (
	"errors"
	"strings"

	"github.com/QuantumNous/new-api/model"
)

// rejectUnresolvedHostedMedia 保证 FunCloud V3 带托管前缀的引用都不会在未解析为内部
// URL 的情况下发送给上游；解析缺口一律失败关闭。
func rejectUnresolvedHostedMedia(content []ContentItem) error {
	for _, item := range content {
		for _, media := range []*MediaURL{item.ImageURL, item.VideoURL, item.AudioURL} {
			if media == nil {
				continue
			}
			if strings.HasPrefix(strings.TrimSpace(media.URL), "asset://"+model.FunCloudHostedAssetIDPrefix) {
				return errors.New("hosted asset reference was not resolved before request build")
			}
		}
	}
	return nil
}
