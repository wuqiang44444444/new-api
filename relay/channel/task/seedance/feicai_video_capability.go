package seedance

import (
	"fmt"

	taskdto "github.com/QuantumNous/new-api/dto"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/gin-gonic/gin"
)

func buildFeicaiVideoCreateRequest(
	c *gin.Context,
	info *relaycommon.RelayInfo,
	profile taskdto.VideoUpstreamProfile,
) ([]byte, bool, error) {
	if profile != taskdto.VideoUpstreamProfileThirdPartyFeicaiVideos {
		return nil, false, nil
	}
	// feicai_videos_v1 的南向转换已迁移到 seedance-link 扩展插件；
	// 旧 Go 转换不再服务新请求，缺少插件时失败关闭，不回退。
	return nil, true, fmt.Errorf("the selected video adapter requires the Seedance extension")
}
