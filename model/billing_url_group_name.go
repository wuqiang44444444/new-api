package model

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/QuantumNous/new-api/common"

	"gorm.io/gorm"
)

// MaxProviderURLGroupNameLength bounds the trimmed display name in Unicode code points.
const MaxProviderURLGroupNameLength = 255

// maxBillingURLGroupKeyLength matches the controller-side bound for URL
// grouping filters; grouping keys can carry long base paths.
const maxBillingURLGroupKeyLength = 2048

const (
	providerURLGroupNameEntity     = "url_group_name"
	reasonURLGroupNameManualUpdate = "admin update of upstream display name"
)

// ProviderURLGroupDisplayName is the admin-defined alias of one upstream URL
// grouping key. It is a pure reporting label: it never re-groups channels,
// never proves the URL used at request time and never feeds routing, billing
// or settlement. A channel whose base URL changes reads the name of its new
// grouping key; old names are never migrated or deleted automatically.
type ProviderURLGroupDisplayName struct {
	Id int64 `json:"id"`
	// URLHash is the unique lookup key (sha256 of URLKey) so the text key stays
	// unbounded and the unique index stays valid on every supported database.
	URLHash   string `json:"url_hash" gorm:"type:char(64);not null;uniqueIndex"`
	URLKey    string `json:"url_key" gorm:"type:text;not null"`
	Name      string `json:"name" gorm:"type:varchar(255);not null"`
	CreatedBy int    `json:"created_by" gorm:"not null"`
	UpdatedBy int    `json:"updated_by" gorm:"not null"`
	CreatedAt int64  `json:"created_at" gorm:"autoCreateTime"`
	UpdatedAt int64  `json:"updated_at" gorm:"autoUpdateTime"`
}

// validBillingURLGroupKey accepts exactly the identities the URL summary can
// produce: a fully normalized base URL or one per-channel fallback key.
func validBillingURLGroupKey(urlKey string) bool {
	if urlKey == "" || len(urlKey) > maxBillingURLGroupKeyLength {
		return false
	}
	if channelId, ok := strings.CutPrefix(urlKey, billingURLGroupChannelFallbackPrefix); ok {
		id, err := strconv.Atoi(channelId)
		return err == nil && id > 0
	}
	return normalizeBillingURLGroupKey(urlKey) == urlKey
}

func providerURLGroupHash(urlKey string) string {
	return fmt.Sprintf("%x", common.Sha256Raw([]byte(urlKey)))
}

// SaveProviderURLGroupName stores or clears the display name of one grouping
// key in the same transaction as its audit row. An empty name clears the
// alias and restores the default label; saving is idempotent on content.
func SaveProviderURLGroupName(urlKey string, name string, operatorId int) error {
	urlKey = strings.TrimSpace(urlKey)
	if !validBillingURLGroupKey(urlKey) {
		return errors.New("invalid upstream URL grouping key")
	}
	name = strings.TrimSpace(name)
	if utf8.RuneCountInString(name) > MaxProviderURLGroupNameLength {
		return errors.New("upstream name is too long")
	}
	return DB.Transaction(func(tx *gorm.DB) error {
		var existing ProviderURLGroupDisplayName
		err := lockForUpdate(tx).Where("url_hash = ?", providerURLGroupHash(urlKey)).First(&existing).Error
		found := err == nil
		if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		if name == "" {
			if !found {
				return nil
			}
			if err := tx.Delete(&existing).Error; err != nil {
				return err
			}
			return createProviderBillingAudit(tx, providerURLGroupNameEntity, urlKey, "delete", &existing, nil, reasonURLGroupNameManualUpdate, operatorId)
		}
		if found && existing.Name == name {
			return nil
		}
		if found {
			before := existing
			result := tx.Model(&existing).Updates(map[string]interface{}{"name": name, "updated_by": operatorId})
			if result.Error != nil {
				return result.Error
			}
			after := existing
			after.Name = name
			after.UpdatedBy = operatorId
			return createProviderBillingAudit(tx, providerURLGroupNameEntity, urlKey, "update", &before, &after, reasonURLGroupNameManualUpdate, operatorId)
		}
		record := ProviderURLGroupDisplayName{
			URLHash: providerURLGroupHash(urlKey), URLKey: urlKey, Name: name,
			CreatedBy: operatorId, UpdatedBy: operatorId,
		}
		if err := tx.Create(&record).Error; err != nil {
			return err
		}
		return createProviderBillingAudit(tx, providerURLGroupNameEntity, urlKey, "create", nil, &record, reasonURLGroupNameManualUpdate, operatorId)
	})
}

// Alias scans use bounded database pages; callers retain only the display map.
func getProviderURLGroupNames() (map[string]string, error) {
	var rows []ProviderURLGroupDisplayName
	names := make(map[string]string)
	if err := DB.Select("id, url_key, name").FindInBatches(&rows, 500, func(tx *gorm.DB, batch int) error {
		for _, row := range rows {
			names[row.URLKey] = row.Name
		}
		return nil
	}).Error; err != nil {
		return nil, err
	}
	return names, nil
}
