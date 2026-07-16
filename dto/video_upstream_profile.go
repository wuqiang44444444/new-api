package dto

import "fmt"

// VideoUpstreamProfile 标识 DoubaoVideo 渠道使用的上游视频协议方案。
// 渠道显式选择一个稳定 ID，new-api 据此选择请求/响应协议并完成必要的转换；
// Base URL、API Key、模型映射和（第三方协议下的）创建/查询路径仍全部来自标准渠道字段。
type VideoUpstreamProfile string

const (
	// VideoUpstreamProfileOfficial 使用 new-api 现有原生 Ark 协议。
	// 空字符串与之运行语义一致，保证旧渠道无需迁移。
	VideoUpstreamProfileOfficial VideoUpstreamProfile = "official"
	// VideoUpstreamProfileThirdPartyRelay 适用于提供统一媒体异步任务协议的第三方中转平台：
	// new-api 把 Ark 请求转换为统一媒体任务结构，并归一化四态查询响应。
	VideoUpstreamProfileThirdPartyRelay VideoUpstreamProfile = "third_party_relay"
	// VideoUpstreamProfileThirdPartyReverseProxy 适用于兼容官方 Ark 视频合同的第三方代理或反代服务：
	// 请求保持 Ark 兼容结构透传，响应按已定义兼容差异归一化。
	VideoUpstreamProfileThirdPartyReverseProxy VideoUpstreamProfile = "third_party_reverse_proxy"
)

// IsOfficial 报告该 profile 是否表示官方原生 Ark 协议。
// 空字符串与 "official" 运行语义一致，旧渠道没有该字段时视为官方。
func (p VideoUpstreamProfile) IsOfficial() bool {
	return p == "" || p == VideoUpstreamProfileOfficial
}

// IsThirdParty 报告该 profile 是否为两种第三方协议之一，需要渠道配置创建/查询路径。
func (p VideoUpstreamProfile) IsThirdParty() bool {
	return p == VideoUpstreamProfileThirdPartyRelay || p == VideoUpstreamProfileThirdPartyReverseProxy
}

// IsValid 报告该 profile 是否命中已知集合（含 official）。
// 受控的已知 ID 集中在此 switch，新增上游时必须先实现完整创建与查询协议再在此登记。
func (p VideoUpstreamProfile) IsValid() bool {
	switch p {
	case VideoUpstreamProfileOfficial, VideoUpstreamProfileThirdPartyRelay, VideoUpstreamProfileThirdPartyReverseProxy:
		return true
	}
	return false
}

// ValidateVideoUpstreamProfile 校验渠道保存的 profile：空或已知 ID 通过，未知 ID 拒绝。
func ValidateVideoUpstreamProfile(p VideoUpstreamProfile) error {
	if p == "" || p.IsValid() {
		return nil
	}
	return fmt.Errorf("unknown video upstream profile: %q", p)
}
