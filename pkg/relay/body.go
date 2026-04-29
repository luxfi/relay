package relay

import (
	"bytes"
	"io"
)

// makeBody wraps a byte slice as an io.ReadCloser for net/http.
func makeBody(b []byte) io.ReadCloser {
	return io.NopCloser(bytes.NewReader(b))
}
