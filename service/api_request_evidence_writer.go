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
}

func (w *apiEvidenceWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	conn, rw, err := w.ResponseWriter.Hijack()
	if err != nil {
		return conn, rw, err
	}
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
				c.writer.capture.mu.Lock()
				exists := c.writer.capture.deliveryPrepared
				c.writer.capture.deliveryPrepared = true
				c.writer.capture.mu.Unlock()
				if !exists {
					record := c.writer.capture.newRecord("client_delivery", EvidenceObjectMeta{StatusCode: resp.StatusCode, Headers: resp.Header}, true)
					c.writer.capture.mu.Lock()
					c.writer.capture.delivery = record
					c.writer.capture.mu.Unlock()
				}
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
