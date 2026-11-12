package requests

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/roseboy/private-gotool/requests/exceptions"
)

func TestGetQueryParams(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("page") != "2" || r.URL.Query().Get("q") != "hello world" {
			t.Errorf("unexpected query: %s", r.URL.RawQuery)
		}
		fmt.Fprint(w, "ok")
	}))
	defer srv.Close()

	r, err := Get(srv.URL, Params(map[string]string{"page": "2", "q": "hello world"}))
	if err != nil {
		t.Fatal(err)
	}
	if r.Text() != "ok" {
		t.Fatalf("unexpected body: %q", r.Text())
	}
	if !r.Ok() {
		t.Fatalf("expected ok")
	}
}

func TestPostJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if ct := r.Header.Get("Content-Type"); ct != "application/json" {
			t.Errorf("unexpected content type: %q", ct)
		}
		fmt.Fprint(w, `{"received": true}`)
	}))
	defer srv.Close()

	r, err := Post(srv.URL, JSON(map[string]any{"name": "requests.go"}))
	if err != nil {
		t.Fatal(err)
	}
	v, err := r.JSON()
	if err != nil {
		t.Fatal(err)
	}
	if m, ok := v.(map[string]any); !ok || m["received"] != true {
		t.Fatalf("unexpected json: %#v", v)
	}
}

func TestPostForm(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			t.Error(err)
		}
		if r.Form.Get("user") != "alice" {
			t.Errorf("unexpected form: %#v", r.Form)
		}
	}))
	defer srv.Close()

	if _, err := Post(srv.URL, Data(map[string]string{"user": "alice"})); err != nil {
		t.Fatal(err)
	}
}

func TestSessionCookies(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, err := r.Cookie("session")
		if err != nil {
			http.SetCookie(w, &http.Cookie{Name: "session", Value: "abc123"})
			fmt.Fprint(w, "set")
			return
		}
		fmt.Fprint(w, "cookie="+c.Value)
	}))
	defer srv.Close()

	s := NewSession()
	r1, err := s.Get(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	if r1.Text() != "set" {
		t.Fatalf("first response: %q", r1.Text())
	}
	r2, err := s.Get(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	if r2.Text() != "cookie=abc123" {
		t.Fatalf("cookie not persisted: %q", r2.Text())
	}
}

func TestRedirectAndHistory(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/redirect" {
			http.Redirect(w, r, "/final", http.StatusFound)
			return
		}
		fmt.Fprint(w, "final")
	}))
	defer srv.Close()

	r, err := Get(srv.URL + "/redirect")
	if err != nil {
		t.Fatal(err)
	}
	if r.Text() != "final" {
		t.Fatalf("unexpected body: %q", r.Text())
	}
	if len(r.History) != 1 || r.History[0].StatusCode != 302 {
		t.Fatalf("unexpected history: %d", len(r.History))
	}
}

func TestTooManyRedirects(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/loop", http.StatusFound)
	}))
	defer srv.Close()

	_, err := Get(srv.URL, MaxRedirects(3))
	var tmr *exceptions.TooManyRedirects
	if !errors.As(err, &tmr) {
		t.Fatalf("expected TooManyRedirects, got %v", err)
	}
}

func TestRaiseForStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "not found", http.StatusNotFound)
	}))
	defer srv.Close()

	r, err := Get(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	err = r.RaiseForStatus()
	var he *exceptions.HTTPError
	if !errors.As(err, &he) {
		t.Fatalf("expected HTTPError, got %v", err)
	}
	if he.StatusCode != 404 {
		t.Fatalf("expected 404, got %d", he.StatusCode)
	}
}

func TestBasicAuth(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		u, p, ok := r.BasicAuth()
		if !ok || u != "user" || p != "pass" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		fmt.Fprint(w, "authorized")
	}))
	defer srv.Close()

	r, err := Get(srv.URL, Auth(BasicAuth("user", "pass")))
	if err != nil {
		t.Fatal(err)
	}
	if r.Text() != "authorized" {
		t.Fatalf("unexpected body: %q", r.Text())
	}
}

func TestTimeout(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(300 * time.Millisecond)
		fmt.Fprint(w, "slow")
	}))
	defer srv.Close()

	_, err := Get(srv.URL, Timeout(50*time.Millisecond))
	var te *exceptions.Timeout
	if !errors.As(err, &te) {
		t.Fatalf("expected Timeout, got %v", err)
	}
}

func TestMissingSchema(t *testing.T) {
	_, err := Get("example.com")
	var ms *exceptions.MissingSchema
	if !errors.As(err, &ms) {
		t.Fatalf("expected MissingSchema, got %v", err)
	}
}

func TestVerifyFalse(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "secure")
	}))
	defer srv.Close()

	// Should fail with SSLError when verifying the self-signed cert.
	_, err := Get(srv.URL)
	var se *exceptions.SSLError
	if !errors.As(err, &se) {
		t.Fatalf("expected SSLError, got %v", err)
	}

	// Should succeed with verification disabled.
	r, err := Get(srv.URL, Verify(false))
	if err != nil {
		t.Fatal(err)
	}
	if r.Text() != "secure" {
		t.Fatalf("unexpected body: %q", r.Text())
	}
}

func TestStream(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		for i := 0; i < 3; i++ {
			fmt.Fprintf(w, "line%d\n", i)
		}
	}))
	defer srv.Close()

	r, err := Get(srv.URL, Stream(true))
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()

	var lines []string
	for line := range r.IterLines() {
		lines = append(lines, line)
	}
	if strings.Join(lines, "|") != "line0|line1|line2" {
		t.Fatalf("unexpected lines: %#v", lines)
	}
}

func TestPreparedRequestWorkflow(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, "%s %s", r.Method, r.URL.RawQuery)
	}))
	defer srv.Close()

	req := NewRequest("GET", srv.URL, Params(map[string]string{"a": "1"}))
	s := NewSession()
	prep, err := s.PrepareRequest(req)
	if err != nil {
		t.Fatal(err)
	}
	r, err := s.Send(prep)
	if err != nil {
		t.Fatal(err)
	}
	if r.Text() != "GET a=1" {
		t.Fatalf("unexpected body: %q", r.Text())
	}
}
