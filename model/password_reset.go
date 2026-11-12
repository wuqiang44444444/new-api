package model

import (
	"errors"
	"time"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
)

const AuthFlowPurposePasswordReset = "password_reset"
const PasswordResetLifetime = 15 * time.Minute

type passwordResetState struct {
	Email       string `json:"email"`
	AuthVersion int64  `json:"auth_version"`
}

var ErrPasswordResetRecent = errors.New("password reset already requested")

// CreatePasswordReset limits requests for the same account across nodes. A
// second request cannot invalidate a link which was already sent.
func CreatePasswordReset(user *User) (string, error) {
	var token string
	err := DB.Transaction(func(tx *gorm.DB) error {
		var current User
		if err := lockForUpdate(tx).Select("id", "email", "status", "auth_version").First(&current, user.Id).Error; err != nil {
			return err
		}
		if current.Status != common.UserStatusEnabled || NormalizeEmail(current.Email) != NormalizeEmail(user.Email) {
			return ErrAuthFlowInvalid
		}
		now := time.Now()
		var recent AuthFlow
		err := tx.Select("id").Where("user_id = ? AND purpose = ? AND created_at >= ?",
			current.Id, AuthFlowPurposePasswordReset, now.Add(-time.Minute)).Take(&recent).Error
		if err == nil {
			return ErrPasswordResetRecent
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		payload, err := common.Marshal(passwordResetState{NormalizeEmail(current.Email), current.AuthVersion})
		if err != nil {
			return err
		}
		token, _, err = createAuthFlowWithTx(tx, AuthFlowCreate{
			Purpose: AuthFlowPurposePasswordReset, UserId: current.Id,
			Payload: string(payload), ExpiresAt: now.Add(PasswordResetLifetime),
		})
		return err
	})
	return token, err
}

// DiscardUnsentPasswordReset releases the cooldown for a link that was never submitted.
func DiscardUnsentPasswordReset(token string) error {
	return applyAuthFlowMatch(DB, token, AuthFlowMatch{Purpose: AuthFlowPurposePasswordReset}).
		Where("consumed_at IS NULL").Delete(&AuthFlow{}).Error
}

// CompletePasswordReset binds reset to the mailbox and credential version that
// authorized it. Claim, password update and version bump commit together.
func CompletePasswordReset(token, password string) (int, error) {
	match := AuthFlowMatch{Purpose: AuthFlowPurposePasswordReset}
	// Reject invalid public requests before allocating the password hash workload.
	if _, err := GetAuthFlow(token, match); err != nil {
		return 0, err
	}
	hash, err := common.HashAccountPassword(password)
	if err != nil {
		return 0, err
	}
	flow, err := ConsumeAuthFlowWithAction(token, match, func(tx *gorm.DB, flow *AuthFlow) error {
		var state passwordResetState
		if err := common.UnmarshalJsonStr(flow.Payload, &state); err != nil {
			return ErrAuthFlowInvalid
		}
		var user User
		if err := lockForUpdate(tx).Select("id", "email", "status", "auth_version").First(&user, flow.UserId).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrAuthFlowInvalid
			}
			return err
		}
		if user.Status != common.UserStatusEnabled || NormalizeEmail(user.Email) != state.Email || user.AuthVersion != state.AuthVersion {
			return ErrAuthFlowInvalid
		}
		if _, err := IncrementUserAuthVersionWithTx(tx, user.Id); err != nil {
			return err
		}
		return tx.Model(&User{}).Where("id = ?", user.Id).Update("password", hash).Error
	})
	if err != nil {
		return 0, err
	}
	// The committed auth version already invalidates old sessions.
	return flow.UserId, nil
}
