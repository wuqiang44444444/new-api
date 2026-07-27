package controller

import (
	"crypto/subtle"
	"errors"
	"html/template"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/asset_setting"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

const consentCSRFCookie = "newapi_rpa_csrf"
const consentReceiptCookie = "newapi_rpa_receipt"

var consentPageTemplate = template.Must(template.New("consent").Parse(`<!doctype html>
<html lang="{{.Lang}}"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1">
<title>{{.Title}}</title><style>body{font-family:system-ui,sans-serif;max-width:760px;margin:40px auto;padding:0 20px;line-height:1.65;color:#172033}main{border:1px solid #dfe3ea;border-radius:14px;padding:28px}pre{white-space:pre-wrap;font:inherit;background:#f7f8fa;padding:18px;border-radius:10px}label{display:block;margin:16px 0}button{padding:10px 18px;margin-right:10px}small{color:#5f6b7a}</style></head>
<body><main><h1>{{.Title}}</h1><pre>{{.Content}}</pre><form method="post" action="/api/real-person-consents/{{.Token}}/accept">
<input type="hidden" name="csrf_token" value="{{.CSRF}}"><label><input type="checkbox" name="consent" value="yes"> {{.ConsentLabel}}</label>
<label><input type="checkbox" name="adult" value="yes"> {{.AdultLabel}}</label><button type="submit">{{.AcceptLabel}}</button></form>
<form method="post" action="/api/real-person-consents/{{.Token}}/reject"><input type="hidden" name="csrf_token" value="{{.CSRF}}"><button type="submit">{{.RejectLabel}}</button></form>
<small>{{.AgeNotice}}</small></main></body></html>`))

var receiptPageTemplate = template.Must(template.New("receipt").Parse(`<!doctype html><html lang="{{.Lang}}"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><title>{{.Title}}</title><style>body{font-family:system-ui,sans-serif;max-width:680px;margin:40px auto;padding:0 20px;line-height:1.6}main{border:1px solid #dfe3ea;border-radius:14px;padding:28px}button{padding:10px 18px}</style></head><body><main><h1>{{.Title}}</h1><p>{{.StatusLabel}}: <strong>{{.Status}}</strong></p>{{if .CanRevoke}}<form method="post" action="/api/real-person-consents/receipt/{{.Token}}/revoke"><input type="hidden" name="csrf_token" value="{{.CSRF}}"><button type="submit">{{.RevokeLabel}}</button></form>{{end}}</main></body></html>`))

func CreateRealPersonAuthorization(c *gin.Context) {
	var req dto.CreateRealPersonAuthorizationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		assetAPIError(c, http.StatusBadRequest, "invalid_request", "invalid authorization request")
		return
	}
	usingGroup := common.GetContextKeyString(c, constant.ContextKeyUsingGroup)
	userGroup := common.GetContextKeyString(c, constant.ContextKeyUserGroup)
	authorization, token, err := service.CreateRealPersonAuthorization(c.Request.Context(), c.GetInt("id"), c.GetInt("token_id"), userGroup, usingGroup, req)
	if err != nil {
		assetAPIError(c, http.StatusServiceUnavailable, "real_person_authorization_unavailable", "real-person authorization is unavailable")
		return
	}
	response := realPersonAuthorizationResponse(authorization)
	response.ConsentURL = asset_setting.Current().PublicBaseURL + "/consent/real-person/" + token
	response.ExpiresAt = authorization.ConsentTokenExpiresAt
	c.JSON(http.StatusCreated, response)
}

func CreateConsentPolicy(c *gin.Context) {
	var req dto.CreateConsentPolicyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		assetAPIError(c, http.StatusBadRequest, "invalid_request", "invalid consent policy")
		return
	}
	policy, err := service.CreateConsentPolicy(c.Request.Context(), req)
	if err != nil {
		if errors.Is(err, service.ErrInvalidConsentPolicy) {
			assetAPIError(c, http.StatusBadRequest, "invalid_consent_policy", "consent policy could not be created")
		} else {
			assetAPIError(c, http.StatusInternalServerError, "database_error", "consent policy could not be created")
		}
		return
	}
	c.JSON(http.StatusCreated, policy)
}

