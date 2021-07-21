package lfshttp

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/textproto"
	"net/url"
	"os"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	spnego "github.com/dpotapov/go-spnego"
	"github.com/git-lfs/git-lfs/config"
	"github.com/git-lfs/git-lfs/creds"
	"github.com/git-lfs/git-lfs/errors"
	"github.com/git-lfs/git-lfs/tools"
	"github.com/rubyist/tracerx"
	"golang.org/x/net/http2"
)

const MediaType = "application/vnd.git-lfs+json; charset=utf-8"

var (
	UserAgent = "git-lfs"
	httpRE    = regexp.MustCompile(`\Ahttps?://`)
)

var hintFileUrl = strings.TrimSpace(`
hint: The remote resolves to a file:// URL, which can only work with a
hint: standalone transfer agent.  See section "Using a Custom Transfer Type
hint: without the API server" in custom-transfers.md for details.
`)

type hostData struct {
	host string
	mode creds.AccessMode
}

type Client struct {
	SSH SSHResolver

	DialTimeout         int
	KeepaliveTimeout    int
	TLSTimeout          int
	ConcurrentTransfers int
	SkipSSLVerify       bool

	Verbose          bool
	DebuggingVerbose bool
	VerboseOut       io.Writer

	hostClients map[hostData]*http.Client
	clientMu    sync.Mutex

	httpLogger *syncLogger

	gitEnv config.Environment
	osEnv  config.Environment
	uc     *config.URLConfig

	credHelperContext *creds.CredentialHelperContext

	sshTries int
}

func NewClient(ctx Context) (*Client, error) {
	if ctx == nil {
		ctx = NewContext(nil, nil, nil)
	}

	gitEnv := ctx.GitEnv()
	osEnv := ctx.OSEnv()

	cacheCreds := gitEnv.Bool("lfs.cachecredentials", true)
	var sshResolver SSHResolver = &sshAuthClient{os: osEnv, git: gitEnv}
	if cacheCreds {
		sshResolver = withSSHCache(sshResolver)
	}

	c := &Client{
		SSH:                 sshResolver,
		DialTimeout:         gitEnv.Int("lfs.dialtimeout", 0),
		KeepaliveTimeout:    gitEnv.Int("lfs.keepalive", 0),
		TLSTimeout:          gitEnv.Int("lfs.tlstimeout", 0),
		ConcurrentTransfers: gitEnv.Int("lfs.concurrenttransfers", 8),
		SkipSSLVerify:       !gitEnv.Bool("http.sslverify", true) || osEnv.Bool("GIT_SSL_NO_VERIFY", false),
		Verbose:             osEnv.Bool("GIT_CURL_VERBOSE", false),
		DebuggingVerbose:    osEnv.Bool("LFS_DEBUG_HTTP", false),
		gitEnv:              gitEnv,
		osEnv:               osEnv,
		uc:                  config.NewURLConfig(gitEnv),
		sshTries:            gitEnv.Int("lfs.ssh.retries", 5),
		credHelperContext:   creds.NewCredentialHelperContext(gitEnv, osEnv),
	}

	return c, nil
}

func (c *Client) GitEnv() config.Environment {
	return c.gitEnv
}

func (c *Client) OSEnv() config.Environment {
	return c.osEnv
}

func (c *Client) URLConfig() *config.URLConfig {
	return c.uc
}

func (c *Client) NewRequest(method string, e Endpoint, suffix string, body interface{}) (*http.Request, error) {
	if strings.HasPrefix(e.Url, "file://") {
		// Initial `\n` to avoid overprinting `Downloading LFS...`.
		fmt.Fprintf(os.Stderr, "\n%s\n", hintFileUrl)
	}

	sshRes, err := c.sshResolveWithRetries(e, method)
	if err != nil {
		return nil, err
	}

	prefix := e.Url
	if len(sshRes.Href) > 0 {
		prefix = sshRes.Href
	}

	if !httpRE.MatchString(prefix) {
		urlfragment := strings.SplitN(prefix, "?", 2)[0]
		return nil, fmt.Errorf("missing protocol: %q", urlfragment)
	}

	req, err := http.NewRequest(method, joinURL(prefix, suffix), nil)
	if err != nil {
		return req, err
	}

	for key, value := range sshRes.Header {
		req.Header.Set(key, value)
	}
	req.Header.Set("Accept", MediaType)

	if body != nil {
		if merr := MarshalToRequest(req, body); merr != nil {
			return req, merr
		}
		req.Header.Set("Content-Type", MediaType)
	}

	return req, err
}

