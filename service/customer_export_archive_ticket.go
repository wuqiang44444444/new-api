package service

import (
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

const customerExportArchiveTokenUse = "customer_export_archive"

func IssueCustomerExportArchiveTicket(identity AuthIdentity, jobID string) (string, error) {
	now := time.Now()
	claims := authClaims{
		TokenUse:        customerExportArchiveTokenUse,
		SessionID:       identity.SessionID,
		UserAuthVersion: identity.UserAuthVersion,
		SessionVersion:  identity.SessionVersion,
		ContextHash:     jobID,
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    authTokenIssuer,
			Audience:  jwt.ClaimStrings{authTokenAudience},
			Subject:   strconv.Itoa(identity.UserID),
			ExpiresAt: jwt.NewNumericDate(now.Add(time.Minute)),
			NotBefore: jwt.NewNumericDate(now),
			IssuedAt:  jwt.NewNumericDate(now),
			ID:        uuid.NewString(),
		},
	}
	return jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(authSigningKey(customerExportArchiveTokenUse))
}

func ValidateCustomerExportArchiveTicket(raw, jobID string) (int, error) {
	claims, err := parseAuthClaims(raw, customerExportArchiveTokenUse, authSigningKey(customerExportArchiveTokenUse))
	if err != nil || claims.ContextHash != jobID {
		return 0, ErrAuthTokenInvalid
	}
	userID, err := strconv.Atoi(claims.Subject)
	if err != nil || userID <= 0 {
		return 0, ErrAuthTokenInvalid
	}
	_, user, err := ValidateLoginSession(AuthIdentity{
		UserID:          userID,
		SessionID:       claims.SessionID,
		UserAuthVersion: claims.UserAuthVersion,
		SessionVersion:  claims.SessionVersion,
	})
	if err != nil {
		return 0, err
	}
	if user.Role < common.RoleCommonUser || !common.IsValidateRole(user.Role) || strings.TrimSpace(user.Username) == "" {
		return 0, ErrAuthTokenInvalid
	}
	return userID, nil
}
