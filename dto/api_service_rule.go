package dto

type CreateAPIServiceRuleRequest struct {
	Version     string `json:"version"`
	Title       string `json:"title"`
	Content     string `json:"content"`
	EffectiveAt int64  `json:"effective_at,omitempty"`
}

type AcceptAPIServiceRuleRequest struct {
	Version                string `json:"version"`
	ContentSHA256          string `json:"content_sha256"`
	ComplianceOwner        string `json:"compliance_owner"`
	ConsentRecordSystemRef string `json:"consent_record_system_ref"`
}

type APIServiceRuleAcceptanceResponse struct {
	Accepted   bool   `json:"accepted"`
	AppID      int    `json:"app_id"`
	Version    string `json:"version"`
	ContentSHA string `json:"content_sha256"`
	AcceptedAt int64  `json:"accepted_at,omitempty"`
}

type AdminAPIServiceRuleAcceptanceResponse struct {
	AppID                  int    `json:"app_id"`
	UserID                 int    `json:"user_id"`
	RuleVersion            string `json:"rule_version"`
	ContentSHA256          string `json:"content_sha256"`
	AcceptedAt             int64  `json:"accepted_at"`
	AcceptedBy             string `json:"accepted_by"`
	AcceptanceMethod       string `json:"acceptance_method"`
	ComplianceOwner        string `json:"compliance_owner"`
	ConsentRecordSystemRef string `json:"consent_record_system_ref"`
}
