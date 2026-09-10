package model

import (
	"errors"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
)

var ErrBatchFileNotFound = errors.New("batch file not found")

// BatchFile is one platform-persisted customer upload. Content lives only in
// the private object store; the row records the reference and the validated
// shape summary. Ownership is user_id + app_id; the uploading key is kept for
// audit only.
type BatchFile struct {
	Id           string `json:"id" gorm:"type:varchar(64);primaryKey"`
	UserId       int    `json:"user_id" gorm:"index"`
	AppID        int    `json:"app_id" gorm:"index"`
	TokenId      int    `json:"token_id" gorm:"index"`
	FileName     string `json:"filename" gorm:"type:varchar(255)"`
	Purpose      string `json:"purpose" gorm:"type:varchar(32)"`
	SizeBytes    int64  `json:"bytes"`
	LineCount    int64  `json:"line_count"`
	ModelName    string `json:"model" gorm:"type:varchar(255)"`
	Checksum     string `json:"checksum" gorm:"type:varchar(64)"`
	ObjectKey    string `json:"-" gorm:"type:varchar(512)"`
	UploadSource string `json:"-" gorm:"type:varchar(32)"`
	CreatedAt    int64  `json:"created_at" gorm:"bigint"`
}

func newBatchFileId() (string, error) {
	key, err := common.GenerateRandomCharsKey(28)
	if err != nil {
		return "", err
	}
	return "file-" + strings.ToLower(key), nil
}

type CreateBatchFileParams struct {
	UserId       int
	AppID        int
	TokenId      int
	FileName     string
	Purpose      string
	SizeBytes    int64
	LineCount    int64
	ModelName    string
	Checksum     string
	ObjectKey    string
	UploadSource string
}

func CreateBatchFile(params CreateBatchFileParams) (*BatchFile, error) {
	id, err := newBatchFileId()
	if err != nil {
		return nil, err
	}
	file := &BatchFile{
		Id: id, UserId: params.UserId, AppID: params.AppID, TokenId: params.TokenId,
		FileName: params.FileName, Purpose: params.Purpose,
		SizeBytes: params.SizeBytes, LineCount: params.LineCount, ModelName: params.ModelName,
		Checksum: params.Checksum, ObjectKey: params.ObjectKey, UploadSource: params.UploadSource,
		CreatedAt: common.GetTimestamp(),
	}
	if err := DB.Create(file).Error; err != nil {
		return nil, err
	}
	return file, nil
}

func GetBatchFileOwned(fileId string, userId int, appID int) (*BatchFile, error) {
	if fileId == "" || userId <= 0 || appID <= 0 {
		return nil, ErrBatchFileNotFound
	}
	var file BatchFile
	if err := DB.Where("id = ? AND user_id = ? AND app_id = ?", fileId, userId, appID).First(&file).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrBatchFileNotFound
		}
		return nil, err
	}
	return &file, nil
}

type BatchFileListPage struct {
	Items   []BatchFile `json:"items"`
	Total   int64       `json:"total"`
	HasMore bool        `json:"has_more"`
}

func ListBatchFilesOwned(userId int, appID int, limit int, after string) (*BatchFileListPage, error) {
	if userId <= 0 || appID <= 0 {
		return nil, ErrBatchFileNotFound
	}
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	var total int64
	if err := DB.Model(&BatchFile{}).Where("user_id = ? AND app_id = ?", userId, appID).Count(&total).Error; err != nil {
		return nil, err
	}
	page := &BatchFileListPage{Items: []BatchFile{}}
	query := DB.Where("user_id = ? AND app_id = ?", userId, appID)
	if after != "" {
		cursor, err := GetBatchFileOwned(after, userId, appID)
		if err != nil {
			return nil, err
		}
		query = query.Where("created_at < ? OR (created_at = ? AND id < ?)", cursor.CreatedAt, cursor.CreatedAt, cursor.Id)
	}
	err := query.
		Order("created_at DESC, id DESC").
		Limit(limit + 1).
		Find(&page.Items).Error
	if err != nil {
		return nil, err
	}
	page.Total = total
	page.HasMore = len(page.Items) > limit
	if page.HasMore {
		page.Items = page.Items[:limit]
	}
	return page, nil
}

// NewBatchObjectName generates the random private object name used before a
// database row exists.
func NewBatchObjectName() (string, error) {
	key, err := common.GenerateRandomCharsKey(24)
	if err != nil {
		return "", err
	}
	return key, nil
}
