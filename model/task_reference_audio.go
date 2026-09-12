package model

// TaskReferenceAudioFact records request-scoped audio storage, not a reusable Asset.
// The owning Task/attempt supplies user_id + app_id. No input or signed URL is saved.
type TaskReferenceAudioFact struct {
	ContentIndex    int                           `json:"content_index"`
	ObjectKey       string                        `json:"object_key"`
	StorageLocation FunCloudHostedStorageLocation `json:"storage_location"`
	MimeType        string                        `json:"mime_type"`
	SizeBytes       int64                         `json:"size_bytes"`
}
