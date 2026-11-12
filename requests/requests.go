// Package requests is a Go port of the ergonomics of the popular Python
// `requests` library, built on top of net/http.
//
// Basic usage:
//
//	import requests "github.com/roseboy/private-gotool/requests"
//
//	r, err := requests.Get("https://api.github.com/repos/psf/requests",
//		requests.Params(map[string]any{"per_page": 5}),
//		requests.Headers(map[string]string{"Accept": "application/json"}),
//	)
//	if err != nil {
//		log.Fatal(err)
//	}
//	fmt.Println(r.StatusCode) // 200
//	fmt.Println(r.Text())     // body as string
//	body, _ := r.JSON()       // parsed JSON
//	if err := r.RaiseForStatus(); err != nil {
//		log.Fatal(err)
//	}
//
// Sessions persist cookies and headers across requests:
//
//	s := requests.NewSession()
//	s.Headers.Set("User-Agent", "my-agent")
//	r, err := s.Post("https://example.com/login",
//		requests.Data(map[string]string{"user": "a", "pass": "b"}),
//	)
//
// The functional options mirror requests' keyword arguments: Params, Headers,
// Data, JSON, Files, Cookies, Auth, Timeout, ConnectTimeout, ReadTimeout,
// AllowRedirects, Proxies, Verify, Stream, Hooks, Client and MaxRedirects.
package requests

import (
	"fmt"
	"net/http"
	"strings"
	"time"
)

// Version is the library version string.
const Version = "1.0.0"

const defaultUserAgent = "requests.go/" + Version

// Option mutates a Request before it is prepared. Options mirror the keyword
// arguments accepted by requests.request.
type Option func(*Request) error

// Params sets the URL query parameters. Accepted types mirror requests' params:
// map[string]string, map[string][]string, map[string]any, url.Values,
// []Param (order-preserving), a pre-encoded string, or nil.
func Params(p any) Option {
	return func(r *Request) error {
		r.Params = p
		return nil
	}
}

// Headers merges request headers into the existing header dict.
func Headers(h any) Option {
	return func(r *Request) error {
		return mergeHeaders(r.Headers, h)
	}
}

// Data sets the request body as form data (maps) or raw data (string/bytes).
func Data(d any) Option {
	return func(r *Request) error {
		r.Data = d
		return nil
	}
}

// JSON sets a JSON-encoded request body.
func JSON(v any) Option {
	return func(r *Request) error {
		r.JSON = v
		return nil
	}
}

// Files sets multipart file uploads. See the File type.
func Files(f any) Option {
	return func(r *Request) error {
		r.Files = f
		return nil
	}
}

// Cookies sets per-request cookies (map[string]string, *http.Cookie or
// []*http.Cookie).
func Cookies(c any) Option {
	return func(r *Request) error {
		r.Cookies = c
		return nil
	}
}

// Auth sets the request authenticator.
func Auth(a Authenticator) Option {
	return func(r *Request) error {
		r.Auth = a
		return nil
	}
}

// Timeout sets the total request timeout.
func Timeout(d time.Duration) Option {
	return func(r *Request) error {
		r.Timeout = d
		return nil
	}
}

// ConnectTimeout sets the connection establishment timeout.
func ConnectTimeout(d time.Duration) Option {
	return func(r *Request) error {
		r.ConnectTimeout = d
		return nil
	}
}

// ReadTimeout sets the response read timeout.
func ReadTimeout(d time.Duration) Option {
	return func(r *Request) error {
		r.ReadTimeout = d
		return nil
	}
}

// AllowRedirects enables or disables following redirects.
func AllowRedirects(b bool) Option {
	return func(r *Request) error {
		r.AllowRedirects = b
		return nil
	}
}

// Proxies sets proxy URLs by scheme ("http", "https", "all", "no_proxy").
func Proxies(p map[string]string) Option {
	return func(r *Request) error {
		r.Proxies = p
		return nil
	}
}