const slash = "/"

func joinURL(prefix, suffix string) string {
	if strings.HasSuffix(prefix, slash) {
		return prefix + suffix
	}
	return prefix + slash + suffix
}

// Do sends an HTTP request to get an HTTP response. It wraps net/http, adding
// extra headers, redirection handling, and error reporting.
func (c *Client) Do(req *http.Request) (*http.Response, error) {
	req.Header = c.ExtraHeadersFor(req)

	return c.do(req, "", nil, creds.NoneAccess)
}

// DoWithAccess sends an HTTP request to get an HTTP response using the
// specified access mode. It wraps net/http, adding extra headers, redirection
// handling, and error reporting.
func (c *Client) DoWithAccess(req *http.Request, mode creds.AccessMode) (*http.Response, error) {
	req.Header = c.ExtraHeadersFor(req)

	return c.do(req, "", nil, mode)
}

// do performs an *http.Request respecting redirects, and handles the response
// as defined in c.handleResponse. Notably, it does not alter the headers for
// the request argument in any way.
func (c *Client) do(req *http.Request, remote string, via []*http.Request, mode creds.AccessMode) (*http.Response, error) {
	req.Header.Set("User-Agent", UserAgent)

	client, err := c.HttpClient(req.URL, mode)
	if err != nil {
		return nil, err
	}

	res, err := c.doWithRedirects(client, req, remote, via)
	if err != nil {
		return res, err
	}

	return res, c.handleResponse(res)
}

// Close closes any resources that this client opened.
func (c *Client) Close() error {
	return c.httpLogger.Close()
}

func (c *Client) sshResolveWithRetries(e Endpoint, method string) (*sshAuthResponse, error) {
	var sshRes sshAuthResponse
	var err error

	requests := tools.MaxInt(0, c.sshTries) + 1
	for i := 0; i < requests; i++ {
		sshRes, err = c.SSH.Resolve(e, method)
		if err == nil {
			return &sshRes, nil
		}

		tracerx.Printf(
			"ssh: %s failed, error: %s, message: %s (try: %d/%d)",
			e.SSHMetadata.UserAndHost, err.Error(), sshRes.Message, i,
			requests,
		)
	}

	if len(sshRes.Message) > 0 {
		return nil, errors.Wrap(err, sshRes.Message)
	}
	return nil, err
}

func (c *Client) ExtraHeadersFor(req *http.Request) http.Header {
	extraHeaders := c.extraHeaders(req.URL)
	if len(extraHeaders) == 0 {
		return req.Header
	}

	copy := make(http.Header, len(req.Header))
	for k, vs := range req.Header {
		copy[k] = vs
	}

	for k, vs := range extraHeaders {
		if len(req.Header.Get(k)) == 0 {
			for _, v := range vs {
				copy[k] = append(copy[k], v)
			}
		}
	}
	return copy
}

func (c *Client) extraHeaders(u *url.URL) map[string][]string {
	hdrs := c.uc.GetAll("http", u.String(), "extraHeader")
	m := make(map[string][]string, len(hdrs))

	for _, hdr := range hdrs {
		parts := strings.SplitN(hdr, ":", 2)
		if len(parts) < 2 {
			continue
		}

		k, v := parts[0], strings.TrimSpace(parts[1])
		// If header keys are given in non-canonicalized form (e.g.,
		// "AUTHORIZATION" as opposed to "Authorization") they will not
		// be returned in calls to net/http.Header.Get().
		//
		// So, we avoid this problem by first canonicalizing header keys
		// for extra headers.
		k = textproto.CanonicalMIMEHeaderKey(k)

		m[k] = append(m[k], v)
	}
	return m
}

