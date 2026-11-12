package model

// An absent global snapshot entry means neither a channel nor a month record
// existed at submission. Apply the same recorded default without reading live
// configuration. An existing entry with nil Discount remains explicitly unknown.
func (scope *UpstreamExportScope) Channel(id int, period int64) UpstreamExportChannel {
	channel, exists := scope.Channels[id]
	if !exists && scope.AllChannels {
		record := inheritedProviderChannelDiscount(period, id, ProviderChannelBillingDiscount{}, false)
		projection := providerChannelDiscountProjection(record)
		channel.Discount = &projection
	}
	return channel
}
