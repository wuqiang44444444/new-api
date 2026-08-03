package dto

import "net/http"

type KlingVideoErrorResponse struct {
	Code      int    `json:"code"`
	Message   string `json:"message"`
	RequestID string `json:"request_id"`
	Data      any    `json:"data"`
}

type JimengVideoErrorResponse struct {
	Code      int    `json:"code"`
	Data      any    `json:"data"`
	Message   string `json:"message"`
	RequestID string `json:"request_id"`
	Status    int    `json:"status"`
}

func KlingVideoErrorCode(status int) int {
	switch status {
	case http.StatusUnauthorized:
		return 1000
	case http.StatusPaymentRequired:
		return 1101
	case http.StatusForbidden:
		return 1103
	case http.StatusNotFound:
		return 1203
	case http.StatusTooManyRequests:
		return 1303
	case http.StatusServiceUnavailable:
		return 5001
	case http.StatusGatewayTimeout:
		return 5002
	default:
		if status >= http.StatusInternalServerError {
			return 5000
		}
		return 1200
	}
}

func JimengVideoErrorCode(status int) int {
	switch status {
	case http.StatusUnauthorized, http.StatusForbidden:
		return 50400
	case http.StatusTooManyRequests:
		return 50430
	default:
		if status >= http.StatusInternalServerError {
			return 50500
		}
		return 50200
	}
}
