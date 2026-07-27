package dto

type ChannelAssetCredentialInput struct {
	AccessKeyID     string `json:"access_key_id"`
	SecretAccessKey string `json:"secret_access_key"`
}

type ChannelAssetCredentialStatus struct {
	Configured      bool   `json:"configured"`
	AccessKeyIDHint string `json:"access_key_id_hint,omitempty"`
}
