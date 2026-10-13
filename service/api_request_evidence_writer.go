package service

import (
	"bufio"
	"bytes"
	"net"
	"net/http"
)

// Gin cannot observe the status written after Hijack. Track the actual HTTP
// upgrade header while leaving websocket wire frames to the frame observer.
type apiEvidenceWriter struct {
	imageEvidenceWriter
	hijacked         bool
	handshakeStatus  int
	handshakeHeaders http.Header
	control          *http.ResponseController
}

func (w *apiEvidenceWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	conn, rw, err := w.ResponseWriter.Hijack()
	if err != nil {
		return conn, rw, err
	}
	w.hijacked = true
	return &apiEvidenceConnection{Conn: conn, writer: w}, rw, nil
}

type apiEvidenceConnection struct {
	net.Conn
	writer *apiEvidenceWriter
	header bytes.Buffer
	done   bool
}

func (c *apiEvidenceConnection) Write(p []byte) (int, error) {
	n, err := c.Conn.Write(p)
	if !c.done {
		keep := min(n, (64<<10)-c.header.Len())
		if keep > 0 {
			_, _ = c.header.Write(p[:keep])
		}
		if bytes.Contains(c.header.Bytes(), []byte("\r\n\r\n")) {
			resp, parseErr := http.ReadResponse(bufio.NewReader(bytes.NewReader(c.header.Bytes())), nil)
			if parseErr == nil {
				c.writer.handshakeStatus = resp.StatusCode
				c.writer.handshakeHeaders = resp.Header.Clone()
			}
			c.done = true
			c.header.Reset()
		} else if c.header.Len() >= 64<<10 {
			c.done = true
			c.header.Reset()
		}
	}
	return n, err
}

func (w *apiEvidenceWriter) WriteHeader(status int) {
	if status >= 400 && w.control != nil {
		_ = w.control.EnableFullDuplex()
	}
	w.ResponseWriter.WriteHeader(status)
}
