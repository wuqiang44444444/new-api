package jsplugin

import "errors"

// Only host-authored semantic validation errors carry an administrator-safe
// reason. Their messages must contain no values supplied by the artifact.
type seedanceConfigurationValidationError struct {
	reason string
}

func (err *seedanceConfigurationValidationError) Error() string { return err.reason }

// SeedanceCompilationDiagnostic exposes declaration validation failures without
// returning JavaScript exceptions, source excerpts or arbitrary metadata values.
// Both the management API and the offline upgrade check use this boundary.
func SeedanceCompilationDiagnostic(err error) string {
	var validation *seedanceConfigurationValidationError
	if errors.As(err, &validation) {
		return validation.reason
	}
	return "invalid plugin code or declaration"
}