func ListConsentPolicies(c *gin.Context) {
	policies, err := model.ListConsentPolicies()
	if err != nil {
		assetAPIError(c, http.StatusInternalServerError, "database_error", "failed to list consent policies")
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": policies})
}

func ActivateConsentPolicy(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("policy_id"), 10, 64)
	if err != nil || id <= 0 {
		assetAPIError(c, http.StatusBadRequest, "invalid_request", "invalid consent policy id")
		return
	}
	if err := model.ActivateConsentPolicy(id); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			assetAPIError(c, http.StatusNotFound, "consent_policy_not_found", "consent policy not found")
		} else {
			assetAPIError(c, http.StatusInternalServerError, "database_error", "failed to activate consent policy")
		}
		return
	}
	c.Status(http.StatusNoContent)
}

func GetRealPersonAuthorization(c *gin.Context) {
	authorization, err := model.GetRealPersonAuthorization(c.GetInt("id"), c.Param("authorization_id"))
	if err != nil {
		assetAPIError(c, http.StatusInternalServerError, "database_error", "failed to load authorization")
		return
	}
	if authorization == nil {
		assetAPIError(c, http.StatusNotFound, "authorization_not_found", "authorization not found")
		return
	}
	if err := service.RefreshRealPersonVerification(c.Request.Context(), authorization); err != nil {
		assetAPIError(c, http.StatusBadGateway, "verification_status_unavailable", "verification status is temporarily unavailable")
		return
	}
	c.JSON(http.StatusOK, realPersonAuthorizationResponse(authorization))
}

func RevokeRealPersonAuthorization(c *gin.Context) {
	authorization, err := model.GetRealPersonAuthorization(c.GetInt("id"), c.Param("authorization_id"))
	if err != nil {
		assetAPIError(c, http.StatusInternalServerError, "database_error", "failed to load authorization")
		return
	}
	if authorization == nil {
		assetAPIError(c, http.StatusNotFound, "authorization_not_found", "authorization not found")
		return
	}
	if err := service.RevokeRealPersonAuthorization(c.Request.Context(), authorization); err != nil {
		assetAPIError(c, http.StatusInternalServerError, "authorization_revoke_failed", "failed to revoke authorization")
		return
	}
	c.JSON(http.StatusOK, realPersonAuthorizationResponse(authorization))
}

func RetryRealPersonAuthorization(c *gin.Context) {
	authorization, err := model.GetRealPersonAuthorization(c.GetInt("id"), c.Param("authorization_id"))
	if err != nil {
		assetAPIError(c, http.StatusInternalServerError, "database_error", "failed to load authorization")
		return
	}
	if authorization == nil {
		assetAPIError(c, http.StatusNotFound, "authorization_not_found", "authorization not found")
		return
	}
	h5URL, err := service.RetryRealPersonVerification(c.Request.Context(), authorization)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrRealPersonAuthorizationNotRetryable):
			assetAPIError(c, http.StatusConflict, "authorization_not_retryable", "authorization cannot be retried")
		case errors.Is(err, service.ErrRealPersonVerificationUpstream):
			assetAPIError(c, http.StatusBadGateway, "verification_upstream_error", "verification session could not be created")
		default:
			assetAPIError(c, http.StatusInternalServerError, "database_error", "verification session could not be created")
		}
		return
	}
	c.JSON(http.StatusOK, gin.H{"authorization": realPersonAuthorizationResponse(authorization), "verification_url": h5URL})
}

