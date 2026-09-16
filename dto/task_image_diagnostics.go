package dto

// TaskImageDiagnostics is an admin-only projection of bounded execution facts.
// Provider request IDs remain in the existing RootInfo projection.
type TaskImageDiagnostics struct {
	UpstreamStatus  int  `json:"upstream_status,omitempty"`
	ViolationMarker bool `json:"violation_marker"`
}
