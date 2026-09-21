package model

// A missing key means unrecorded; a present zero is a measured zero. These are
// token subcategories and are never added back to the parent token totals.
func mergeUsageTokenDetails(target *map[string]int64, source map[string]int64) {
	if len(source) == 0 {
		return
	}
	if *target == nil {
		*target = make(map[string]int64)
	}
	for meter, value := range source {
		(*target)[meter] += value
	}
}
