package model

import (
	"errors"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
)

const (
	RealPersonAuthorizationAwaitingConsent      = "awaiting_consent"
	RealPersonAuthorizationConsentRejected      = "consent_rejected"
	RealPersonAuthorizationAwaitingVerification = "awaiting_verification"
	RealPersonAuthorizationVerifying            = "verifying"
	RealPersonAuthorizationAuthorized           = "authorized"
	RealPersonAuthorizationFailed               = "failed"
	RealPersonAuthorizationExpired              = "expired"
	RealPersonAuthorizationRevoked              = "revoked"
	RealPersonAuthorizationDeleting             = "deleting"
	RealPersonAuthorizationDeleted              = "deleted"
)

type ConsentPolicy struct {
	ID            int64  `json:"id" gorm:"primaryKey"`
	Version       string `json:"version" gorm:"type:varchar(64);uniqueIndex:idx_consent_policy_version_locale"`
	Locale        string `json:"locale" gorm:"type:varchar(16);uniqueIndex:idx_consent_policy_version_locale"`
	Title         string `json:"title" gorm:"type:varchar(191)"`
	Content       string `json:"content" gorm:"type:text"`
	ContentSHA256 string `json:"content_sha256" gorm:"type:varchar(64)"`
	Status        string `json:"status" gorm:"type:varchar(32);index"`
	EffectiveAt   int64  `json:"effective_at" gorm:"bigint;index"`
	CreatedAt     int64  `json:"created_at" gorm:"bigint"`
}

type RealPersonAuthorization struct {
	ID                    int64   `json:"-" gorm:"primaryKey"`
	PublicID              string  `json:"id" gorm:"type:varchar(64);uniqueIndex"`
	UserID                int     `json:"-" gorm:"index"`
	CreatedByTokenID      int     `json:"-" gorm:"index"`
	RequestedModel        string  `json:"model" gorm:"type:varchar(191)"`
	ChannelID             int     `json:"-" gorm:"index"`
	CredentialFingerprint string  `json:"-" gorm:"type:varchar(64)"`
	UpstreamProfile       string  `json:"-" gorm:"type:varchar(32)"`
	ProviderProject       string  `json:"-" gorm:"type:varchar(128)"`
	Region                string  `json:"-" gorm:"type:varchar(64)"`
	PolicyID              int64   `json:"-" gorm:"index"`
	PolicyHash            string  `json:"-" gorm:"type:varchar(64)"`
	Locale                string  `json:"locale" gorm:"type:varchar(16)"`
	Status                string  `json:"status" gorm:"type:varchar(32);index"`
	ErrorCode             string  `json:"error_code,omitempty" gorm:"type:varchar(64)"`
	AdultConfirmed        bool    `json:"-"`
	ConsentedAt           int64   `json:"-" gorm:"bigint"`
	ConsentEvidenceHMAC   string  `json:"-" gorm:"type:varchar(64)"`
	UserAgent             string  `json:"-" gorm:"type:varchar(512)"`
	ConsentTokenHash      string  `json:"-" gorm:"type:varchar(64);uniqueIndex"`
	ReceiptTokenHash      *string `json:"-" gorm:"type:varchar(64);uniqueIndex"`
	ConsentTokenExpiresAt int64   `json:"-" gorm:"bigint;index"`
	RevokedAt             int64   `json:"-" gorm:"bigint"`
	CreatedAt             int64   `json:"created_at" gorm:"bigint;index"`
	UpdatedAt             int64   `json:"updated_at" gorm:"bigint;index"`
}

