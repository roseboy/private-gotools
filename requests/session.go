package requests

import (
	"crypto/tls"
	"crypto/x509"
	"errors"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/roseboy/private-gotool/requests/exceptions"
)

// Session keeps cookies, headers, authentication and connection settings that
// persist across requests, mirroring requests.Session.
//
// A Session is not safe for concurrent use by multiple goroutines.
type Session struct {
	// Headers are merged into every request.
	Headers *CaseInsensitiveDict
	// Cookies is the cookie jar shared across requests.
	Cookies *CookieJar
	// Auth is applied to every request unless overridden per request.
	Auth Authenticator
	// Proxies maps schemes ("http", "https", "all") to proxy URLs.
	Proxies map[string]string
	// Verify enables TLS certificate verification. When false, self-signed
	// certificates are accepted.
	Verify bool
	// CACert is an optional path to a PEM CA bundle.
	CACert string
	// Stream keeps response bodies open for streaming when true.
	Stream bool
	// Timeout is the total request timeout.
	Timeout time.Duration
	// ConnectTimeout limits the time to establish a connection.
	ConnectTimeout time.Duration
	// ReadTimeout limits the time waiting for response data.
	ReadTimeout time.Duration
	// MaxRedirects is the maximum number of redirects to follow.
	MaxRedirects int
	// Hooks are response hooks invoked for every request.
	Hooks map[string][]Hook
	// Client overrides the underlying *http.Client when set.
	Client *http.Client

	transport *http.Transport
}

// NewSession returns a Session with sensible defaults.
func NewSession() *Session {
	return &Session{
		Headers:      NewCaseInsensitiveDict(),
		Cookies:      NewCookieJar(),
		Verify:       true,
		MaxRedirects: 30,
	}
}

// Request performs a request of the given method, mirroring
// requests.Session.request(method, url, **kwargs).
func (s *Session) Request(method, urlStr string, opts ...Option) (*Response, error) {
	req := s.newRequest(method, urlStr)
	for _, opt := range opts {
		if opt == nil {
			continue
		}
		if err := opt(req); err != nil {
			return nil, err
		}
	}
	prep, err := req.Prepare()
	if err != nil {
		return nil, err
	}
	prep.jar = s.Cookies
	return s.Send(prep)
}

// newRequest builds a Request pre-populated with the session's defaults.
func (s *Session) newRequest(method, urlStr string) *Request {
	return &Request{
		Method:         method,
		URL:            urlStr,
		Headers:        s.Headers.Clone(),
		Auth:           s.Auth,
		Proxies:        s.Proxies,
		Verify:         s.Verify,
		CACert:         s.CACert,
		Stream:         s.Stream,
		Timeout:        s.Timeout,
		ConnectTimeout: s.ConnectTimeout,
		ReadTimeout:    s.ReadTimeout,
		MaxRedirects:   s.MaxRedirects,
		AllowRedirects: true,
		Hooks:          s.Hooks,
		Client:         s.Client,
	}
}

// Get sends a GET request.
func (s *Session) Get(urlStr string, opts ...Option) (*Response, error) {
	return s.Request("GET", urlStr, opts...)
}

// Post sends a POST request.
func (s *Session) Post(urlStr string, opts ...Option) (*Response, error) {
	return s.Request("POST", urlStr, opts...)
}

// Put sends a PUT request.
func (s *Session) Put(urlStr string, opts ...Option) (*Response, error) {
	return s.Request("PUT", urlStr, opts...)
}

// Delete sends a DELETE request.
func (s *Session) Delete(urlStr string, opts ...Option) (*Response, error) {
	return s.Request("DELETE", urlStr, opts...)
}

// Head sends a HEAD request.
func (s *Session) Head(urlStr string, opts ...Option) (*Response, error) {
	return s.Request("HEAD", urlStr, opts...)
}

// Options sends an OPTIONS request.
func (s *Session) Options(urlStr string, opts ...Option) (*Response, error) {
	return s.Request("OPTIONS", urlStr, opts...)
}

// Patch sends a PATCH request.
func (s *Session) Patch(urlStr string, opts ...Option) (*Response, error) {
	return s.Request("PATCH", urlStr, opts...)
}

// PrepareRequest prepares a Request for sending, binding it to this session's
// cookie jar.
func (s *Session) PrepareRequest(req *Request) (*PreparedRequest, error) {
	prep, err := req.Prepare()
	if err != nil {
		return nil, err
	}
	prep.jar = s.Cookies
	return prep, nil
}

// Send performs a prepared request and returns the response.
func (s *Session) Send(prep *PreparedRequest) (*Response, error) {
	maxRedirects := prep.MaxRedirects
	if maxRedirects <= 0 {
		maxRedirects = 30
	}

	current := prep
	var history []*Response

	for redirectCount := 0; ; redirectCount++ {
		req, err := current.toHTTPRequest()
		if err != nil {
			return nil, err
		}

		client := s.clientFor(current)
		start := time.Now()
		resp, err := client.Do(req)
		elapsed := time.Since(start)
		if err != nil {
			return nil, wrapTransportError(err)
		}

		r := newResponse(resp, current, elapsed)

		if !isRedirect(resp.StatusCode) || !prep.AllowRedirects {
			r.History = history
			s.runHooks(prep, r, nil)
			return r, nil
		}

		if redirectCount >= maxRedirects {
			r.Close()
			return nil, &exceptions.TooManyRedirects{
				RequestError: exceptions.RequestError{Message: "Exceeded maximum redirects"},
				Response:     r,
			}
		}

		location := resp.Header.Get("Location")
		if location == "" {
			r.History = history
			s.runHooks(prep, r, nil)
			return r, nil
		}

		nextURL := resolveLocation(current.URL, location)
		if nextURL == nil {
			r.Close()
			return nil, &exceptions.InvalidURL{RequestError: exceptions.RequestError{Message: "Invalid redirect URL: " + location}}
		}

		history = append(history, r)
		r.Close()
		current = current.cloneForRedirect(nextURL, resp.StatusCode)
	}
}

