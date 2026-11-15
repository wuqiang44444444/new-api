package service

import (
	"context"
	"crypto/tls"
	"errors"
	"io"
	"net"
	"syscall"

	"github.com/QuantumNous/new-api/relaykit/types"
)

// Values come from gateway classifications. Provider messages and codes are
// never stored as alert diagnostics.
type channelAlertErrorReason string

const (
	channelAlertTimeout        channelAlertErrorReason = "timeout"
	channelAlertDNSFailure     channelAlertErrorReason = "dns_error"
	channelAlertTLSFailure     channelAlertErrorReason = "tls_error"
	channelAlertConnRefused    channelAlertErrorReason = "connection_refused"
	channelAlertConnReset      channelAlertErrorReason = "connection_reset"
	channelAlertStreamRead     channelAlertErrorReason = "stream_read_error"
	channelAlertTransport      channelAlertErrorReason = "transport_error"
	channelAlertOverload       channelAlertErrorReason = "upstream_overloaded"
	channelAlertUpstreamLimit  channelAlertErrorReason = "upstream_deadline_exceeded"
	channelAlertParamOverride  channelAlertErrorReason = "parameter_override_invalid"
	channelAlertHeaderOverride channelAlertErrorReason = "header_override_invalid"
	channelAlertModelMapping   channelAlertErrorReason = "model_mapping_invalid"
	channelAlertMissingKey     channelAlertErrorReason = "no_available_key"
	channelAlertInvalidKey     channelAlertErrorReason = "invalid_key"
)

func channelAlertTransportReason(err error) channelAlertErrorReason {
	var dnsErr *net.DNSError
	var tlsErr *tls.CertificateVerificationError
	var netErr net.Error
	switch {
	case errors.As(err, &dnsErr):
		return channelAlertDNSFailure
	case errors.As(err, &tlsErr):
		return channelAlertTLSFailure
	case errors.Is(err, context.DeadlineExceeded), errors.As(err, &netErr) && netErr.Timeout():
		return channelAlertTimeout
	case errors.Is(err, syscall.ECONNREFUSED):
		return channelAlertConnRefused
	case errors.Is(err, syscall.ECONNRESET), errors.Is(err, io.ErrUnexpectedEOF):
		return channelAlertConnReset
	default:
		return channelAlertTransport
	}
}

func channelAlertConfigReason(code types.ErrorCode) channelAlertErrorReason {
	switch code {
	case types.ErrorCodeChannelParamOverrideInvalid:
		return channelAlertParamOverride
	case types.ErrorCodeChannelHeaderOverrideInvalid:
		return channelAlertHeaderOverride
	case types.ErrorCodeChannelModelMappedError:
		return channelAlertModelMapping
	case types.ErrorCodeChannelInvalidKey:
		return channelAlertInvalidKey
	default:
		return ""
	}
}

func channelAlertServiceReason(code string) channelAlertErrorReason {
	switch code {
	case "overloaded_error", "server_overloaded":
		return channelAlertOverload
	case "DEADLINE_EXCEEDED":
		return channelAlertUpstreamLimit
	default:
		return ""
	}
}

func (reason channelAlertErrorReason) mailLabel() string {
	switch reason {
	case channelAlertTimeout:
		return "连接或读取超时 / Connection or read timed out"
	case channelAlertDNSFailure:
		return "DNS 解析失败 / DNS resolution failed"
	case channelAlertTLSFailure:
		return "TLS 证书验证失败 / TLS certificate verification failed"
	case channelAlertConnRefused:
		return "连接被拒绝 / Connection refused"
	case channelAlertConnReset:
		return "连接意外中断 / Connection closed unexpectedly"
	case channelAlertStreamRead:
		return "读取上游流失败 / Failed to read upstream stream"
	case channelAlertTransport:
		return "未收到上游 HTTP 响应 / No upstream HTTP response"
	case channelAlertOverload:
		return "上游报告服务过载 / Upstream reported service overload"
	case channelAlertUpstreamLimit:
		return "上游报告处理超时 / Upstream reported a processing timeout"
	case channelAlertParamOverride:
		return "渠道参数覆盖无效 / Invalid channel parameter override"
	case channelAlertHeaderOverride:
		return "渠道请求头覆盖无效 / Invalid channel header override"
	case channelAlertModelMapping:
		return "渠道模型映射无效 / Invalid channel model mapping"
	case channelAlertMissingKey:
		return "渠道没有可用凭据 / No available channel credential"
	case channelAlertInvalidKey:
		return "渠道凭据配置无效 / Invalid channel credential configuration"
	default:
		return ""
	}
}
