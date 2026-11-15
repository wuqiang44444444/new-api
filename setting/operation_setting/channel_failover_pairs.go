package operation_setting

// ChannelFailoverVerifiedPair records one source→target migration that was
// verified for a customer model. Revisions are stamped when the policy is
// saved, so any later channel configuration change disables the pair until
// it is verified and saved again.
type ChannelFailoverVerifiedPair struct {
	SourceChannelID     int    `json:"source_channel_id"`
	TargetChannelID     int    `json:"target_channel_id"`
	Model               string `json:"model"`
	UpstreamModel       string `json:"upstream_model,omitempty"`
	SourceUpstreamModel string `json:"source_upstream_model,omitempty"`
	SourceRevision      string `json:"source_revision,omitempty"`
	TargetRevision      string `json:"target_revision,omitempty"`
}

func (p ChannelFailoverVerifiedPair) EffectiveUpstreamModel() string {
	if p.UpstreamModel != "" {
		return p.UpstreamModel
	}
	return p.Model
}
