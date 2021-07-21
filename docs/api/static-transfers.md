# Static Transfer API

The Static transfer API is a non-interactive API for downloading LFS objects.
Its main intent is to provide simple interoperability with existing data
storage solutions by offloading serving LFS objects to them while using
git and git LFS for version management.

Support for it by Git LFS clients and servers is optional.

Git LFS clients MAY implement this transfer API in a fallback manner:
first attempting to download the LFS object using the Static transfer
API and if this fails (e.g., the manifest file does not exist,
oid cannot be found in the manifest file) use the Basic transfer API.

## Downloads

Downloading an object first requires fetching a manifest file from using
HTTP from the remote server. The proposed location of the manifest file
is `https://<remote domain>/.well-known/git-lfs-manifest.json`. The
location is configured using the `lfs.staticurl` configuration option,
or remote-specific `remote.{name}.lfsstaticurl` configuration option.
Both of them can be provided in `.lfsconfig` file as well.

The manifest file MUST contain a JSON payload that looks like this:

```json
{
  "version": "1",
  "transfer": "static",
  "objects": [
    {
      "oid": "1111111",
      "size": 123,
      "authenticated": true,
      "actions": {
        "download": {
          "href": "https://some-download.com/1111111",
          "header": {
            "Authorization": "Basic ..."
          },
          "expires_at": "2016-11-10T15:29:07Z"
        }
      }
    }
  ]
}
```

The remote server SHOULD return together with the contents of the
manifest file also suitable caching response headers (e.g.,
`Cache-Control`, `Pragma`, `Last-Modified`, `ETag`). Moreover, it SHOULD
respond to GET and HEAD HTTP requests and support caching request
headers (e.g., `If-Modified-Since`, `If-None-Match`). Git LFS client
SHOULD attempt to fetch the manifest file again if any of the
authorization tokens expires.

The Static transfer adapter will make a GET request on the `href`, expecting the
raw bytes returned in the HTTP response.

```
> GET https://some-download.com/1111111
> Authorization: Basic ...
<
< HTTP/1.1 200 OK
< Content-Type: application/octet-stream
< Content-Length: 123
<
< {contents}
```

Git LFS clients MAY support non-HTTP `href` URIs (e.g., IPFS, dat). Those URIs
should use dedicated URI schemas to differentiate them from HTTP URIs.

## Uploads

Uploading is not supported with this transfer adapter. Managing uploads and
updating the manifest file accordingly is out of the scope. This enables
one to integrate it with existing data storage solutions in the way which
makes the most sense for a particular data storage solution.
