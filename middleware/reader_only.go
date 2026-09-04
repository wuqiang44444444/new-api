package middleware

import "io"

// readerOnly wraps a seekable BodyStorage as a plain reader so it can be
// assigned to c.Request.Body without the request-cleanup path closing the
// shared storage. Upstream removed common.ReaderOnly in rc.31; the Link
// middlewares keep the same behavior with this package-local helper.
type readerOnly struct{ io.Reader }

func readerOnlyOf(r io.Reader) io.Reader {
	return readerOnly{r}
}
