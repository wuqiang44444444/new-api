package model

import "gorm.io/gorm"

func migrateTaskApplicationScope() error {
	var tasks []Task
	return DB.Select("id", "private_data").
		Where("app_id = ? AND client_protocol <> ?", 0, "").
		FindInBatches(&tasks, 100, func(tx *gorm.DB, _ int) error {
			for i := range tasks {
				appID := tasks[i].PrivateData.AppID
				if appID <= 0 {
					appID = tasks[i].PrivateData.TokenId
				}
				if appID <= 0 {
					continue
				}
				if err := tx.Model(&Task{}).Where("id = ? AND app_id = ?", tasks[i].ID, 0).Update("app_id", appID).Error; err != nil {
					return err
				}
			}
			return nil
		}).Error
}
