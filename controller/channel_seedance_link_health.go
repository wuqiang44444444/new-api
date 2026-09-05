package controller

import (
	"context"
	"fmt"

	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
)

// Seedance Link 渠道没有无副作用的 Chat 探针，也不允许为通过健康检查而创建收费视频或
// 收费素材。自动健康检查因此只执行代码登记协议提供的只读探针：当前是素材协议的
// 分页大小为 1 的列表查询；视频协议没有无副作用视频探针时，探针不可用只记录失败，
// 绝不退回 Chat 测试。
func seedanceLinkChannelHealthResult(ctx context.Context, channel *model.Channel) testResult {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := service.CheckAssetChannelConnectivity(ctx, channel); err != nil {
		// 素材控制面与视频创建可使用不同凭据和端点。无论是 HTTP
		// 拒绝、Provider 业务码还是 transport 错误，该探针都只能记录素材
		// 健康失败，不得据此改变承载视频履约的 Channel 状态。
		return testResult{localErr: fmt.Errorf("seedance link asset probe failed: %s", err)}
	}
	return testResult{}
}

// seedanceLinkChannel 声明 Link 渠道类型常量的本地引用，供通用测试文件的单行接线判断使用。
func seedanceLinkChannel(channel *model.Channel) bool {
	return channel != nil && channel.Type == constant.ChannelTypeSeedanceLink
}