type RealPersonVerificationSession struct {
	ID                           int64   `json:"-" gorm:"primaryKey"`
	AuthorizationID              int64   `json:"-" gorm:"index"`
	UpstreamSessionID            string  `json:"-" gorm:"type:varchar(191);index"`
	VerificationHandleCiphertext string  `json:"-" gorm:"type:text"`
	VerificationTokenHash        *string `json:"-" gorm:"type:varchar(64);uniqueIndex"`
	H5URLCiphertext              string  `json:"-" gorm:"type:text"`
	UpstreamGroupID              string  `json:"-" gorm:"type:varchar(191)"`
	Status                       string  `json:"status" gorm:"type:varchar(32);index"`
	ExpiresAt                    int64   `json:"expires_at" gorm:"bigint;index"`
	CallbackReceivedAt           int64   `json:"-" gorm:"bigint"`
	LastPolledAt                 int64   `json:"-" gorm:"bigint"`
	AttemptCount                 int     `json:"-"`
	NextRetryAt                  int64   `json:"-" gorm:"bigint;index"`
	ErrorCode                    string  `json:"error_code,omitempty" gorm:"type:varchar(64)"`
	ErrorMessage                 string  `json:"error,omitempty" gorm:"type:text"`
	CreatedAt                    int64   `json:"created_at" gorm:"bigint"`
	UpdatedAt                    int64   `json:"updated_at" gorm:"bigint"`
}

func (a *RealPersonAuthorization) BeforeCreate(_ *gorm.DB) error {
	if a.PublicID == "" {
		id, err := generateAssetPublicID("rpa_")
		if err != nil {
			return err
		}
		a.PublicID = id
	}
	now := common.GetTimestamp()
	if a.CreatedAt == 0 {
		a.CreatedAt = now
	}
	a.UpdatedAt = now
	return nil
}

func (p *ConsentPolicy) BeforeCreate(_ *gorm.DB) error {
	if p.CreatedAt == 0 {
		p.CreatedAt = common.GetTimestamp()
	}
	return nil
}

func (s *RealPersonVerificationSession) BeforeCreate(_ *gorm.DB) error {
	now := common.GetTimestamp()
	if s.CreatedAt == 0 {
		s.CreatedAt = now
	}
	s.UpdatedAt = now
	return nil
}

func GetRealPersonAuthorization(userID int, publicID string) (*RealPersonAuthorization, error) {
	var authorization RealPersonAuthorization
	err := DB.Where("user_id = ? AND public_id = ?", userID, publicID).First(&authorization).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	return &authorization, err
}

func GetRealPersonAuthorizationByConsentHash(hash string) (*RealPersonAuthorization, error) {
	var authorization RealPersonAuthorization
	err := DB.Where("consent_token_hash = ?", hash).First(&authorization).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	return &authorization, err
}

func GetRealPersonAuthorizationByReceiptHash(hash string) (*RealPersonAuthorization, error) {
	var authorization RealPersonAuthorization
	err := DB.Where("receipt_token_hash = ?", hash).First(&authorization).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	return &authorization, err
}

// LockRealPersonAuthorization serializes state transitions that can create or
// remove resources owned by the same real-person authorization.
func LockRealPersonAuthorization(tx *gorm.DB, id int64) (*RealPersonAuthorization, error) {
	var authorization RealPersonAuthorization
	if err := lockForUpdate(tx).First(&authorization, "id = ?", id).Error; err != nil {
		return nil, err
	}
	return &authorization, nil
}

func GetActiveConsentPolicy(locale string) (*ConsentPolicy, error) {
	var policy ConsentPolicy
	err := DB.Where("status = ? AND locale = ? AND effective_at <= ?", "active", locale, common.GetTimestamp()).Order("effective_at desc").First(&policy).Error
	if errors.Is(err, gorm.ErrRecordNotFound) && locale != "en" {
		err = DB.Where("status = ? AND locale = ? AND effective_at <= ?", "active", "en", common.GetTimestamp()).Order("effective_at desc").First(&policy).Error
	}
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	return &policy, err
}

func ListConsentPolicies() ([]ConsentPolicy, error) {
	var policies []ConsentPolicy
	err := DB.Order("id desc").Find(&policies).Error
	return policies, err
}

func ActivateConsentPolicy(id int64) error {
	return DB.Transaction(func(tx *gorm.DB) error {
		var policy ConsentPolicy
		if err := lockForUpdate(tx).First(&policy, "id = ?", id).Error; err != nil {
			return err
		}
		if err := tx.Model(&ConsentPolicy{}).Where("locale = ? AND status = ?", policy.Locale, "active").Update("status", "retired").Error; err != nil {
			return err
		}
		return tx.Model(&policy).Updates(map[string]any{"status": "active"}).Error
	})
}
