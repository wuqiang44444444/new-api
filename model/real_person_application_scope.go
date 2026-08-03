package model

import (
	"errors"

	"gorm.io/gorm"
)

func GetRealPersonAuthorizationForApp(userID, appID int, publicID string) (*RealPersonAuthorization, error) {
	var authorization RealPersonAuthorization
	err := DB.Where("user_id = ? AND app_id = ? AND public_id = ?", userID, appID, publicID).First(&authorization).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	return &authorization, err
}

func GetLatestRealPersonVerificationSession(authorizationID int64) (*RealPersonVerificationSession, error) {
	var session RealPersonVerificationSession
	err := DB.Where("authorization_id = ?", authorizationID).Order("id desc").First(&session).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	return &session, err
}
