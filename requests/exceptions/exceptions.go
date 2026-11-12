// Package exceptions mirrors the exception hierarchy of the Python `requests`
// library, adapted to Go's error idiom.
//
// Every error returned by the parent `requests` package implements the
// RequestException interface, so callers can catch the broad category with
// errors.As and a *RequestException target, or match specific types such as
// *HTTPError, *Timeout, *ConnectionError, etc. via errors.As / errors.Is.
package exceptions

import (
	"fmt"
	"net/http"
)

// RequestException is implemented by every error raised by the requests
// package. It allows callers to test for the broad category of request errors
// in one place.
type RequestException interface {
	error
	requestException()
}

// RequestError is the generic base exception, equivalent to
// requests.exceptions.RequestException. Other concrete exceptions embed it.
type RequestError struct {
	Message string
	Cause   error
}

// Error implements the error interface.
func (e *RequestError) Error() string {
	if e.Message != "" && e.Cause != nil {
		return fmt.Sprintf("%s: %v", e.Message, e.Cause)
	}
	if e.Cause != nil {
		return e.Cause.Error()
	}
	if e.Message != "" {
		return e.Message
	}
	return "request error"
}

// Unwrap returns the underlying cause, if any.
func (e *RequestError) Unwrap() error { return e.Cause }

func (e *RequestError) requestException() {}

// HTTPError is raised when a request returns a 4xx or 5xx status code.
// Response holds the *requests.Response that produced the error (typed as any
// to avoid an import cycle between the two packages).
type HTTPError struct {
	RequestError
	StatusCode int
	Reason     string
	URL        string
	Response   any
}

// Error mirrors requests' message: "404 Client Error: Not Found for url: ...".
func (e *HTTPError) Error() string {
	kind := "Client Error"
	if e.StatusCode >= 500 {
		kind = "Server Error"
	}
	if e.Reason == "" {
		e.Reason = http.StatusText(e.StatusCode)
	}
	return fmt.Sprintf("%d %s: %s for url: %s", e.StatusCode, kind, e.Reason, e.URL)
}

// ConnectionError is raised for low-level network failures.
type ConnectionError struct{ RequestError }

// ProxyError is raised for proxy connection failures.
type ProxyError struct{ RequestError }

// SSLError is raised for TLS/certificate verification failures.
type SSLError struct{ RequestError }

// Timeout is raised when a request times out.
type Timeout struct{ RequestError }

// ConnectTimeout is raised when connecting to the remote server times out.
type ConnectTimeout struct{ RequestError }

// ReadTimeout is raised when the server does not send data in time.
type ReadTimeout struct{ RequestError }

// TooManyRedirects is raised when the request exceeds the redirect limit.
type TooManyRedirects struct {
	RequestError
	Response any
}

// MissingSchema is raised when a URL has no scheme (e.g. "example.com").
type MissingSchema struct{ RequestError }

// InvalidSchema is raised when a URL uses an unsupported scheme.
type InvalidSchema struct{ RequestError }

// InvalidURL is raised when a URL cannot be parsed.
type InvalidURL struct{ RequestError }

// URLRequired is raised when a request is made without a URL.
type URLRequired struct{ RequestError }

// ChunkedEncodingError is raised for invalid chunked transfer encoding.
type ChunkedEncodingError struct{ RequestError }

// ContentDecodingError is raised when a response body cannot be decoded.
type ContentDecodingError struct{ RequestError }

// InvalidHeader is raised when an invalid header value is supplied.
type InvalidHeader struct{ RequestError }

// InvalidJSONError is raised when a value cannot be encoded as JSON.
type InvalidJSONError struct{ RequestError }

// Is makes ProxyError and SSLError satisfy errors.Is against ConnectionError,
// preserving the subclass relationship from requests.exceptions.
func (e *ProxyError) Is(target error) bool {
	_, ok := target.(*ConnectionError)
	return ok
}

// Is makes SSLError satisfy errors.Is against ConnectionError.
func (e *SSLError) Is(target error) bool {
	_, ok := target.(*ConnectionError)
	return ok
}

// Is makes ConnectTimeout satisfy errors.Is against Timeout.
func (e *ConnectTimeout) Is(target error) bool {
	_, ok := target.(*Timeout)
	return ok
}

// Is makes ReadTimeout satisfy errors.Is against Timeout.
func (e *ReadTimeout) Is(target error) bool {
	_, ok := target.(*Timeout)
	return ok
}
