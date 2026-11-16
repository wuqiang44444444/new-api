package service

import "net/http"

// dropUpstreamBodyValidators removes metadata for the upstream representation
// once the gateway has added reasoning_content to the response.
func dropUpstreamBodyValidators(headers http.Header) {
	for _, name := range [...]string{"Content-Digest", "Repr-Digest", "Digest", "Content-MD5", "ETag"} {
		headers.Del(name)
	}
}
