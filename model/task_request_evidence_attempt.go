package model

// Read the existing attempt facts; evidence never creates a second send permit.
func TaskCreateAttemptAllowsEvidenceSend(id int64) (bool, error) {
	var attempt TaskCreateAttempt
	err := DB.Select("status", "billing_hold_state").First(&attempt, "id = ?", id).Error
	return err == nil && attempt.Status == TaskCreateAttemptSending && attempt.BillingHoldState == TaskCreateAttemptBillingHeld, err
}