func (c *Client) DoWithRedirect(cli *http.Client, req *http.Request, remote string, via []*http.Request) (*http.Request, *http.Response, error) {
	tracedReq, err := c.traceRequest(req)
	if err != nil {
		return nil, nil, err
	}

	var retries int
	if n, ok := Retries(req); ok {
		retries = n
	} else {
		retries = defaultRequestRetries
	}

	var res *http.Response

	requests := tools.MaxInt(0, retries) + 1
	for i := 0; i < requests; i++ {
		res, err = cli.Do(req)
		if err == nil {
			break
		}

		if seek, ok := req.Body.(io.Seeker); ok {
			seek.Seek(0, io.SeekStart)
		}

		c.traceResponse(req, tracedReq, nil)
	}

	if err != nil {
		c.traceResponse(req, tracedReq, nil)
		return nil, nil, err
	}

	if res == nil {
		return nil, nil, nil
	}

	if res.Uncompressed {
		tracerx.Printf("http: decompressed gzipped response")
	}

	c.traceResponse(req, tracedReq, res)

	if res.StatusCode != 301 &&
		res.StatusCode != 302 &&
		res.StatusCode != 303 &&
		res.StatusCode != 307 &&
		res.StatusCode != 308 {

		// Above are the list of 3xx status codes that we know
		// how to handle below. If the status code contained in
		// the HTTP response was none of them, return the (res,
		// err) tuple as-is, otherwise handle the redirect.
		return nil, res, c.handleResponse(res)
	}

	redirectTo := res.Header.Get("Location")
	locurl, err := url.Parse(redirectTo)
	if err == nil && !locurl.IsAbs() {
		locurl = req.URL.ResolveReference(locurl)
		redirectTo = locurl.String()
	}

	via = append(via, req)
	if len(via) >= 3 {
		return nil, res, errors.New("too many redirects")
	}

	redirectedReq, err := newRequestForRetry(req, redirectTo)
	if err != nil {
		return nil, res, err
	}

	res.Body.Close()

	return redirectedReq, nil, nil
}

func (c *Client) doWithRedirects(cli *http.Client, req *http.Request, remote string, via []*http.Request) (*http.Response, error) {
	redirectedReq, res, err := c.DoWithRedirect(cli, req, remote, via)
	if err != nil || res != nil {
		return res, err
	}

	if redirectedReq == nil {
		return nil, errors.New("failed to redirect request")
	}

	return c.doWithRedirects(cli, redirectedReq, remote, via)
}

func (c *Client) configureProtocols(u *url.URL, tr *http.Transport) error {
	version, _ := c.uc.Get("http", u.String(), "version")
	switch version {
	case "HTTP/1.1":
		// This disables HTTP/2, according to the documentation.
		tr.TLSNextProto = make(map[string]func(authority string, c *tls.Conn) http.RoundTripper)
	case "HTTP/2":
		if u.Scheme != "https" {
			return fmt.Errorf("HTTP/2 cannot be used except with TLS")
		}
		http2.ConfigureTransport(tr)
		delete(tr.TLSNextProto, "http/1.1")
	case "":
		http2.ConfigureTransport(tr)
	default:
		return fmt.Errorf("Unknown HTTP version %q", version)
	}
	return nil
}

func (c *Client) Transport(u *url.URL, access creds.AccessMode) (http.RoundTripper, error) {
	host := u.Host

	if c.gitEnv == nil {
		c.gitEnv = make(testEnv)
	}

	if c.osEnv == nil {
		c.osEnv = make(testEnv)
	}

	concurrentTransfers := c.ConcurrentTransfers
	if concurrentTransfers < 1 {
		concurrentTransfers = 8
	}

	dialtime := c.DialTimeout
	if dialtime < 1 {
		dialtime = 30
	}

	keepalivetime := c.KeepaliveTimeout
	if keepalivetime < 1 {
		keepalivetime = 1800
	}

	tlstime := c.TLSTimeout
	if tlstime < 1 {
		tlstime = 30
	}
	tr := &http.Transport{
		Proxy:               proxyFromClient(c),
		TLSHandshakeTimeout: time.Duration(tlstime) * time.Second,
		MaxIdleConnsPerHost: concurrentTransfers,
	}

	activityTimeout := 30
	if v, ok := c.uc.Get("lfs", u.String(), "activitytimeout"); ok {
		if i, err := strconv.Atoi(v); err == nil {
			activityTimeout = i
		} else {
			activityTimeout = 0
		}
	}

	dialer := &net.Dialer{
		Timeout:   time.Duration(dialtime) * time.Second,
		KeepAlive: time.Duration(keepalivetime) * time.Second,
		DualStack: true,
	}

	if activityTimeout > 0 {
		activityDuration := time.Duration(activityTimeout) * time.Second
		tr.DialContext = func(ctx context.Context, network, addr string) (net.Conn, error) {
			c, err := dialer.DialContext(ctx, network, addr)
			if c == nil {
				return c, err
			}
			if tc, ok := c.(*net.TCPConn); ok {
				tc.SetKeepAlive(true)
				tc.SetKeepAlivePeriod(dialer.KeepAlive)
			}
			return &deadlineConn{Timeout: activityDuration, Conn: c}, err
		}
	} else {
		tr.DialContext = dialer.DialContext
	}

	tr.TLSClientConfig = &tls.Config{
		Renegotiation: tls.RenegotiateFreelyAsClient,
	}

	if isClientCertEnabledForHost(c, host) {
		tracerx.Printf("http: client cert for %s", host)
		cert := getClientCertForHost(c, host)
		if cert != nil {
			tr.TLSClientConfig.Certificates = []tls.Certificate{*cert}
			tr.TLSClientConfig.BuildNameToCertificate()
		}
	}

	if isCertVerificationDisabledForHost(c, host) {
		tr.TLSClientConfig.InsecureSkipVerify = true
	} else {
		tr.TLSClientConfig.RootCAs = getRootCAsForHost(c, host)
	}

	if err := c.configureProtocols(u, tr); err != nil {
		return nil, err
	}

	if access == creds.NegotiateAccess {
		// This technically copies a mutex, but we know since we've just created
		// the object that this mutex is unlocked.
		return &spnego.Transport{Transport: *tr}, nil
	}
	return tr, nil
}

