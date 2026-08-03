package service

import (
	"errors"

	"github.com/QuantumNous/new-api/model"
)

type TaskCreateAttemptManualRejection struct {
	AttemptID        string `json:"attempt_id"`
	PublicTaskID     string `json:"public_task_id"`
	Status           string `json:"status"`
	BillingHoldState string `json:"billing_hold_state"`
	ReleasedQuota    int    `json:"released_quota"`
}

func RejectUnknownTaskCreateAttempt(
	attemptID string,
	providerVerified bool,
	operatorID int,
	note string,
) (*TaskCreateAttemptManualRejection, error) {
	if !providerVerified {
		return nil, errors.New("provider verification is required before manual rejection")
	}
	released, err := model.RejectUnknownTaskCreateAttempt(attemptID, operatorID, note)
	if err != nil {
		return nil, err
	}
	attempt, err := model.GetTaskCreateAttemptByAttemptID(attemptID)
	if err != nil {
		return nil, err
	}
	return &TaskCreateAttemptManualRejection{
		AttemptID:        attempt.AttemptID,
		PublicTaskID:     attempt.PublicTaskID,
		Status:           string(attempt.Status),
		BillingHoldState: string(attempt.BillingHoldState),
		ReleasedQuota:    released.ReleasedQuota,
	}, nil
}