// Verify controls TLS certificate verification. Pass a bool, or a string path
// to a custom CA bundle.
func Verify(v any) Option {
	return func(r *Request) error {
		switch t := v.(type) {
		case bool:
			r.Verify = t
		case string:
			r.CACert = t
			r.Verify = true
		default:
			return fmt.Errorf("unsupported verify type %T (want bool or string)", v)
		}
		return nil
	}
}

// Stream keeps the response body open for streaming when true.
func Stream(b bool) Option {
	return func(r *Request) error {
		r.Stream = b
		return nil
	}
}

// Hooks registers response hooks.
func Hooks(h map[string][]Hook) Option {
	return func(r *Request) error {
		if r.Hooks == nil {
			r.Hooks = map[string][]Hook{}
		}
		for k, v := range h {
			r.Hooks[k] = append(r.Hooks[k], v...)
		}
		return nil
	}
}

// Client overrides the underlying *http.Client for this request.
func Client(c *http.Client) Option {
	return func(r *Request) error {
		r.Client = c
		return nil
	}
}

// MaxRedirects sets the maximum number of redirects to follow.
func MaxRedirects(n int) Option {
	return func(r *Request) error {
		r.MaxRedirects = n
		return nil
	}
}

// mergeHeaders merges a headers-like value into dst.
func mergeHeaders(dst *CaseInsensitiveDict, src any) error {
	switch h := src.(type) {
	case nil:
		return nil
	case map[string]string:
		for k, v := range h {
			dst.Set(k, v)
		}
	case map[string][]string:
		for k, vs := range h {
			dst.Set(k, strings.Join(vs, ", "))
		}
	case http.Header:
		for k, vs := range h {
			dst.Set(k, strings.Join(vs, ", "))
		}
	case *CaseInsensitiveDict:
		for k, vs := range h.Items() {
			dst.Set(k, strings.Join(vs, ", "))
		}
	default:
		return fmt.Errorf("unsupported headers type %T", src)
	}
	return nil
}

// NewRequest creates a Request with defaults applied and the given options set.
// Use it together with Session.PrepareRequest / Session.Send for the
// prepare/send workflow.
func NewRequest(method, urlStr string, opts ...Option) *Request {
	req := &Request{
		Method:         method,
		URL:            urlStr,
		Headers:        NewCaseInsensitiveDict(),
		Verify:         true,
		AllowRedirects: true,
		MaxRedirects:   30,
	}
	for _, opt := range opts {
		if opt != nil {
			_ = opt(req)
		}
	}
	return req
}

// Do performs a request of the given method using a fresh Session. It is the
// equivalent of requests.request(method, url, **kwargs).
func Do(method, urlStr string, opts ...Option) (*Response, error) {
	return NewSession().Request(method, urlStr, opts...)
}

// Get sends a GET request.
func Get(urlStr string, opts ...Option) (*Response, error) {
	return Do("GET", urlStr, opts...)
}

// Post sends a POST request.
func Post(urlStr string, opts ...Option) (*Response, error) {
	return Do("POST", urlStr, opts...)
}

// Put sends a PUT request.
func Put(urlStr string, opts ...Option) (*Response, error) {
	return Do("PUT", urlStr, opts...)
}

// Delete sends a DELETE request.
func Delete(urlStr string, opts ...Option) (*Response, error) {
	return Do("DELETE", urlStr, opts...)
}

// Head sends a HEAD request.
func Head(urlStr string, opts ...Option) (*Response, error) {
	return Do("HEAD", urlStr, opts...)
}

// Options sends an OPTIONS request.
func Options(urlStr string, opts ...Option) (*Response, error) {
	return Do("OPTIONS", urlStr, opts...)
}

// Patch sends a PATCH request.
func Patch(urlStr string, opts ...Option) (*Response, error) {
	return Do("PATCH", urlStr, opts...)
}
