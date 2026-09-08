package model

// TaskNativeImageRequest preserves the exact adapter output. Connection and
// authentication are encrypted together; potentially large bodies live in the
// private object store, split into independently bounded objects.
type TaskNativeImageRequest struct {
	ConnectionCiphertext string              `json:"connection_ciphertext"`
	Body                 []TaskImageArtifact `json:"body"`
	BillingProbe         []TaskImageArtifact `json:"billing_probe,omitempty"`
}
