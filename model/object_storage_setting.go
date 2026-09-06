package model

import (
	"errors"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/system_setting"
	"gorm.io/gorm"
)

// 对象存储配置的持久化与分发。配置以单行 Option 持久化，但不进入通用
// OptionMap：updateOptionMap 对该命名空间直接转发给观察者（model/option.go
// 中的唯一接线点），普通选项接口因此读不到、也绕不过校验写入。

var objectStorageSettingObserver func(value string)

// SetObjectStorageSettingObserver 注册跨包观察者。service 在包初始化时注册，
// 保证 InitOptionMap 装载数据库配置时观察者已经就位。
func SetObjectStorageSettingObserver(fn func(value string)) {
	objectStorageSettingObserver = fn
}

// NotifyObjectStorageSettingUpdate 把该命名空间的最新配置值转发给观察者。
func NotifyObjectStorageSettingUpdate(value string) {
	if objectStorageSettingObserver != nil {
		objectStorageSettingObserver(value)
	}
}

// GetObjectStorageSetting 读取持久化的原始配置 JSON；未配置时返回空串。
func GetObjectStorageSetting() (string, error) {
	var option Option
	err := DB.Where("key = ?", system_setting.ObjectStorageSettingOptionKey).First(&option).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return "", nil
		}
		return "", err
	}
	return option.Value, nil
}

// SaveObjectStorageSetting 在单事务中原子保存完整配置 JSON，并通过
// updateOptionMap 分发给本进程观察者；其他节点经既有 SyncOptions 周期装载。
func SaveObjectStorageSetting(value string) error {
	var next system_setting.ObjectStorageConfig
	if err := common.UnmarshalJsonStr(value, &next); err != nil {
		return err
	}
	err := DB.Transaction(func(tx *gorm.DB) error {
		option := Option{Key: system_setting.ObjectStorageSettingOptionKey}
		if err := tx.FirstOrCreate(&option, Option{Key: option.Key}).Error; err != nil {
			return err
		}
		if err := lockForUpdate(tx).Where("key = ?", option.Key).First(&option).Error; err != nil {
			return err
		}
		var previous system_setting.ObjectStorageConfig
		if option.Value != "" {
			if err := common.UnmarshalJsonStr(option.Value, &previous); err != nil {
				return err
			}
		}
		// Online updates rotate credentials only. Moving or disabling an established
		// store requires offline maintenance, regardless of current task counts.
		if previous.Backend != "" && previous.Backend != system_setting.ObjectStorageBackendUpstream &&
			(previous.Backend != next.Backend || previous.Endpoint != next.Endpoint ||
				previous.Bucket != next.Bucket || previous.Prefix != next.Prefix ||
				previous.Region != next.Region || previous.AccountName != next.AccountName) {
			return errors.New("Storage location changes require offline maintenance and data migration; only credentials can be updated here.")
		}
		return tx.Model(&option).Update("value", value).Error
	})
	if err != nil {
		return err
	}
	NotifyObjectStorageSettingUpdate(value)
	return nil
}
