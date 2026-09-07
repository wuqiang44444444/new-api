package service

import (
	"fmt"
	"net/url"
	"strings"
)

// BuildDashboardTaskArtifactContentURL keeps dashboard media on the current
// deployment. Public API links continue using the configured public address.
func BuildDashboardTaskArtifactContentURL(taskID, artifactKey string) (string, error) {
	taskID, artifactKey = strings.TrimSpace(taskID), strings.TrimSpace(artifactKey)
	access, err := IssueTaskArtifactAccess(taskID, artifactKey)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("/v1/tasks/%s/artifacts/%s/content?%s",
		url.PathEscape(taskID), url.PathEscape(artifactKey),
		url.Values{TaskArtifactAccessQueryParameter: {access}}.Encode()), nil
}
