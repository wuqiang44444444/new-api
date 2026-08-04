package model

import (
	"errors"
	"fmt"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
)

// MigrateDirectLinkModelPublications preserves the pre-publication behavior for
// channels whose customer model already is the registered Link SKU. Custom
// aliases are intentionally not guessed during migration; saving or enabling
// their selected Link access plan publishes them through the normal gate.
func MigrateDirectLinkModelPublications() error {
	if !DB.Migrator().HasTable(&LinkModelPublication{}) {
		return nil
	}
	var channels []Channel
	if err := DB.Where("status = ?", common.ChannelStatusEnabled).Find(&channels).Error; err != nil {
		return err
	}
	for i := range channels {
		channel := &channels[i]
		settings := channel.GetOtherSettings()
		if settings.LinkImplementation.Empty() {
			continue
		}
		if err := ValidateLinkImplementationRegistration(channel, &settings); err != nil {
			common.SysError(fmt.Sprintf("skip Link publication migration for invalid channel %d: %v", channel.Id, err))
			continue
		}
		executions, err := DeriveChannelLinkExecutions(channel, &settings)
		if err != nil {
			common.SysError(fmt.Sprintf("skip Link publication migration for channel %d: %v", channel.Id, err))
			continue
		}
		for _, execution := range executions {
			if execution.CustomerModel != execution.LinkSKU {
				continue
			}
			err := DB.Transaction(func(tx *gorm.DB) error {
				if err := rejectOrdinaryLinkModelConflict(tx, channel, execution); err != nil {
					return err
				}
				_, err := EnsureLinkModelPublication(tx, LinkModelPublicationKey{
					ContractNamespace: LinkContractNamespaceDefault,
					RouteFamily:       execution.Binding.RouteFamily,
					CustomerModel:     execution.CustomerModel,
				}, execution.LinkSKU, 0, channel.Id, "migration of direct registered Link SKU")
				return err
			})
			if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
				common.SysError(fmt.Sprintf("skip direct Link publication for channel %d model %s: %v", channel.Id, execution.CustomerModel, err))
			}
		}
	}
	return nil
}
