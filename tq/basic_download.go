package tq

import (
	"fmt"
	"hash"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"

	"github.com/git-lfs/git-lfs/errors"
	"github.com/git-lfs/git-lfs/tools"
	"github.com/rubyist/tracerx"
)

// Adapter for basic HTTP downloads, includes resuming via HTTP Range
type basicDownloadAdapter struct {
	*adapterBase
}

func (a *basicDownloadAdapter) ClearTempStorage() error {
	return os.RemoveAll(a.tempDir())
}

func (a *basicDownloadAdapter) tempDir() string {
	// Must be dedicated to this adapter as deleted by ClearTempStorage
	// Also make local to this repo not global, and separate to localstorage temp,
	// which gets cleared at the end of every invocation
	d := filepath.Join(a.fs.LFSStorageDir, "incomplete")
	if err := tools.MkdirAll(d, a.fs); err != nil {
		return os.TempDir()
	}
	return d
}

func (a *basicDownloadAdapter) WorkerStarting(workerNum int) (interface{}, error) {
	return nil, nil
}
func (a *basicDownloadAdapter) WorkerEnding(workerNum int, ctx interface{}) {
}

func (a *basicDownloadAdapter) DoTransfer(ctx interface{}, t *Transfer, cb ProgressCallback, authOkFunc func()) error {
	// Reserve a temporary filename. We need to make sure nobody operates on the file simultaneously with us.
	f, err := tools.TempFile(a.tempDir(), t.Oid, a.fs)
	if err != nil {
		return err
	}
	tmpName := f.Name()
	defer func() {
		// Fail-safe: Most implementation of os.File.Close() does nil check
		if f != nil {
			f.Close()
		}
		// This will delete temp file if:
		// - we failed to fully download file and move it to final location including the case when final location already
		//   exists because other parallel git-lfs processes downloaded file
		// - we also failed to move it to a partially-downloaded location
		os.Remove(tmpName)
	}()

	// Close file because we will attempt to move partially-downloaded one on top of it
	if err := f.Close(); err != nil {
		return err
	}

	// Attempt to resume download. No error checking here. If we fail, we'll simply download from the start
	tools.RobustRename(a.downloadFilename(t), f.Name())

	// Open temp file. It is either empty or partially downloaded
	f, err = os.OpenFile(f.Name(), os.O_RDWR, 0644)
	if err != nil {
		return err
	}

	// Read any existing data into hash
	hash := tools.NewLfsContentHash()
	fromByte, err := io.Copy(hash, f)
	if err != nil {
		return err
	}

	// Ensure that partial file seems valid
	if fromByte > 0 {
		if fromByte < t.Size-1 {
			tracerx.Printf("xfer: Attempting to resume download of %q from byte %d", t.Oid, fromByte)
		} else {
			// Somehow we have more data than expected. Let's retry from the beginning.
			if _, err := f.Seek(0, io.SeekStart); err != nil {
				return err
			}
			if err := f.Truncate(0); err != nil {
				return err
			}
			fromByte = 0
			hash = nil
		}
	}

	err = a.download(t, cb, authOkFunc, f, fromByte, hash)

	if err != nil {
		f.Close()
		// Rename file so next download can resume from where we stopped.
		// No error checking here, if rename fails then file will be deleted and there just will be no download resuming
		tools.RobustRename(f.Name(), a.downloadFilename(t))
	}

	return err
}

// Returns path where partially downloaded file should be stored for download resuming
func (a *basicDownloadAdapter) downloadFilename(t *Transfer) string {
	return filepath.Join(a.tempDir(), t.Oid+".part")
}

