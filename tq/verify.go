package tq

import (
	"fmt"
	"net/http"

	"github.com/git-lfs/git-lfs/lfsapi"
	"github.com/git-lfs/git-lfs/tools"
	"github.com/rubyist/tracerx"
)

const (
	maxVerifiesConfigKey     = "lfs.transfer.maxverifies"
	defaultMaxVerifyAttempts = 3
)

func verifyUpload(c *lfsapi.Client, remote string, t *Transfer) error {
	action, err := t.Actions.Get("verify")
	if err != nil {
		return err
	}
	if action == nil {
		return nil
	}

	req, err := http.NewRequest("POST", action.Href, nil)
	if err != nil {
		return err
	}

	err = lfsapi.MarshalToRequest(req, struct {
		Oid  string `json:"oid"`
		Size int64  `json:"size"`
	}{Oid: t.Oid, Size: t.Size})
	if err != nil {
		return err
	}

	req.Header.Set("Content-Type", "application/vnd.git-lfs+json")
	req.Header.Set("Accept", "application/vnd.git-lfs+json")
	req.Header.Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", t.Name))

	for key, value := range action.Header {
		req.Header.Set(key, value)
	}

	mv := c.GitEnv().Int(maxVerifiesConfigKey, defaultMaxVerifyAttempts)
	mv = tools.MaxInt(defaultMaxVerifyAttempts, mv)
	req = c.LogRequest(req, "lfs.verify")

	for i := 1; i <= mv; i++ {
		tracerx.Printf("tq: verify %s attempt #%d (max: %d)", t.Oid[:7], i, mv)

		var res *http.Response
		if t.Authenticated {
			res, err = c.Do(req)
		} else {
			res, err = c.DoWithAuth(remote, c.Endpoints.AccessFor(action.Href), req)
		}

		if err != nil {
			tracerx.Printf("tq: verify err: %+v", err.Error())
		} else {
			err = res.Body.Close()
			break
		}
	}
	return err
}
