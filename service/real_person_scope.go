package service

import (
	"errors"
	"fmt"
	"strings"

	"github.com/QuantumNous/new-api/common"
)

var ErrInvalidEndUserSubject = errors.New("end_user_subject must be a stable opaque identifier")

const defaultRealPersonVerificationTTLSeconds int64 = 30 * 60

// EndUserSubjectHash validates the Link contract's opaque end-user subject and returns the
// only representation persisted or sent to a Provider. Raw subjects never
// enter database rows or logs.
func EndUserSubjectHash(appID int, raw string) (string, error) {
	if appID <= 0 {
		return "", ErrInvalidEndUserSubject
	}
	raw = strings.TrimSpace(raw)
	if len(raw) < 8 || len(raw) > 128 || strings.Contains(raw, "@") {
		return "", ErrInvalidEndUserSubject
	}
	hasLetter, hasDigit, hasSeparator := false, false, false
	for _, r := range raw {
		switch {
		case r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z':
			hasLetter = true
		case r >= '0' && r <= '9':
			hasDigit = true
		case strings.ContainsRune("._:-", r):
			hasSeparator = true
		default:
			return "", ErrInvalidEndUserSubject
		}
	}
	if !hasLetter || !hasDigit || !hasSeparator {
		return "", ErrInvalidEndUserSubject
	}
	return common.GenerateHMAC(fmt.Sprintf("real-person-end-user-subject/v1\n%d\n%s", appID, raw)), nil
}

func realPersonVerificationExpiresAt(providerExpiresAt, now int64) int64 {
	if providerExpiresAt <= now {
		return now + defaultRealPersonVerificationTTLSeconds
	}
	return providerExpiresAt
}
