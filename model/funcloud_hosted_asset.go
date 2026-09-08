package model

import (
	"errors"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
)

// FunCloud 托管素材（funcloud_material_hosted）是已登记素材协议对应的平台
// 持久化事实：素材组只是官方素材流程所需的组织关系，素材对象保存在本站
// 私有对象存储中。按部署决策，托管素材组和素材仅按 user_id 隔离与共享
// （同账号所有 API Key 共享）；不绑定创建时的 API Key、Channel 或 FunCloud
// 上游账号，上游凭据轮换不影响已保存素材。

const (
	// FunCloudHostedAssetIDPrefix / FunCloudHostedGroupIDPrefix 是平台生成的
	// 不透明 ID 前缀。视频引用带该前缀时走托管解析；不带前缀的 asset://
	// 引用保持原样发送给 Provider，不探测、不双读、不回退。
	FunCloudHostedAssetIDPrefix = "fhas_"
	FunCloudHostedGroupIDPrefix = "fhgrp_"

	// FunCloudHostedAssetStatusReady / FunCloudHostedAssetStatusDeleted 是托管
	// 素材仅有的两个状态。删除只停止新引用，不删除本站对象。
	FunCloudHostedAssetStatusReady   = "ready"
	FunCloudHostedAssetStatusDeleted = "deleted"
)

type FunCloudHostedAssetGroup struct {
	ID          string `json:"id" gorm:"type:varchar(64);primaryKey"`
	UserID      int    `json:"user_id" gorm:"index;not null;autoIncrement:false"`
	Model       string `json:"model" gorm:"type:varchar(255);not null"`
	Name        string `json:"name" gorm:"type:varchar(255);not null"`
	Description string `json:"description" gorm:"type:varchar(512)"`
	CreatedAt   int64  `json:"created_at" gorm:"bigint;not null"`
	UpdatedAt   int64  `json:"updated_at" gorm:"bigint;not null"`
}

type FunCloudHostedAsset struct {
	ID              string                        `json:"id" gorm:"type:varchar(64);primaryKey"`
	UserID          int                           `json:"user_id" gorm:"index;not null;autoIncrement:false"`
	GroupID         string                        `json:"group_id" gorm:"type:varchar(64);index"`
	Model           string                        `json:"model" gorm:"type:varchar(255);not null"`
	Name            string                        `json:"name" gorm:"type:varchar(255);not null"`
	ObjectKey       string                        `json:"object_key" gorm:"type:varchar(512);not null"`
	StorageLocation FunCloudHostedStorageLocation `json:"storage_location" gorm:"embedded;embeddedPrefix:storage_"`
	MimeType        string                        `json:"mime_type" gorm:"type:varchar(127);not null"`
	SizeBytes       int64                         `json:"size_bytes" gorm:"bigint;not null"`
	Status          string                        `json:"status" gorm:"type:varchar(31);not null;index"`
	CreatedAt       int64                         `json:"created_at" gorm:"bigint;not null"`
	UpdatedAt       int64                         `json:"updated_at" gorm:"bigint;not null"`
}

// Object location excludes credentials and survives credential rotation. An
// unavailable location is rejected rather than interpreted in the current bucket.
type FunCloudHostedStorageLocation struct {
	Backend  string `json:"backend" gorm:"type:varchar(31)"`
	Endpoint string `json:"endpoint" gorm:"type:varchar(512)"`
	Bucket   string `json:"bucket" gorm:"type:varchar(255)"`
	Prefix   string `json:"prefix" gorm:"type:varchar(512)"`
}

func newFunCloudHostedID(prefix string) (string, error) {
	random, err := common.GenerateRandomCharsKey(24)
	if err != nil {
		return "", err
	}
	return prefix + random, nil
}

func NewFunCloudHostedAssetGroupID() (string, error) {
	return newFunCloudHostedID(FunCloudHostedGroupIDPrefix)
}

func NewFunCloudHostedAssetID() (string, error) {
	return newFunCloudHostedID(FunCloudHostedAssetIDPrefix)
}

