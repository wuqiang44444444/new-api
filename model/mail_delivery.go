package model

// Delivery states shared by durable operations mail. Unknown submissions are
// retained for reconciliation and are never eligible for automatic retry.
const (
	MailDeliveryPending   = "pending"
	MailDeliverySending   = "sending"
	MailDeliveryAccepted  = "transport_accepted"
	MailDeliveryRetryWait = "retry_wait"
	MailDeliveryUnknown   = "submission_unknown"
	MailDeliveryStopped   = "stopped"
)
