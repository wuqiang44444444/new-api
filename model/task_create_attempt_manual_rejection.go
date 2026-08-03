package model

import (
	"errors"
	"strings"
)

// RejectUnknownTaskCreateAttempt records a provider-verified non-creation and
// releases the held customer funds in the same state transition. The caller is
// responsible for enforcing Root authorization and a fresh security proof.
func RejectUnknownTaskCreateAttempt(
	attemptID string,
	operatorID int,
	note string,
) (*TaskAttemptReleaseResult, error) {
	attemptID = strings.TrimSpace(attemptID)
	validatedNote, noteErr := validateOperationalAuditNote(note)
	if attemptID == "" || len(attemptID) > 64 || containsControlCharacter(attemptID) ||
		operatorID <= 0 || noteErr != nil {
		return nil, errors.New("manual task rejection identity is invalid")
	}
	var attempt TaskCreateAttempt
	if err := DB.Select("id").Where("attempt_id = ?", attemptID).First(&attempt).Error; err != nil {
		return nil, err
	}
	return releaseTaskCreateAttemptHold(
		attempt.ID,
		TaskCreateAttemptRejected,
		taskAttemptReleaseOptions{
			requireUnknown:         true,
			deleteIdempotencyClaim: true,
			operatorID:             operatorID,
			auditNote:              validatedNote,
		},
	)
}