func CreateFunCloudHostedAssetGroup(record *FunCloudHostedAssetGroup) error {
	if record == nil || record.UserID <= 0 || strings.TrimSpace(record.ID) == "" {
		return errors.New("hosted asset group is invalid")
	}
	now := common.GetTimestamp()
	record.CreatedAt = now
	record.UpdatedAt = now
	return DB.Create(record).Error
}

// GetFunCloudHostedAssetGroup 按 user_id + id 读取托管素材组；未命中返回
// (nil, nil)，跨用户与不存在的组不可区分。
func GetFunCloudHostedAssetGroup(userID int, groupID string) (*FunCloudHostedAssetGroup, error) {
	if userID <= 0 || strings.TrimSpace(groupID) == "" {
		return nil, errors.New("hosted asset group lookup is invalid")
	}
	var record FunCloudHostedAssetGroup
	result := DB.Session(&gorm.Session{Logger: DB.Logger.LogMode(gormlogger.Silent)}).
		Where("user_id = ? AND id = ?", userID, groupID).Limit(1).Find(&record)
	if result.Error != nil {
		return nil, result.Error
	}
	if result.RowsAffected == 0 {
		return nil, nil
	}
	return &record, nil
}

func CreateFunCloudHostedAsset(record *FunCloudHostedAsset) error {
	if record == nil || record.UserID <= 0 || strings.TrimSpace(record.ID) == "" ||
		strings.TrimSpace(record.ObjectKey) == "" {
		return errors.New("hosted asset record is invalid")
	}
	now := common.GetTimestamp()
	record.CreatedAt = now
	record.UpdatedAt = now
	return DB.Create(record).Error
}

// GetFunCloudHostedAsset 按 user_id + id 读取仍可引用的托管素材；已删除与
// 不存在同样返回 (nil, nil)，不向调用方泄漏其它用户素材的存在性。
func GetFunCloudHostedAsset(userID int, assetID string) (*FunCloudHostedAsset, error) {
	if userID <= 0 || strings.TrimSpace(assetID) == "" {
		return nil, errors.New("hosted asset lookup is invalid")
	}
	var record FunCloudHostedAsset
	result := DB.Session(&gorm.Session{Logger: DB.Logger.LogMode(gormlogger.Silent)}).
		Where("user_id = ? AND id = ? AND status = ?", userID, assetID, FunCloudHostedAssetStatusReady).
		Limit(1).Find(&record)
	if result.Error != nil {
		return nil, result.Error
	}
	if result.RowsAffected == 0 {
		return nil, nil
	}
	return &record, nil
}

// MarkFunCloudHostedAssetDeleted 只改变可引用状态；不删除本站对象，也不
// 中断已经受理的视频任务。存在性以读取判定，不依赖 UPDATE RowsAffected
// （MySQL 与 SQLite/PostgreSQL 对无变化 UPDATE 的计数语义不同），重复删除
// 在三种数据库上都按幂等成功返回。
func MarkFunCloudHostedAssetDeleted(userID int, assetID string) (bool, error) {
	if userID <= 0 || strings.TrimSpace(assetID) == "" {
		return false, errors.New("hosted asset delete is invalid")
	}
	db := DB.Session(&gorm.Session{Logger: DB.Logger.LogMode(gormlogger.Silent)})
	var existing FunCloudHostedAsset
	result := db.Where("user_id = ? AND id = ?", userID, assetID).Limit(1).Find(&existing)
	if result.Error != nil {
		return false, result.Error
	}
	if result.RowsAffected == 0 {
		return false, nil
	}
	return true, db.Model(&FunCloudHostedAsset{}).
		Where("user_id = ? AND id = ?", userID, assetID).
		Updates(map[string]any{
			"status":     FunCloudHostedAssetStatusDeleted,
			"updated_at": common.GetTimestamp(),
		}).Error
}

// TaskHostedMediaFact 描述一个已接受并冻结的 FunCloud 托管素材事实。签名
// URL 只在当次 Provider 调用中签发，不进入该快照。
type TaskHostedMediaFact struct {
	StorageLocation FunCloudHostedStorageLocation `json:"storage_location"`
	AssetID         string                        `json:"asset_id"`
	ObjectKey       string                        `json:"object_key"`
	MimeType        string                        `json:"mime_type,omitempty"`
	SizeBytes       int64                         `json:"size_bytes,omitempty"`
}
