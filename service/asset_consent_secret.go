package service

import "fmt"

func realPersonVerificationSecretScope(authorizationID, sessionID int64) string {
	return fmt.Sprintf("real-person-verification/%d/%d", authorizationID, sessionID)
}