// download starts or resumes and download. dlFile is expected to be an existing file open in RW mode
func (a *basicDownloadAdapter) download(t *Transfer, cb ProgressCallback, authOkFunc func(), dlFile *os.File, fromByte int64, hash hash.Hash) error {
	rel, err := t.Rel("download")
	if err != nil {
		return err
	}
	if rel == nil {
		return errors.Errorf("Object %s not found on the server.", t.Oid)
	}

	req, err := a.newHTTPRequest("GET", rel)
	if err != nil {
		return err
	}

	if fromByte > 0 {
		// We could just use a start byte, but since we know the length be specific
		req.Header.Set("Range", fmt.Sprintf("bytes=%d-%d", fromByte, t.Size-1))
	}

	req = a.apiClient.LogRequest(req, "lfs.data.download")
	res, err := a.makeRequest(t, req)
	if err != nil {
		if res == nil {
			// We encountered a network or similar error which caused us
			// to not receive a response at all.
			return errors.NewRetriableError(err)
		}

		// Special-case status code 416 () - fall back
		if fromByte > 0 && dlFile != nil && res.StatusCode == 416 {
			tracerx.Printf("xfer: server rejected resume download request for %q from byte %d; re-downloading from start", t.Oid, fromByte)
			if _, err := dlFile.Seek(0, io.SeekStart); err != nil {
				return err
			}
			if err := dlFile.Truncate(0); err != nil {
				return err
			}
			return a.download(t, cb, authOkFunc, dlFile, 0, nil)
		}

		// Special-cae status code 429 - retry after certain time
		if res.StatusCode == 429 {
			retLaterErr := errors.NewRetriableLaterError(err, res.Header.Get("Retry-After"))
			if retLaterErr != nil {
				return retLaterErr
			}
		}

		return errors.NewRetriableError(err)
	}

	defer res.Body.Close()

	// Range request must return 206 & content range to confirm
	if fromByte > 0 {
		rangeRequestOk := false
		var failReason string
		// check 206 and Content-Range, fall back if either not as expected
		if res.StatusCode == 206 {
			// Probably a successful range request, check Content-Range
			if rangeHdr := res.Header.Get("Content-Range"); rangeHdr != "" {
				regex := regexp.MustCompile(`bytes (\d+)\-.*`)
				match := regex.FindStringSubmatch(rangeHdr)
				if match != nil && len(match) > 1 {
					contentStart, _ := strconv.ParseInt(match[1], 10, 64)
					if contentStart == fromByte {
						rangeRequestOk = true
					} else {
						failReason = fmt.Sprintf("Content-Range start byte incorrect: %s expected %d", match[1], fromByte)
					}
				} else {
					failReason = fmt.Sprintf("badly formatted Content-Range header: %q", rangeHdr)
				}
			} else {
				failReason = "missing Content-Range header in response"
			}
		} else {
			failReason = fmt.Sprintf("expected status code 206, received %d", res.StatusCode)
		}
		if rangeRequestOk {
			tracerx.Printf("xfer: server accepted resume download request: %q from byte %d", t.Oid, fromByte)
			advanceCallbackProgress(cb, t, fromByte)
		} else {
			// Abort resume, perform regular download
			tracerx.Printf("xfer: failed to resume download for %q from byte %d: %s. Re-downloading from start", t.Oid, fromByte, failReason)

			if _, err := dlFile.Seek(0, io.SeekStart); err != nil {
				return err
			}
			if err := dlFile.Truncate(0); err != nil {
				return err
			}
			fromByte = 0
			hash = nil

			if res.StatusCode == 200 {
				// If status code was 200 then server just ignored Range header and
				// sent everything. Don't re-request, use this one from byte 0
			} else {
				// re-request needed
				return a.download(t, cb, authOkFunc, dlFile, fromByte, hash)
			}
		}
	}

	// Signal auth OK on success response, before starting download to free up
	// other workers immediately
	if authOkFunc != nil {
		authOkFunc()
	}

	var hasher *tools.HashingReader
	httpReader := tools.NewRetriableReader(res.Body)

	if fromByte > 0 && hash != nil {
		// pre-load hashing reader with previous content
		hasher = tools.NewHashingReaderPreloadHash(httpReader, hash)
	} else {
		hasher = tools.NewHashingReader(httpReader)
	}

	dlfilename := dlFile.Name()
	// Wrap callback to give name context
	ccb := func(totalSize int64, readSoFar int64, readSinceLast int) error {
		if cb != nil {
			return cb(t.Name, totalSize, readSoFar+fromByte, readSinceLast)
		}
		return nil
	}
	written, err := tools.CopyWithCallback(dlFile, hasher, res.ContentLength, ccb)
	if err != nil {
		return errors.Wrapf(err, "cannot write data to tempfile %q", dlfilename)
	}

	if actual := hasher.Hash(); actual != t.Oid {
		return fmt.Errorf("expected OID %s, got %s after %d bytes written", t.Oid, actual, written)
	}

	if err := dlFile.Close(); err != nil {
		return fmt.Errorf("can't close tempfile %q: %v", dlfilename, err)
	}

	err = tools.RenameFileCopyPermissions(dlfilename, t.Path)
	if _, err2 := os.Stat(t.Path); err2 == nil {
		// Target file already exists, possibly was downloaded by other git-lfs process
		return nil
	}
	return err
}

func configureBasicDownloadAdapter(m *Manifest) {
	m.RegisterNewAdapterFunc(BasicAdapterName, Download, func(name string, dir Direction) Adapter {
		switch dir {
		case Download:
			bd := &basicDownloadAdapter{newAdapterBase(m.fs, name, dir, nil)}
			// self implements impl
			bd.transferImpl = bd
			return bd
		case Upload:
			panic("Should never ask this func to upload")
		}
		return nil
	})
}

func (a *basicDownloadAdapter) makeRequest(t *Transfer, req *http.Request) (*http.Response, error) {
	res, err := a.doHTTP(t, req)
	if errors.IsAuthError(err) && len(req.Header.Get("Authorization")) == 0 {
		return a.makeRequest(t, req)
	}

	return res, err
}
