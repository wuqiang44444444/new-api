package model

import (
	"errors"
)

const (
	TaskAssetAuthorizationReserved  = "reserved"
	TaskAssetAuthorizationTaskBound = "task_bound"
	TaskAssetAuthorizationClosed    = "closed"
)

type TaskAssetAuthorization struct {
	ID                 int64  `json:"-" gorm:"primaryKey"`
	AttemptID          string `json:"-" gorm:"type:varchar(64);uniqueIndex:idx_task_asset_attempt_authorization"`
	TaskID             string `json:"-" gorm:"type:varchar(191);uniqueIndex:idx_task_asset_task_authorization;index"`
	UserID             int    `json:"-" gorm:"index"`
	AppID              int    `json:"-" gorm:"index"`
	EndUserSubjectHash string `json:"-" gorm:"type:varchar(64);index"`
	AssetID            int64  `json:"-" gorm:"uniqueIndex:idx_task_asset_attempt_authorization;uniqueIndex:idx_task_asset_task_authorization"`
	AuthorizationID    int64  `json:"-" gorm:"uniqueIndex:idx_task_asset_attempt_authorization;uniqueIndex:idx_task_asset_task_authorization;index:idx_task_asset_authorization_state"`
	AssetKind          string `json:"-" gorm:"type:varchar(32)"`
	State              string `json:"-" gorm:"type:varchar(20);index:idx_task_asset_authorization_state"`
	CreatedAt          int64  `json:"-" gorm:"bigint"`
	UpdatedAt          int64  `json:"-" gorm:"bigint"`
}

func TaskContentAuthorizationActive(userID, appID int, subjectHash, taskID string) (bool, error) {
	if userID <= 0 || appID <= 0 || subjectHash == "" || taskID == "" {
		return false, errors.New("task authorization identity is incomplete")
	}
	var reservations []TaskAssetAuthorization
	if err := DB.Where("user_id = ? AND app_id = ? AND end_user_subject_hash = ? AND task_id = ? AND state = ?",
		userID,
		appID,
		subjectHash,
		taskID,
		TaskAssetAuthorizationTaskBound,
	).Find(&reservations).Error; err != nil {
		return false, err
	}
	for i := range reservations {
		var count int64
		if err := DB.Model(&RealPersonAuthorization{}).
			Where("id = ? AND user_id = ? AND app_id = ? AND end_user_subject_hash = ? AND status = ?",
				reservations[i].AuthorizationID,
				userID,
				appID,
				subjectHash,
				RealPersonAuthorizationAuthorized,
			).
			Count(&count).Error; err != nil {
			return false, err
		}
		if count != 1 {
			return false, nil
		}
	}
	return true, nil
}

func HasRevokedTaskAssetAuthorizationWork() bool {
	var id int64
	err := DB.Model(&TaskAssetAuthorization{}).
		Joins("JOIN real_person_authorizations ON real_person_authorizations.id = task_asset_authorizations.authorization_id").
		Where("task_asset_authorizations.state IN ? AND real_person_authorizations.status IN ?",
			[]string{TaskAssetAuthorizationReserved, TaskAssetAuthorizationTaskBound},
			[]string{RealPersonAuthorizationRevoked, RealPersonAuthorizationDeleting, RealPersonAuthorizationDeleted},
		).
		Limit(1).
		Pluck("task_asset_authorizations.id", &id).Error
	return err == nil && id != 0
}

func GetRevokedTaskAssetAuthorizationWork(limit int) []*TaskAssetAuthorization {
	if limit <= 0 {
		return nil
	}
	var relations []*TaskAssetAuthorization
	err := DB.Model(&TaskAssetAuthorization{}).
		Joins("JOIN real_person_authorizations ON real_person_authorizations.id = task_asset_authorizations.authorization_id").
		Where("task_asset_authorizations.state IN ? AND real_person_authorizations.status IN ?",
			[]string{TaskAssetAuthorizationReserved, TaskAssetAuthorizationTaskBound},
			[]string{RealPersonAuthorizationRevoked, RealPersonAuthorizationDeleting, RealPersonAuthorizationDeleted},
		).
		Order("task_asset_authorizations.id").
		Limit(limit).
		Find(&relations).Error
	if err != nil {
		return nil
	}
	return relations
}
