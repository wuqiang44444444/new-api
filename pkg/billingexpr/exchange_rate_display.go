package billingexpr

// Cache entries contain only the amount projection. Attach a copy of this
// response's rate fact so equal rates never share timestamps or mutable metadata.
func projectionWithExchangeRate(projection *DisplayProjection, rate *ExchangeRateContext) *DisplayProjection {
	result := *projection
	fact := *rate
	result.AppliedExchangeRate = &fact
	return &result
}
