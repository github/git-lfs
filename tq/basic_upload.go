package tq

import (
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/git-lfs/git-lfs/config"
	"github.com/git-lfs/git-lfs/errors"
	"github.com/git-lfs/git-lfs/lfsapi"
	"github.com/git-lfs/git-lfs/tools"
)

const (
	BasicAdapterName   = "basic"
	defaultContentType = "application/octet-stream"
)

// Adapter for basic uploads (non resumable)
type basicUploadAdapter struct {
	*adapterBase
}

func (a *basicUploadAdapter) ClearTempStorage() error {
	// Should be empty already but also remove dir
	return os.RemoveAll(a.tempDir())
}

func (a *basicUploadAdapter) tempDir() string {
	// Must be dedicated to this adapter as deleted by ClearTempStorage
	d := filepath.Join(os.TempDir(), "git-lfs-basic-temp")
	if err := tools.MkdirAll(d, a.fs); err != nil {
		return os.TempDir()
	}
	return d
}

func (a *basicUploadAdapter) WorkerStarting(workerNum int) (interface{}, error) {
	return nil, nil
}
func (a *basicUploadAdapter) WorkerEnding(workerNum int, ctx interface{}) {
}

func (a *basicUploadAdapter) DoTransfer(ctx interface{}, t *Transfer, cb ProgressCallback, authOkFunc func()) error {
	rel, err := t.Rel("upload")
	if err != nil {
		return err
	}
	if rel == nil {
		return errors.Errorf("No upload action for object: %s", t.Oid)
	}

	req, err := a.newHTTPRequest("PUT", rel)
	if err != nil {
		return err
	}

	if req.Header.Get("Transfer-Encoding") == "chunked" {
		req.TransferEncoding = []string{"chunked"}
	} else {
		req.Header.Set("Content-Length", strconv.FormatInt(t.Size, 10))
	}

	req.ContentLength = t.Size

	f, err := os.OpenFile(t.Path, os.O_RDONLY, 0644)
	if err != nil {
		return errors.Wrap(err, "basic upload")
	}
	defer f.Close()

	if err := a.setContentTypeFor(req, f); err != nil {
		return err
	}

	// Ensure progress callbacks made while uploading
	// Wrap callback to give name context
	ccb := func(totalSize int64, readSoFar int64, readSinceLast int) error {
		if cb != nil {
			return cb(t.Name, totalSize, readSoFar, readSinceLast)
		}
		return nil
	}

	cbr := tools.NewFileBodyWithCallback(f, t.Size, ccb)
	var reader lfsapi.ReadSeekCloser = cbr

	// Signal auth was ok on first read; this frees up other workers to start
	if authOkFunc != nil {
		reader = newStartCallbackReader(reader, func() error {
			authOkFunc()
			return nil
		})
	}

	req.Body = reader

	req = a.apiClient.LogRequest(req, "lfs.data.upload")
	res, err := a.makeRequest(t, req)
	if err != nil {
		if errors.IsUnprocessableEntityError(err) {
			// If we got an HTTP 422, we do _not_ want to retry the
			// request later below, because it is likely that the
			// implementing server does not support non-standard
			// Content-Type headers.
			//
			// Instead, return immediately and wait for the
			// *tq.TransferQueue to report an error message.
			return err
		}

		// We're about to return a retriable error, meaning that this
		// transfer will either be retried, or it will fail.
		//
		// Either way, let's decrement the number of bytes that we've
		// read _so far_, so that the next iteration doesn't re-transfer
		// those bytes, according to the progress meter.
		if perr := cbr.ResetProgress(); perr != nil {
			err = errors.Wrap(err, perr.Error())
		}

		if res == nil {
			// We encountered a network or similar error which caused us
			// to not receive a response at all.
			return errors.NewRetriableError(err)
		}

		if res.StatusCode == 429 {
			retLaterErr := errors.NewRetriableLaterError(err, res.Header.Get("Retry-After"))
			if retLaterErr != nil {
				return retLaterErr
			}
		}
		return errors.NewRetriableError(err)
	}

	// A status code of 403 likely means that an authentication token for the
	// upload has expired. This can be safely retried.
	if res.StatusCode == 403 {
		err = errors.New("http: received status 403")
		return errors.NewRetriableError(err)
	}

	if res.StatusCode > 299 {
		return errors.Wrapf(nil, "Invalid status for %s %s: %d",
			req.Method,
			strings.SplitN(req.URL.String(), "?", 2)[0],
			res.StatusCode,
		)
	}

	io.Copy(ioutil.Discard, res.Body)
	res.Body.Close()

	return verifyUpload(a.apiClient, a.remote, t)
}

func (a *adapterBase) setContentTypeFor(req *http.Request, r io.ReadSeeker) error {
	uc := config.NewURLConfig(a.apiClient.GitEnv())
	disabled := !uc.Bool("lfs", req.URL.String(), "contenttype", true)
	if len(req.Header.Get("Content-Type")) != 0 {
		return nil
	}

	var contentType string

	if !disabled {
		buffer := make([]byte, 512)
		n, err := r.Read(buffer)
		if err != nil && err != io.EOF {
			return errors.Wrap(err, "content type detect")
		}

		contentType = http.DetectContentType(buffer[:n])
		if _, err := r.Seek(0, io.SeekStart); err != nil {
			return errors.Wrap(err, "content type rewind")
		}
	}

	if contentType == "" {
		contentType = defaultContentType
	}

	req.Header.Set("Content-Type", contentType)
	return nil
}

// startCallbackReader is a reader wrapper which calls a function as soon as the
// first Read() call is made. This callback is only made once
type startCallbackReader struct {
	cb     func() error
	cbDone bool
	lfsapi.ReadSeekCloser
}

func (s *startCallbackReader) Read(p []byte) (n int, err error) {
	if !s.cbDone && s.cb != nil {
		if err := s.cb(); err != nil {
			return 0, err
		}
		s.cbDone = true
	}
	return s.ReadSeekCloser.Read(p)
}
func newStartCallbackReader(r lfsapi.ReadSeekCloser, cb func() error) *startCallbackReader {
	return &startCallbackReader{
		ReadSeekCloser: r,
		cb:             cb,
	}
}

func configureBasicUploadAdapter(m *Manifest) {
	m.RegisterNewAdapterFunc(BasicAdapterName, Upload, func(name string, dir Direction) Adapter {
		switch dir {
		case Upload:
			bu := &basicUploadAdapter{newAdapterBase(m.fs, name, dir, nil)}
			// self implements impl
			bu.transferImpl = bu
			return bu
		case Download:
			panic("Should never ask this func for basic download")
		}
		return nil
	})
}

func (a *basicUploadAdapter) makeRequest(t *Transfer, req *http.Request) (*http.Response, error) {
	res, err := a.doHTTP(t, req)
	if errors.IsAuthError(err) && len(req.Header.Get("Authorization")) == 0 {
		// Construct a new body with just the raw file and no callbacks. Since
		// all progress tracking happens when the net.http code copies our
		// request body into a new request, we can safely make this request
		// outside of the flow of the transfer adapter, and if it fails, the
		// transfer progress will be rewound at the top level
		f, _ := os.OpenFile(t.Path, os.O_RDONLY, 0644)
		defer f.Close()

		req.Body = tools.NewBodyWithCallback(f, t.Size, nil)
		return a.makeRequest(t, req)
	}

	return res, err
}