// runHooks dispatches response hooks.
func (s *Session) runHooks(prep *PreparedRequest, r *Response, err error) {
	for _, hook := range prep.Hooks["response"] {
		hook(r, err)
	}
}

// clientFor returns the *http.Client to use, ensuring manual redirect handling
// is enabled regardless of the client's own configuration.
func (s *Session) clientFor(prep *PreparedRequest) *http.Client {
	var client *http.Client
	switch {
	case prep.Client != nil:
		client = prep.Client
	case s.Client != nil:
		client = s.Client
	default:
		client = &http.Client{
			Transport:     s.transportFor(prep),
			Timeout:       prep.Timeout,
			Jar:           s.cookieJarFor(prep),
			CheckRedirect: func(req *http.Request, via []*http.Request) error { return http.ErrUseLastResponse },
		}
	}

	// Shallow copy so we do not mutate the caller's client, and force manual
	// redirect handling so our history/max-redirect logic stays in control.
	cc := *client
	cc.CheckRedirect = func(req *http.Request, via []*http.Request) error { return http.ErrUseLastResponse }
	return &cc
}

// cookieJarFor resolves the cookie jar to attach to the client.
func (s *Session) cookieJarFor(prep *PreparedRequest) http.CookieJar {
	jar := prep.jar
	if jar == nil {
		jar = s.Cookies
	}
	if jar == nil || jar.jar == nil {
		return nil
	}
	return jar.jar
}

// transportFor builds an *http.Transport honoring proxy, TLS and dial settings.
func (s *Session) transportFor(prep *PreparedRequest) *http.Transport {
	if s.transport != nil {
		return s.transport
	}
	dialer := &net.Dialer{}
	if prep.ConnectTimeout > 0 {
		dialer.Timeout = prep.ConnectTimeout
	}

	tr := &http.Transport{
		Proxy:             proxyFunc(prep.Proxies),
		DialContext:       dialer.DialContext,
		ForceAttemptHTTP2: true,
	}

	tlsCfg := &tls.Config{}
	if !prep.Verify {
		tlsCfg.InsecureSkipVerify = true
	} else if prep.CACert != "" {
		if ca, err := os.ReadFile(prep.CACert); err == nil {
			pool := x509.NewCertPool()
			pool.AppendCertsFromPEM(ca)
			tlsCfg.RootCAs = pool
		}
	}
	tr.TLSClientConfig = tlsCfg
	return tr
}

// Close closes idle connections. Sessions may be reused after Close.
func (s *Session) Close() error {
	if s.transport != nil {
		s.transport.CloseIdleConnections()
	}
	return nil
}

// resolveLocation resolves a redirect Location against the current URL.
func resolveLocation(current *url.URL, location string) *url.URL {
	ref, err := url.Parse(location)
	if err != nil {
		return nil
	}
	return current.ResolveReference(ref)
}

// proxyFunc builds a Transport proxy function from a proxies map.
func proxyFunc(proxies map[string]string) func(*http.Request) (*url.URL, error) {
	if len(proxies) == 0 {
		return http.ProxyFromEnvironment
	}
	noProxy := proxies["no_proxy"]
	return func(req *http.Request) (*url.URL, error) {
		host := req.URL.Hostname()
		if host != "" && matchesNoProxy(host, noProxy) {
			return nil, nil
		}
		raw := proxies[req.URL.Scheme]
		if raw == "" {
			raw = proxies["all"]
		}
		if raw == "" {
			return nil, nil
		}
		return url.Parse(raw)
	}
}

// wrapTransportError converts a net/http error into the matching exception.
func wrapTransportError(err error) error {
	var ue *url.Error
	if errors.As(err, &ue) {
		if ue.Timeout() {
			return &exceptions.Timeout{RequestError: exceptions.RequestError{Message: "Request timed out", Cause: err}}
		}
		err = ue.Err
	}

	var ne net.Error
	if errors.As(err, &ne) && ne.Timeout() {
		return &exceptions.Timeout{RequestError: exceptions.RequestError{Message: "Request timed out", Cause: err}}
	}

	if isSSLError(err) {
		return &exceptions.SSLError{RequestError: exceptions.RequestError{Message: "SSL certificate verification failed", Cause: err}}
	}

	if strings.Contains(err.Error(), "proxy") {
		return &exceptions.ProxyError{RequestError: exceptions.RequestError{Message: "Proxy error", Cause: err}}
	}

	return &exceptions.ConnectionError{RequestError: exceptions.RequestError{Message: "Connection error", Cause: err}}
}

// isSSLError reports whether err is a TLS/certificate verification error.
func isSSLError(err error) bool {
	var certErr *tls.CertificateVerificationError
	if errors.As(err, &certErr) {
		return true
	}
	var ua x509.UnknownAuthorityError
	if errors.As(err, &ua) {
		return true
	}
	var hn x509.HostnameError
	if errors.As(err, &hn) {
		return true
	}
	var ci x509.CertificateInvalidError
	if errors.As(err, &ci) {
		return true
	}
	var roots x509.SystemRootsError
	if errors.As(err, &roots) {
		return true
	}
	return false
}
