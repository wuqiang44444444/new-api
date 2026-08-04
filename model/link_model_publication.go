package model

import (
	"errors"
	"fmt"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var ErrLinkModelPublicationConflict = errors.New("Link customer model publication conflicts with an existing SKU")
var ErrLinkModelPublicationInvalidRebind = errors.New("invalid Link publication rebind")
var ErrLinkModelPublicationVersionConflict = errors.New("Link publication version conflict")
var ErrLinkModelPublicationNoopRebind = errors.New("Link publication already points to the requested SKU")
var ErrLinkModelPublicationConcurrentChange = errors.New("Link publication changed concurrently")

// LinkPubSnapshot is embedded at durable execution boundaries so one
// publication identity is frozen consistently without duplicating schema tags.
type LinkPubSnapshot struct {
	LinkContractNamespace    string `json:"link_contract_namespace,omitempty" gorm:"type:varchar(64);index"`
	LinkRouteFamily          string `json:"link_route_family,omitempty" gorm:"type:varchar(64);index"`
	PublishedLinkContractSKU string `json:"published_link_contract_sku,omitempty" gorm:"type:varchar(191);index"`
	LinkPublicationVersion   int64  `json:"link_publication_version,omitempty" gorm:"bigint"`
}

type LinkModelPublication struct {
	ID                 int64           `json:"id" gorm:"primaryKey"`
	ContractNamespace  string          `json:"contract_namespace" gorm:"type:varchar(64);uniqueIndex:idx_link_model_publication,priority:1"`
	RouteFamily        LinkRouteFamily `json:"route_family" gorm:"type:varchar(64);uniqueIndex:idx_link_model_publication,priority:2;index"`
	CustomerModel      string          `json:"customer_model" gorm:"type:varchar(191);uniqueIndex:idx_link_model_publication,priority:3;index"`
	LinkSKU            string          `json:"link_sku" gorm:"type:varchar(191);index"`
	PublicationVersion int64           `json:"publication_version"`
	CreatedBy          int             `json:"created_by"`
	UpdatedBy          int             `json:"updated_by"`
	SourceChannelID    int             `json:"source_channel_id" gorm:"index"`
	ChangeReason       string          `json:"change_reason" gorm:"type:varchar(500)"`
	CreatedAt          int64           `json:"created_at" gorm:"bigint;index"`
	UpdatedAt          int64           `json:"updated_at" gorm:"bigint"`
}

type LinkModelPublicationAudit struct {
	ID                 int64           `json:"id" gorm:"primaryKey"`
	PublicationID      int64           `json:"publication_id" gorm:"uniqueIndex:idx_link_model_publication_audit,priority:1;index"`
	PublicationVersion int64           `json:"publication_version" gorm:"uniqueIndex:idx_link_model_publication_audit,priority:2"`
	ContractNamespace  string          `json:"contract_namespace" gorm:"type:varchar(64)"`
	RouteFamily        LinkRouteFamily `json:"route_family" gorm:"type:varchar(64)"`
	CustomerModel      string          `json:"customer_model" gorm:"type:varchar(191)"`
	PreviousLinkSKU    string          `json:"previous_link_sku" gorm:"type:varchar(191)"`
	LinkSKU            string          `json:"link_sku" gorm:"type:varchar(191)"`
	ChangedBy          int             `json:"changed_by"`
	SourceChannelID    int             `json:"source_channel_id" gorm:"index"`
	Reason             string          `json:"reason" gorm:"type:varchar(500)"`
	CreatedAt          int64           `json:"created_at" gorm:"bigint;index"`
}

type LinkModelPublicationKey struct {
	ContractNamespace string
	RouteFamily       LinkRouteFamily
	CustomerModel     string
}

func normalizeLinkModelPublicationKey(key LinkModelPublicationKey) LinkModelPublicationKey {
	key.ContractNamespace = strings.TrimSpace(key.ContractNamespace)
	if key.ContractNamespace == "" {
		key.ContractNamespace = LinkContractNamespaceDefault
	}
	key.RouteFamily = LinkRouteFamily(strings.TrimSpace(string(key.RouteFamily)))
	key.CustomerModel = strings.TrimSpace(key.CustomerModel)
	return key
}

func GetLinkModelPublication(namespace string, routeFamily LinkRouteFamily, customerModel string) (*LinkModelPublication, error) {
	key := normalizeLinkModelPublicationKey(LinkModelPublicationKey{ContractNamespace: namespace, RouteFamily: routeFamily, CustomerModel: customerModel})
	if key.RouteFamily == "" || key.CustomerModel == "" {
		return nil, gorm.ErrRecordNotFound
	}
	var publication LinkModelPublication
	err := DB.Where("contract_namespace = ? AND route_family = ? AND customer_model = ?", key.ContractNamespace, key.RouteFamily, key.CustomerModel).First(&publication).Error
	if err != nil {
		return nil, err
	}
	return &publication, nil
}

func GetUniqueLinkModelPublication(namespace, customerModel string) (*LinkModelPublication, error) {
	namespace = strings.TrimSpace(namespace)
	if namespace == "" {
		namespace = LinkContractNamespaceDefault
	}
	var publications []LinkModelPublication
	if err := DB.Where("contract_namespace = ? AND customer_model = ?", namespace, strings.TrimSpace(customerModel)).Limit(2).Find(&publications).Error; err != nil {
		return nil, err
	}
	if len(publications) == 0 {
		return nil, gorm.ErrRecordNotFound
	}
	if len(publications) != 1 {
		return nil, fmt.Errorf("customer model %q is published in multiple Link route families", customerModel)
	}
	return &publications[0], nil
}

func EnsureLinkModelPublication(tx *gorm.DB, key LinkModelPublicationKey, linkSKU string, actorID, sourceChannelID int, reason string) (*LinkModelPublication, error) {
	if tx == nil {
		tx = DB
	}
	key = normalizeLinkModelPublicationKey(key)
	linkSKU = strings.TrimSpace(linkSKU)
	if key.RouteFamily == "" || key.CustomerModel == "" || !IsRegisteredLinkSKU(linkSKU) {
		return nil, errors.New("Link customer model publication is incomplete")
	}
	now := common.GetTimestamp()
	publication := &LinkModelPublication{
		ContractNamespace: key.ContractNamespace, RouteFamily: key.RouteFamily, CustomerModel: key.CustomerModel,
		LinkSKU: linkSKU, PublicationVersion: 1, CreatedBy: actorID, UpdatedBy: actorID,
		SourceChannelID: sourceChannelID, ChangeReason: strings.TrimSpace(reason), CreatedAt: now, UpdatedAt: now,
	}
	result := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(publication)
	if result.Error != nil {
		return nil, result.Error
	}
	if result.RowsAffected == 1 {
		if err := createLinkModelPublicationAudit(tx, publication, "", actorID, sourceChannelID, reason); err != nil {
			return nil, err
		}
		return publication, nil
	}
	var existing LinkModelPublication
	if err := tx.Where("contract_namespace = ? AND route_family = ? AND customer_model = ?", key.ContractNamespace, key.RouteFamily, key.CustomerModel).First(&existing).Error; err != nil {
		return nil, err
	}
	if existing.LinkSKU != linkSKU {
		return nil, fmt.Errorf("%w: %s/%s/%s is version %d of %q, proposed %q", ErrLinkModelPublicationConflict, key.ContractNamespace, key.RouteFamily, key.CustomerModel, existing.PublicationVersion, existing.LinkSKU, linkSKU)
	}
	return &existing, nil
}

func RebindLinkModelPublication(key LinkModelPublicationKey, newLinkSKU string, expectedVersion int64, actorID int, reason string) (*LinkModelPublication, error) {
	key = normalizeLinkModelPublicationKey(key)
	newLinkSKU = strings.TrimSpace(newLinkSKU)
	reason = strings.TrimSpace(reason)
	if expectedVersion <= 0 || actorID <= 0 || reason == "" || !IsRegisteredLinkSKU(newLinkSKU) {
		return nil, fmt.Errorf("%w: expected version, operator, reason, and a registered SKU are required", ErrLinkModelPublicationInvalidRebind)
	}
	if !linkSKUHasRouteFamily(newLinkSKU, key.RouteFamily) {
		return nil, fmt.Errorf("%w: Link SKU %q is not implemented for route family %q", ErrLinkModelPublicationInvalidRebind, newLinkSKU, key.RouteFamily)
	}
	var updated LinkModelPublication
	err := DB.Transaction(func(tx *gorm.DB) error {
		var current LinkModelPublication
		if err := lockForUpdate(tx).Where("contract_namespace = ? AND route_family = ? AND customer_model = ?", key.ContractNamespace, key.RouteFamily, key.CustomerModel).First(&current).Error; err != nil {
			return err
		}
		if current.PublicationVersion != expectedVersion {
			return fmt.Errorf("%w: changed from expected %d to %d", ErrLinkModelPublicationVersionConflict, expectedVersion, current.PublicationVersion)
		}
		if current.LinkSKU == newLinkSKU {
			return ErrLinkModelPublicationNoopRebind
		}
		previousSKU := current.LinkSKU
		now := common.GetTimestamp()
		nextVersion := current.PublicationVersion + 1
		result := tx.Model(&LinkModelPublication{}).
			Where("id = ? AND publication_version = ?", current.ID, current.PublicationVersion).
			Updates(map[string]any{"link_sku": newLinkSKU, "publication_version": nextVersion, "updated_by": actorID, "change_reason": reason, "updated_at": now})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return ErrLinkModelPublicationConcurrentChange
		}
		current.LinkSKU, current.PublicationVersion = newLinkSKU, nextVersion
		current.UpdatedBy, current.ChangeReason, current.UpdatedAt = actorID, reason, now
		if err := createLinkModelPublicationAudit(tx, &current, previousSKU, actorID, current.SourceChannelID, reason); err != nil {
			return err
		}
		updated = current
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &updated, nil
}

func createLinkModelPublicationAudit(tx *gorm.DB, publication *LinkModelPublication, previousSKU string, actorID, sourceChannelID int, reason string) error {
	if publication == nil {
		return errors.New("Link publication audit requires a publication")
	}
	return tx.Create(&LinkModelPublicationAudit{
		PublicationID: publication.ID, PublicationVersion: publication.PublicationVersion,
		ContractNamespace: publication.ContractNamespace, RouteFamily: publication.RouteFamily, CustomerModel: publication.CustomerModel,
		PreviousLinkSKU: strings.TrimSpace(previousSKU), LinkSKU: publication.LinkSKU, ChangedBy: actorID,
		SourceChannelID: sourceChannelID, Reason: strings.TrimSpace(reason), CreatedAt: common.GetTimestamp(),
	}).Error
}

func linkSKUHasRouteFamily(linkSKU string, routeFamily LinkRouteFamily) bool {
	for _, implementation := range LinkImplementationsForSKU(linkSKU) {
		for _, binding := range implementation.ExecutionBindings {
			if binding.LinkSKU == linkSKU && binding.RouteFamily == routeFamily {
				return true
			}
		}
	}
	return false
}

func ListLinkModelPublications(namespace string, routeFamily LinkRouteFamily, customerModel string) ([]LinkModelPublication, error) {
	query := DB.Model(&LinkModelPublication{})
	if namespace = strings.TrimSpace(namespace); namespace != "" {
		query = query.Where("contract_namespace = ?", namespace)
	}
	if routeFamily = LinkRouteFamily(strings.TrimSpace(string(routeFamily))); routeFamily != "" {
		query = query.Where("route_family = ?", routeFamily)
	}
	if customerModel = strings.TrimSpace(customerModel); customerModel != "" {
		query = query.Where("customer_model = ?", customerModel)
	}
	var publications []LinkModelPublication
	err := query.Order("contract_namespace asc").Order("route_family asc").Order("customer_model asc").Find(&publications).Error
	return publications, err
}
