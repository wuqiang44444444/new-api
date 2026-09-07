package model

// AwaitingUsage retains the precharge until a trusted successful observation
// supplies usage. Funding reconciliation must not retry absent evidence.
const TaskBillingStateAwaitingUsage TaskBillingState = "awaiting_usage"
