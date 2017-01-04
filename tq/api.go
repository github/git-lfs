package tq

import (
	"net/http"
	"strings"

	"github.com/git-lfs/git-lfs/api"
	"github.com/git-lfs/git-lfs/errors"
	"github.com/git-lfs/git-lfs/lfsapi"
	"github.com/rubyist/tracerx"
)

type tqClient struct {
	*lfsapi.Client
}

type batchRequest struct {
	Operation            string                `json:"operation"`
	Objects              []*api.ObjectResource `json:"objects"`
	TransferAdapterNames []string              `json:"transfers,omitempty"`
}

type batchResponse struct {
	TransferAdapterName string                `json:"transfer"`
	Objects             []*api.ObjectResource `json:"objects"`
}

func (c *tqClient) Batch(remote string, bReq *batchRequest) (*batchResponse, *http.Response, error) {
	bRes := &batchResponse{}
	if len(bReq.Objects) == 0 {
		return bRes, nil, nil
	}

	if len(bReq.TransferAdapterNames) == 1 && bReq.TransferAdapterNames[0] == "basic" {
		bReq.TransferAdapterNames = nil
	}

	e := c.Endpoints.Endpoint(bReq.Operation, remote)
	req, err := c.NewRequest("POST", e, "objects/batch", bReq)
	if err != nil {
		return nil, nil, errors.Wrap(err, "batch request")
	}

	tracerx.Printf("api: batch %d files", len(bReq.Objects))

	res, err := c.DoWithAuth(remote, req)
	if err != nil {
		tracerx.Printf("api error: %s", err)
		return nil, nil, errors.Wrap(err, "batch response")
	}
	c.LogResponse("lfs.batch", res)

	if res.StatusCode != 200 {
		return nil, res, errors.Errorf("Invalid status for %s %s: %d",
			req.Method,
			strings.SplitN(req.URL.String(), "?", 2)[0],
			res.StatusCode)
	}

	return bRes, res, lfsapi.DecodeJSON(res, bRes)
}