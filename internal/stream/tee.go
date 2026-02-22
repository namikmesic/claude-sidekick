package stream

import (
	"io"
)

// TeeReadCloser splits a stream so that reads flow to both the caller
// and a background pipe. When the caller finishes reading (Close),
// the pipe is closed to signal EOF to the analytics reader.
type TeeReadCloser struct {
	reader io.Reader
	body   io.ReadCloser
	pw     *io.PipeWriter
}

// TeeBody splits an io.ReadCloser into two:
//   - clientReader: the caller reads from this (data also copied to pipe)
//   - analyticsReader: a background consumer reads from this
func TeeBody(body io.ReadCloser) (clientReader *TeeReadCloser, analyticsReader *io.PipeReader) {
	pr, pw := io.Pipe()
	tee := io.TeeReader(body, pw)

	return &TeeReadCloser{
		reader: tee,
		body:   body,
		pw:     pw,
	}, pr
}

func (t *TeeReadCloser) Read(p []byte) (int, error) {
	n, err := t.reader.Read(p)
	if err != nil {
		// On any read error (including EOF), close the pipe writer
		// so the analytics reader also gets EOF.
		t.pw.CloseWithError(err)
	}
	return n, err
}

func (t *TeeReadCloser) Close() error {
	t.pw.Close()
	return t.body.Close()
}
