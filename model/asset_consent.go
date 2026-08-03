package model

import (
	"errors"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
)

const (
	RealPersonAuthorizationAwaitingVerification = "awaiting_verification"
	RealPersonAuthorizationVerifying            = "verifying"
	RealPersonAuthorizationAuthorized           = "authorized"
	RealPersonAuthorizationFailed               = "failed"
	RealPersonAuthorizationExpired              = "expired"
	RealPersonAuthorizationRevoked              = "revoked"
	RealPersonAuthorizationDeleting             = "deleting"
	RealPersonAuthorizationDeleted              = "deleted"

	RealPersonCleanupPending        = "pending"
	RealPersonCleanupPartial        = "partial"
	RealPersonCleanupComplete       = "complete"
	RealPersonCleanupManualRequired = "manual_required"
)

type RealPersonAuthorization struct {
	ID                    int64  `json:"-" gorm:"primaryKey"`
	PublicID              string `json:"id" gorm:"type:varchar(64);uniqueIndex"`
	UserID                int    `json:"-" gorm:"index"`
	CreatedByTokenID      int    `json:"-" gorm:"index"`
	AppID                 int    `json:"-" gorm:"index"`
	EndUserSubjectHash    string `json:"-" gorm:"type:varchar(64);index"`
	RequestedModel        string `json:"model" gorm:"type:varchar(191)"`
	ChannelID             int    `json:"-" gorm:"index"`
	CredentialFingerprint string `json:"-" gorm:"type:varchar(64)"`
	UpstreamProfile       string `json:"-" gorm:"type:varchar(32)"`
	ProviderProject       string `json:"-" gorm:"type:varchar(128)"`
	Region                string `json:"-" gorm:"type:varchar(64)"`
	Status                string `json:"status" gorm:"type:varchar(32);index"`
	ErrorCode             string `json:"error_code,omitempty" gorm:"type:varchar(64)"`
	RevokedAt             int64  `json:"-" gorm:"bigint"`
	CleanupStatus         string `json:"cleanup_status,omitempty" gorm:"type:varchar(32);index"`
	CreatedAt             int64  `json:"created_at" gorm:"bigint;index"`
	UpdatedAt             int64  `json:"updated_at" gorm:"bigint;index"`
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

// LockRealPersonAuthorization serializes state transitions that can create or
// remove resources owned by the same real-person authorization.
func LockRealPersonAuthorization(tx *gorm.DB, id int64) (*RealPersonAuthorization, error) {
	var authorization RealPersonAuthorization
	if err := lockForUpdate(tx).First(&authorization, "id = ?", id).Error; err != nil {
		return nil, err
	}
	return &authorization, nil
}
