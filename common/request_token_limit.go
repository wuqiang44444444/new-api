package common

// MaxRequestTokens is the shared upper bound for request output quantities.
// Native relay validation and durable Batch validation must agree.
const MaxRequestTokens = MaxQuota / 2
