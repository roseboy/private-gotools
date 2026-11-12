package requests

import (
	"net/http"
	"net/url"
	"time"
)

// Authenticator applies authentication credentials to a PreparedRequest,
// mirroring requests' auth callable protocol.
type Authenticator interface {
	Apply(p *PreparedRequest) error
}

// Hook is a callback invoked after a response is received. Only the "response"
// event is currently honored.
type Hook func(*Response, error)

// Param is a single, order-preserving query or form parameter.
type Param struct {
	Key   string
	Value string
}

// File describes a single multipart file upload.
type File struct {
	// FileName is the name sent in the Content-Disposition header. If empty,
	// it is derived from Path.
	FileName string
	// ContentType is the MIME type of the part.
	ContentType string
	// Content is the raw file content (used when Path is empty).
	Content []byte
	// Path is a local file path to read the content from (takes precedence
	// over Content when set).
	Path string
}

// Request carries the parameters of an HTTP request before it is prepared,
// mirroring requests.Request.
//
// Prefer creating requests via NewRequest or the package-level Get/Post/etc.
// helpers so that boolean defaults (Verify, AllowRedirects) are initialized
// correctly.
type Request struct {
	Method         string
	URL            string
	Params         any
	Data           any
	JSON           any
	Files          any
	Headers        *CaseInsensitiveDict
	Cookies        any
	Auth           Authenticator
	Timeout        time.Duration
	ConnectTimeout time.Duration
	ReadTimeout    time.Duration
	Proxies        map[string]string
	Verify         bool
	CACert         string
	AllowRedirects bool
	Stream         bool
	Hooks          map[string][]Hook
	Client         *http.Client
	MaxRedirects   int
}

// PreparedRequest is a fully prepared request ready to be sent, mirroring
// requests.PreparedRequest.
type PreparedRequest struct {
	Method         string
	URL            *url.URL
	Headers        *CaseInsensitiveDict
	Body           []byte
	Cookies        map[string]string
	Stream         bool
	Auth           Authenticator
	Hooks          map[string][]Hook
	MaxRedirects   int
	AllowRedirects bool
	Timeout        time.Duration
	ConnectTimeout time.Duration
	ReadTimeout    time.Duration
	Proxies        map[string]string
	Verify         bool
	CACert         string
	Client         *http.Client

	// jar is the cookie jar in effect when the request was prepared.
	jar *CookieJar
}
