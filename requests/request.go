package requests

import (
	"bytes"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/roseboy/private-gotool/requests/exceptions"
)

// Prepare validates and converts the Request into a PreparedRequest, mirroring
// requests.Request.prepare().
func (r *Request) Prepare() (*PreparedRequest, error) {
	p := &PreparedRequest{
		Headers:        NewCaseInsensitiveDict(),
		Cookies:        map[string]string{},
		Stream:         r.Stream,
		Auth:           r.Auth,
		Hooks:          r.Hooks,
		MaxRedirects:   r.MaxRedirects,
		AllowRedirects: r.AllowRedirects,
		Timeout:        r.Timeout,
		ConnectTimeout: r.ConnectTimeout,
		ReadTimeout:    r.ReadTimeout,
		Proxies:        r.Proxies,
		Verify:         r.Verify,
		CACert:         r.CACert,
		Client:         r.Client,
	}

	if err := p.prepareMethod(r.Method); err != nil {
		return nil, err
	}
	if err := p.prepareURL(r.URL, r.Params); err != nil {
		return nil, err
	}
	if err := p.prepareHeaders(r.Headers); err != nil {
		return nil, err
	}
	if err := p.prepareCookies(r.Cookies); err != nil {
		return nil, err
	}
	if err := p.prepareBody(r.Data, r.JSON, r.Files); err != nil {
		return nil, err
	}
	if err := p.prepareAuth(r.Auth); err != nil {
		return nil, err
	}
	return p, nil
}

func (p *PreparedRequest) prepareMethod(method string) error {
	if method == "" {
		return &exceptions.URLRequired{RequestError: exceptions.RequestError{Message: "Invalid method: empty method is not allowed"}}
	}
	p.Method = strings.ToUpper(method)
	return nil
}

func (p *PreparedRequest) prepareURL(rawurl string, params any) error {
	if rawurl == "" {
		return &exceptions.MissingSchema{RequestError: exceptions.RequestError{Message: "Invalid URL: No URL supplied"}}
	}
	u, err := url.Parse(rawurl)
	if err != nil {
		return &exceptions.InvalidURL{RequestError: exceptions.RequestError{Message: fmt.Sprintf("Invalid URL %q: %v", rawurl, err), Cause: err}}
	}
	if u.Scheme == "" {
		return &exceptions.MissingSchema{RequestError: exceptions.RequestError{Message: fmt.Sprintf("Invalid URL %q: No scheme supplied", rawurl)}}
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return &exceptions.InvalidSchema{RequestError: exceptions.RequestError{Message: fmt.Sprintf("No connection adapters were found for %q", rawurl)}}
	}

	if params != nil {
		q, err := encodeParams(params)
		if err != nil {
			return &exceptions.InvalidURL{RequestError: exceptions.RequestError{Message: err.Error(), Cause: err}}
		}
		if q != "" {
			if u.RawQuery == "" {
				u.RawQuery = q
			} else {
				u.RawQuery = u.RawQuery + "&" + q
			}
		}
	}
	p.URL = u
	return nil
}

func (p *PreparedRequest) prepareHeaders(h *CaseInsensitiveDict) error {
	if h != nil {
		p.Headers = h.Clone()
	}
	if !p.Headers.Has("User-Agent") {
		p.Headers.Set("User-Agent", defaultUserAgent)
	}
	return nil
}

func (p *PreparedRequest) prepareCookies(c any) error {
	if c == nil {
		return nil
	}
	switch v := c.(type) {
	case map[string]string:
		for k, val := range v {
			p.Cookies[k] = val
		}
	case []*http.Cookie:
		for _, ck := range v {
			p.Cookies[ck.Name] = ck.Value
		}
	case *http.Cookie:
		p.Cookies[v.Name] = v.Value
	default:
		return fmt.Errorf("unsupported cookies type %T", c)
	}
	return nil
}

func (p *PreparedRequest) prepareBody(data, jsonVal, files any) error {
	if files != nil {
		return p.prepareMultipart(data, files)
	}
	if jsonVal != nil {
		b, err := encodeJSON(jsonVal)
		if err != nil {
			return &exceptions.InvalidJSONError{RequestError: exceptions.RequestError{Message: "Invalid JSON", Cause: err}}
		}
		p.Body = b
		if !p.Headers.Has("Content-Type") {
			p.Headers.Set("Content-Type", "application/json")
		}
		return nil
	}
	if data != nil {
		b, ct, err := encodeData(data)
		if err != nil {
			return err
		}
		p.Body = b
		if ct != "" && !p.Headers.Has("Content-Type") {
			p.Headers.Set("Content-Type", ct)
		}
	}
	return nil
}

