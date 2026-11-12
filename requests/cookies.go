package requests

import (
	"net/http"
	"net/http/cookiejar"
	"net/url"
)

// CookieJar stores cookies across requests within a Session, mirroring
// requests.cookies.RequestsCookieJar. It is not safe for concurrent use.
type CookieJar struct {
	jar http.CookieJar
}

// NewCookieJar returns an empty CookieJar.
func NewCookieJar() *CookieJar {
	jar, _ := cookiejar.New(nil)
	return &CookieJar{jar: jar}
}

// Cookies returns the cookies that should be sent with a request to u.
func (j *CookieJar) Cookies(u *url.URL) []*http.Cookie {
	if j == nil || j.jar == nil {
		return nil
	}
	return j.jar.Cookies(u)
}

// SetCookies stores the cookies returned by a response from u.
func (j *CookieJar) SetCookies(u *url.URL, cookies []*http.Cookie) {
	if j == nil || j.jar == nil {
		return
	}
	j.jar.SetCookies(u, cookies)
}

// Update stores cookies for the given URL (alias of SetCookies).
func (j *CookieJar) Update(u *url.URL, cookies []*http.Cookie) {
	j.SetCookies(u, cookies)
}

// Set stores a single cookie for the given domain and path.
func (j *CookieJar) Set(cookie *http.Cookie, domain, path string) {
	if j == nil || j.jar == nil || cookie == nil {
		return
	}
	scheme := "http"
	if cookie.Secure {
		scheme = "https"
	}
	u := &url.URL{Scheme: scheme, Host: domain, Path: path}
	j.jar.SetCookies(u, []*http.Cookie{cookie})
}

// Get returns the first cookie with the given name for the domain and path.
func (j *CookieJar) Get(name, domain, path string) *http.Cookie {
	if j == nil {
		return nil
	}
	u := &url.URL{Scheme: "http", Host: domain, Path: path}
	for _, c := range j.jar.Cookies(u) {
		if c.Name == name {
			return c
		}
	}
	return nil
}

// Clear removes all stored cookies.
func (j *CookieJar) Clear() {
	jar, _ := cookiejar.New(nil)
	j.jar = jar
}
