package controller

import (
	"mime"
	"net/http"
	"path/filepath"
	"strings"
)

var safeVideoContentResponseHeaders = []string{
	"Content-Type",
	"Content-Length",
	"Content-Range",
	"Accept-Ranges",
	"ETag",
	"Last-Modified",
}

func copySafeVideoContentHeaders(destination, source http.Header) {
	for _, name := range safeVideoContentResponseHeaders {
		if value := source.Get(name); value != "" {
			destination.Set(name, value)
		}
	}
	if disposition := safeVideoContentDisposition(source.Get("Content-Disposition")); disposition != "" {
		destination.Set("Content-Disposition", disposition)
	}
}

func safeVideoContentDisposition(value string) string {
	disposition, params, err := mime.ParseMediaType(value)
	if err != nil || (disposition != "attachment" && disposition != "inline") {
		return ""
	}
	filename := filepath.Base(strings.TrimSpace(params["filename"]))
	filename = strings.ReplaceAll(filename, "\r", "")
	filename = strings.ReplaceAll(filename, "\n", "")
	if filename == "" || filename == "." {
		return disposition
	}
	return mime.FormatMediaType(disposition, map[string]string{"filename": filename})
}