func ShowRealPersonConsent(c *gin.Context) {
	authorization, policy, err := service.GetConsentAuthorization(c.Param("token"))
	if err != nil || authorization == nil || policy == nil || authorization.ConsentTokenExpiresAt < time.Now().Unix() {
		c.String(http.StatusGone, "This consent link is invalid or expired.")
		return
	}
	if authorization.Status != model.RealPersonAuthorizationAwaitingConsent {
		c.String(http.StatusGone, "This consent request has already been processed.")
		return
	}
	csrf, err := newConsentCSRF(c)
	if err != nil {
		c.Status(http.StatusInternalServerError)
		return
	}
	zh := strings.HasPrefix(strings.ToLower(authorization.Locale), "zh")
	data := gin.H{"Lang": authorization.Locale, "Title": policy.Title, "Content": policy.Content, "Token": c.Param("token"), "CSRF": csrf}
	if zh {
		data["ConsentLabel"] = "我已阅读并单独同意上述人脸信息处理事项"
		data["AdultLabel"] = "我确认本人已满 18 周岁"
		data["AcceptLabel"] = "同意并继续认证"
		data["RejectLabel"] = "拒绝"
		data["AgeNotice"] = "年龄为本人声明，平台未进行身份年龄核验。拒绝不会影响普通视频 API。"
	} else {
		data["ConsentLabel"] = "I have read and separately consent to the facial-information processing described above"
		data["AdultLabel"] = "I confirm that I am at least 18 years old"
		data["AcceptLabel"] = "Consent and continue"
		data["RejectLabel"] = "Reject"
		data["AgeNotice"] = "Age is self-declared; the platform does not verify age identity. Rejection does not affect ordinary video APIs."
	}
	consentHTMLHeaders(c)
	c.Status(http.StatusOK)
	_ = consentPageTemplate.Execute(c.Writer, data)
}

func AcceptRealPersonConsent(c *gin.Context) {
	if !validConsentPost(c) {
		c.String(http.StatusForbidden, "invalid consent request")
		return
	}
	if c.PostForm("consent") != "yes" || c.PostForm("adult") != "yes" {
		c.String(http.StatusBadRequest, "both separate consent and adult confirmation are required")
		return
	}
	authorization, receiptToken, h5URL, err := service.AcceptRealPersonConsent(c.Request.Context(), c.Param("token"), c.Request.UserAgent(), c.ClientIP())
	if receiptToken != "" {
		http.SetCookie(c.Writer, &http.Cookie{Name: consentReceiptCookie, Value: receiptToken, Path: "/consent/real-person", MaxAge: 10 * 365 * 24 * 60 * 60, Secure: true, HttpOnly: true, SameSite: http.SameSiteStrictMode})
	}
	if err != nil {
		if receiptToken != "" {
			c.Redirect(http.StatusSeeOther, "/consent/real-person/receipt/"+url.PathEscape(receiptToken))
			return
		}
		c.String(http.StatusConflict, "unable to continue verification")
		return
	}
	if h5URL != "" {
		c.Redirect(http.StatusSeeOther, h5URL)
		return
	}
	if authorization != nil {
		c.String(http.StatusOK, "Consent has already been processed. Return to the original receipt page to view status.")
		return
	}
	c.Status(http.StatusConflict)
}

func RejectRealPersonConsent(c *gin.Context) {
	if !validConsentPost(c) {
		c.String(http.StatusForbidden, "invalid consent request")
		return
	}
	if err := service.RejectRealPersonConsent(c.Request.Context(), c.Param("token")); err != nil {
		c.String(http.StatusConflict, "unable to reject consent")
		return
	}
	c.String(http.StatusOK, "Consent was rejected. Ordinary video API access is unaffected.")
}

func CompleteRealPersonConsent(c *gin.Context) {
	publicID := strings.TrimSpace(c.Query("authorization_id"))
	var authorization model.RealPersonAuthorization
	if publicID == "" || model.DB.Where("public_id = ?", publicID).First(&authorization).Error != nil {
		c.String(http.StatusNotFound, "authorization not found")
		return
	}
	_ = service.RefreshRealPersonVerification(c.Request.Context(), &authorization)
	if cookie, err := c.Cookie(consentReceiptCookie); err == nil {
		if receiptAuthorization, _ := service.GetReceiptAuthorization(cookie); receiptAuthorization != nil && receiptAuthorization.ID == authorization.ID {
			c.Redirect(http.StatusSeeOther, "/consent/real-person/receipt/"+url.PathEscape(cookie))
			return
		}
	}
	c.String(http.StatusOK, "Verification status: %s. The API client can query the authorization status.", authorization.Status)
}