func (p *PreparedRequest) prepareMultipart(data, files any) error {
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)

	if data != nil {
		switch d := data.(type) {
		case map[string]string:
			for k, v := range d {
				if err := w.WriteField(k, v); err != nil {
					return err
				}
			}
		case map[string]any:
			for k, v := range d {
				if err := w.WriteField(k, fmt.Sprint(v)); err != nil {
					return err
				}
			}
		case url.Values:
			for k, vs := range d {
				for _, v := range vs {
					if err := w.WriteField(k, v); err != nil {
						return err
					}
				}
			}
		}
	}

	switch f := files.(type) {
	case map[string]string:
		for field, path := range f {
			if err := addFile(w, field, File{Path: path}); err != nil {
				return err
			}
		}
	case map[string]*File:
		for field, file := range f {
			if err := addFile(w, field, *file); err != nil {
				return err
			}
		}
	case map[string]File:
		for field, file := range f {
			if err := addFile(w, field, file); err != nil {
				return err
			}
		}
	case map[string][]byte:
		for field, content := range f {
			if err := addFile(w, field, File{FileName: field, Content: content}); err != nil {
				return err
			}
		}
	default:
		return fmt.Errorf("unsupported files type %T", files)
	}

	if err := w.Close(); err != nil {
		return err
	}
	p.Body = buf.Bytes()
	p.Headers.Set("Content-Type", w.FormDataContentType())
	return nil
}

func addFile(w *multipart.Writer, field string, f File) error {
	content := f.Content
	filename := f.FileName
	if f.Path != "" {
		b, err := os.ReadFile(f.Path)
		if err != nil {
			return err
		}
		content = b
		if filename == "" {
			filename = filepath.Base(f.Path)
		}
	}
	if filename == "" {
		filename = field
	}
	part, err := w.CreateFormFile(field, filename)
	if err != nil {
		return err
	}
	_, err = part.Write(content)
	return err
}

func (p *PreparedRequest) prepareAuth(a Authenticator) error {
	if a == nil {
		return nil
	}
	return a.Apply(p)
}

// toHTTPRequest converts the prepared request into a *net/http.Request.
func (p *PreparedRequest) toHTTPRequest() (*http.Request, error) {
	req := &http.Request{
		Method: p.Method,
		URL:    p.URL,
		Header: p.Headers.toHTTPHeader(),
	}

	if p.Body != nil {
		req.Body = io.NopCloser(bytes.NewReader(p.Body))
		req.ContentLength = int64(len(p.Body))
		req.GetBody = func() (io.ReadCloser, error) {
			return io.NopCloser(bytes.NewReader(p.Body)), nil
		}
	} else if p.Method == "POST" || p.Method == "PUT" || p.Method == "PATCH" {
		req.Body = http.NoBody
		req.ContentLength = 0
	}

	for k, v := range p.Cookies {
		req.AddCookie(&http.Cookie{Name: k, Value: v})
	}
	return req, nil
}

// cloneForRedirect produces the next request to send after a redirect,
// applying the method/header adjustments that requests performs.
func (p *PreparedRequest) cloneForRedirect(nextURL *url.URL, status int) *PreparedRequest {
	np := &PreparedRequest{
		Method:         p.Method,
		URL:            nextURL,
		Headers:        p.Headers.Clone(),
		Body:           p.Body,
		Cookies:        p.Cookies,
		Stream:         p.Stream,
		Auth:           p.Auth,
		Hooks:          p.Hooks,
		MaxRedirects:   p.MaxRedirects,
		AllowRedirects: p.AllowRedirects,
		Timeout:        p.Timeout,
		ConnectTimeout: p.ConnectTimeout,
		ReadTimeout:    p.ReadTimeout,
		Proxies:        p.Proxies,
		Verify:         p.Verify,
		CACert:         p.CACert,
		Client:         p.Client,
		jar:            p.jar,
	}

	switch status {
	case 301, 302, 303:
		if np.Method != "GET" && np.Method != "HEAD" {
			np.Method = "GET"
			np.Body = nil
			np.Headers.Del("Content-Type")
			np.Headers.Del("Content-Length")
		}
	case 307, 308:
		// Method and body are preserved.
	}

	// Drop sensitive headers and cookies when redirecting to a different host.
	if p.URL.Host != nextURL.Host {
		np.Headers.Del("Authorization")
		np.Headers.Del("Cookie")
		np.Cookies = map[string]string{}
		np.Auth = nil
	}
	return np
}
