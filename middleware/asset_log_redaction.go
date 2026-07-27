package middleware

import "strings"

func redactAssetBearerToken(path string) string {
	requestPath, rawQuery, hasQuery := strings.Cut(path, "?")
	hasTrailingSlash := len(requestPath) > 1 && strings.HasSuffix(requestPath, "/")
	segments := strings.Split(strings.TrimSuffix(requestPath, "/"), "/")
	redactedPath := requestPath

	switch {
	case len(segments) == 4 && segments[1] == "consent" && segments[2] == "real-person" && segments[3] != "" && segments[3] != "complete" && segments[3] != "receipt":
		segments[3] = ":token"
		redactedPath = strings.Join(segments, "/")
	case len(segments) == 5 && segments[1] == "consent" && segments[2] == "real-person" && segments[3] == "receipt" && segments[4] != "":
		segments[4] = ":receipt_token"
		redactedPath = strings.Join(segments, "/")
	case len(segments) == 5 && segments[1] == "api" && segments[2] == "real-person-consents" && segments[3] != "" && (segments[4] == "accept" || segments[4] == "reject"):
		segments[3] = ":token"
		redactedPath = strings.Join(segments, "/")
	case len(segments) == 6 && segments[1] == "api" && segments[2] == "real-person-consents" && segments[3] == "receipt" && segments[4] != "" && segments[5] == "revoke":
		segments[4] = ":receipt_token"
		redactedPath = strings.Join(segments, "/")
	}
	if redactedPath != requestPath && hasTrailingSlash {
		redactedPath += "/"
	}
	if requestPath == "/consent/real-person/complete" {
		return redactedPath
	}

	if hasQuery {
		return redactedPath + "?" + rawQuery
	}
	return redactedPath
}
