package requests

import (
	"bufio"
	"encoding/json"
	"io"
	"net/http"
	"time"

	"github.com/roseboy/private-gotool/requests/exceptions"
)

// Response represents the server's response to an HTTP request, mirroring
// requests.Response.
type Response struct {
	// StatusCode is the integer status code (e.g. 200).
	StatusCode int
	// Reason is the reason phrase (e.g. "OK").
	Reason string
	// Headers is the case-insensitive response header dict.
	Headers *CaseInsensitiveDict
	// Content holds the decoded body bytes. In streaming mode it stays empty
	// until the body is consumed via Body / IterContent / IterLines.
	Content []byte
	// URL is the final URL after any redirects.
	URL string
	// Encoding is the charset used to decode Text().
	Encoding string
	// Request is the prepared request that produced this response.
	Request *PreparedRequest
	// History lists the redirect responses that led to this one.
	History []*Response
	// Elapsed is the time spent performing the request.
	Elapsed time.Duration
	// Cookies are the cookies set by the response.
	Cookies []*http.Cookie
	// Links are the parsed Link header entries (rel -> url).
	Links map[string]string

	// Body is the open response body, available only in streaming mode.
	Body io.ReadCloser

	raw *http.Response
}

// Ok reports whether the response was successful (2xx or 3xx).
func (r *Response) Ok() bool {
	return r.StatusCode >= 200 && r.StatusCode < 400
}

// Text decodes the body to a string using the detected encoding.
func (r *Response) Text() string {
	if r.Encoding == "" {
		r.Encoding = detectEncodingFromContent(r.Content)
	}
	return decodeText(r.Content, r.Encoding)
}

// JSON decodes the body as JSON into an any value.
func (r *Response) JSON() (any, error) {
	var out any
	if err := json.Unmarshal(r.Content, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// ApparentEncoding guesses the encoding from the raw body content.
func (r *Response) ApparentEncoding() string {
	return detectEncodingFromContent(r.Content)
}

// RaiseForStatus returns an *exceptions.HTTPError if the response has a 4xx or
// 5xx status code, otherwise nil.
func (r *Response) RaiseForStatus() error {
	if r.StatusCode >= 400 {
		return &exceptions.HTTPError{
			StatusCode: r.StatusCode,
			Reason:     r.Reason,
			URL:        r.URL,
			Response:   r,
		}
	}
	return nil
}

// Close closes the underlying response body. Safe to call multiple times.
func (r *Response) Close() {
	if r.raw != nil && r.raw.Body != nil {
		r.raw.Body.Close()
		r.raw = nil
	}
}

// IterContent yields the body in chunks (streaming mode only). The returned
// channel closes when the body is fully consumed.
func (r *Response) IterContent(chunkSize int) <-chan []byte {
	ch := make(chan []byte)
	go func() {
		defer close(ch)
		if r.raw == nil || r.raw.Body == nil {
			return
		}
		if chunkSize <= 0 {
			chunkSize = 8192
		}
		buf := make([]byte, chunkSize)
		for {
			n, err := r.raw.Body.Read(buf)
			if n > 0 {
				b := make([]byte, n)
				copy(b, buf[:n])
				ch <- b
			}
			if err != nil {
				r.raw.Body.Close()
				return
			}
		}
	}()
	return ch
}

// IterLines yields the body line by line (streaming mode only).
func (r *Response) IterLines() <-chan string {
	ch := make(chan string)
	go func() {
		defer close(ch)
		if r.raw == nil || r.raw.Body == nil {
			return
		}
		sc := bufio.NewScanner(r.raw.Body)
		for sc.Scan() {
			ch <- sc.Text()
		}
		r.raw.Body.Close()
	}()
	return ch
}

// newResponse builds a Response from a raw *http.Response.
func newResponse(resp *http.Response, prep *PreparedRequest, elapsed time.Duration) *Response {
	r := &Response{
		StatusCode: resp.StatusCode,
		Reason:     http.StatusText(resp.StatusCode),
		Headers:    caseInsensitiveDictFromHTTP(resp.Header),
		URL:        resp.Request.URL.String(),
		Request:    prep,
		Elapsed:    elapsed,
		Cookies:    resp.Cookies(),
		Links:      parseLinks(resp.Header.Get("Link")),
		raw:        resp,
	}
	if ct := resp.Header.Get("Content-Type"); ct != "" {
		r.Encoding = detectEncoding(resp.Header)
	}
	if !prep.Stream {
		if body, err := io.ReadAll(resp.Body); err == nil {
			r.Content = body
		}
		resp.Body.Close()
		r.raw = nil
	} else {
		r.Body = resp.Body
	}
	return r
}