func OpenRealPersonVerification(c *gin.Context) {
	h5URL, err := service.OpenRealPersonVerification(c.Request.Context(), c.Param("token"))
	if err != nil {
		c.String(http.StatusGone, "This verification link is invalid, expired, or already used.")
		return
	}
	c.Header("Cache-Control", "private, no-store")
	c.Redirect(http.StatusSeeOther, h5URL)
}

func ShowRealPersonReceipt(c *gin.Context) {
	authorization, err := service.GetReceiptAuthorization(c.Param("receipt_token"))
	if err != nil || authorization == nil {
		c.String(http.StatusNotFound, "receipt not found")
		return
	}
	_ = service.RefreshRealPersonVerification(c.Request.Context(), authorization)
	csrf, err := newConsentCSRF(c)
	if err != nil {
		c.Status(http.StatusInternalServerError)
		return
	}
	zh := strings.HasPrefix(strings.ToLower(authorization.Locale), "zh")
	data := gin.H{"Lang": authorization.Locale, "Token": c.Param("receipt_token"), "CSRF": csrf, "Status": authorization.Status, "CanRevoke": authorization.Status != model.RealPersonAuthorizationRevoked && authorization.Status != model.RealPersonAuthorizationDeleted}
	if zh {
		data["Title"], data["StatusLabel"], data["RevokeLabel"] = "真人素材授权回执", "当前状态", "撤回授权"
	} else {
		data["Title"], data["StatusLabel"], data["RevokeLabel"] = "Real-person authorization receipt", "Current status", "Revoke authorization"
	}
	consentHTMLHeaders(c)
	c.Status(http.StatusOK)
	_ = receiptPageTemplate.Execute(c.Writer, data)
}

func RevokeRealPersonReceipt(c *gin.Context) {
	if !validConsentPost(c) {
		c.String(http.StatusForbidden, "invalid consent request")
		return
	}
	authorization, err := service.GetReceiptAuthorization(c.Param("receipt_token"))
	if err != nil || authorization == nil {
		c.String(http.StatusNotFound, "receipt not found")
		return
	}
	if err := service.RevokeRealPersonAuthorization(c.Request.Context(), authorization); err != nil {
		c.String(http.StatusInternalServerError, "unable to revoke authorization")
		return
	}
	c.String(http.StatusOK, "Authorization revoked. Associated assets are being deleted.")
}

func realPersonAuthorizationResponse(authorization *model.RealPersonAuthorization) dto.RealPersonAuthorizationResponse {
	return dto.RealPersonAuthorizationResponse{ID: authorization.PublicID, Status: authorization.Status, ErrorCode: authorization.ErrorCode, CreatedAt: authorization.CreatedAt, UpdatedAt: authorization.UpdatedAt}
}

func newConsentCSRF(c *gin.Context) (string, error) {
	token, err := common.GenerateRandomCharsKey(32)
	if err != nil {
		return "", err
	}
	http.SetCookie(c.Writer, &http.Cookie{Name: consentCSRFCookie, Value: token, Path: "/", MaxAge: 1800, Secure: true, HttpOnly: true, SameSite: http.SameSiteStrictMode})
	return token, nil
}

func validConsentPost(c *gin.Context) bool {
	cookie, err := c.Cookie(consentCSRFCookie)
	if err != nil || subtle.ConstantTimeCompare([]byte(cookie), []byte(c.PostForm("csrf_token"))) != 1 {
		return false
	}
	base, err := url.Parse(asset_setting.Current().PublicBaseURL)
	if err != nil || base.Host == "" {
		return false
	}
	source := c.GetHeader("Origin")
	if source == "" {
		source = c.GetHeader("Referer")
	}
	parsed, err := url.Parse(source)
	return err == nil && strings.EqualFold(parsed.Scheme, base.Scheme) && strings.EqualFold(parsed.Host, base.Host)
}

func consentHTMLHeaders(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	c.Header("Content-Security-Policy", "default-src 'none'; style-src 'unsafe-inline'; form-action 'self'; base-uri 'none'; frame-ancestors 'none'")
	c.Header("Referrer-Policy", "no-referrer")
	c.Header("X-Content-Type-Options", "nosniff")
	c.Header("Content-Type", "text/html; charset=utf-8")
}
