package middleware

import "strings"

func redactAssetBearerToken(path string) string {
	requestPath, rawQuery, hasQuery := strings.Cut(path, "?")
	hasTrailingSlash := len(requestPath) > 1 && strings.HasSuffix(requestPath, "/")
	segments := strings.Split(strings.TrimSuffix(requestPath, "/"), "/")
	redactedPath := requestPath

	switch {
	case len(segments) == 4 && segments[1] == "verification" && segments[2] == "real-person" && segments[3] != "" && segments[3] != "complete":
		segments[3] = ":token"
		redactedPath = strings.Join(segments, "/")
	}
	if redactedPath != requestPath && hasTrailingSlash {
		redactedPath += "/"
	}
	if requestPath == "/verification/real-person/complete" {
		return redactedPath
	}

	if hasQuery {
		return redactedPath + "?" + rawQuery
	}
	return redactedPath
}