func (c *Client) HttpClient(u *url.URL, access creds.AccessMode) (*http.Client, error) {
	c.clientMu.Lock()
	defer c.clientMu.Unlock()

	host := u.Host

	if c.hostClients == nil {
		c.hostClients = make(map[hostData]*http.Client)
	}

	hd := hostData{host: host, mode: access}

	if client, ok := c.hostClients[hd]; ok {
		return client, nil
	}

	tr, err := c.Transport(u, access)
	if err != nil {
		return nil, err
	}

	httpClient := &http.Client{
		Transport: tr,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}

	if isCookieJarEnabledForHost(c, host) {
		tracerx.Printf("http: cookieFile for %s", host)
		if cookieJar, err := getCookieJarForHost(c, host); err == nil {
			httpClient.Jar = cookieJar
		} else {
			tracerx.Printf("http: error while reading cookieFile: %s", err.Error())
		}
	}

	c.hostClients[hd] = httpClient
	if c.VerboseOut == nil {
		c.VerboseOut = os.Stderr
	}

	return httpClient, nil
}

func (c *Client) CurrentUser() (string, string) {
	userName, _ := c.gitEnv.Get("user.name")
	userEmail, _ := c.gitEnv.Get("user.email")
	return userName, userEmail
}

func newRequestForRetry(req *http.Request, location string) (*http.Request, error) {
	newReq, err := http.NewRequest(req.Method, location, nil)
	if err != nil {
		return nil, err
	}

	if req.URL.Scheme == "https" && newReq.URL.Scheme == "http" {
		return nil, errors.New("lfsapi/client: refusing insecure redirect, https->http")
	}

	sameHost := req.URL.Host == newReq.URL.Host
	for key := range req.Header {
		if key == "Authorization" {
			if !sameHost {
				continue
			}
		}
		newReq.Header.Set(key, req.Header.Get(key))
	}

	oldestURL := strings.SplitN(req.URL.String(), "?", 2)[0]
	newURL := strings.SplitN(newReq.URL.String(), "?", 2)[0]
	tracerx.Printf("api: redirect %s %s to %s", req.Method, oldestURL, newURL)

	// This body will have already been rewound from a call to
	// lfsapi.Client.traceRequest().
	newReq.Body = req.Body
	newReq.ContentLength = req.ContentLength

	// Copy the request's context.Context, if any.
	newReq = newReq.WithContext(req.Context())

	return newReq, nil
}

type deadlineConn struct {
	Timeout time.Duration
	net.Conn
}

func (c *deadlineConn) Read(b []byte) (int, error) {
	if err := c.Conn.SetDeadline(time.Now().Add(c.Timeout)); err != nil {
		return 0, err
	}
	return c.Conn.Read(b)
}

func (c *deadlineConn) Write(b []byte) (int, error) {
	if err := c.Conn.SetDeadline(time.Now().Add(c.Timeout)); err != nil {
		return 0, err
	}

	return c.Conn.Write(b)
}

func init() {
	UserAgent = config.VersionDesc
}

type testEnv map[string]string

func (e testEnv) Get(key string) (v string, ok bool) {
	v, ok = e[key]
	return
}

func (e testEnv) GetAll(key string) []string {
	if v, ok := e.Get(key); ok {
		return []string{v}
	}
	return make([]string, 0)
}

func (e testEnv) Int(key string, def int) int {
	s, _ := e.Get(key)
	return config.Int(s, def)
}

func (e testEnv) Bool(key string, def bool) bool {
	s, _ := e.Get(key)
	return config.Bool(s, def)
}

func (e testEnv) All() map[string][]string {
	m := make(map[string][]string)
	for k, _ := range e {
		m[k] = e.GetAll(k)
	}
	return m
}
