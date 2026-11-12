package requests

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"unicode/utf8"
)

// isRedirect reports whether the status code is a redirect.
func isRedirect(status int) bool {
	switch status {
	case 301, 302, 303, 307, 308:
		return true
	default:
		return false
	}
}

// encodeParams normalizes a params-like value into an encoded query string.
// Supported types: string (already encoded), []Param, map[string]string,
// map[string][]string, url.Values, map[string]any, and nil.
func encodeParams(params any) (string, error) {
	if params == nil {
		return "", nil
	}
	switch v := params.(type) {
	case string:
		return v, nil
	case []Param:
		var parts []string
		for _, p := range v {
			parts = append(parts, url.QueryEscape(p.Key)+"="+url.QueryEscape(p.Value))
		}
		return strings.Join(parts, "&"), nil
	case map[string]string:
		vals := url.Values{}
		for k, val := range v {
			vals.Set(k, val)
		}
		return vals.Encode(), nil
	case map[string][]string:
		return url.Values(v).Encode(), nil
	case url.Values:
		return v.Encode(), nil
	case map[string]any:
		vals := url.Values{}
		for k, val := range v {
			switch t := val.(type) {
			case []string:
				for _, s := range t {
					vals.Add(k, s)
				}
			case []any:
				for _, s := range t {
					vals.Add(k, fmt.Sprint(s))
				}
			default:
				vals.Set(k, fmt.Sprint(val))
			}
		}
		return vals.Encode(), nil
	default:
		return "", fmt.Errorf("unsupported params type %T", params)
	}
}

// encodeData encodes a data-like value into a request body. It returns the
// body and a Content-Type (empty when no Content-Type should be set, matching
// requests' behaviour for raw string/bytes data).
func encodeData(data any) ([]byte, string, error) {
	switch v := data.(type) {
	case nil:
		return nil, "", nil
	case []byte:
		return v, "", nil
	case string:
		return []byte(v), "", nil
	case url.Values:
		return []byte(v.Encode()), "application/x-www-form-urlencoded", nil
	case map[string]string:
		vals := url.Values{}
		for k, val := range v {
			vals.Set(k, val)
		}
		return []byte(vals.Encode()), "application/x-www-form-urlencoded", nil
	case map[string]any:
		vals := url.Values{}
		for k, val := range v {
			vals.Set(k, fmt.Sprint(val))
		}
		return []byte(vals.Encode()), "application/x-www-form-urlencoded", nil
	case []Param:
		vals := url.Values{}
		for _, p := range v {
			vals.Add(p.Key, p.Value)
		}
		return []byte(vals.Encode()), "application/x-www-form-urlencoded", nil
	default:
		return nil, "", fmt.Errorf("unsupported data type %T", data)
	}
}

// encodeJSON marshals a value to JSON bytes.
func encodeJSON(v any) ([]byte, error) {
	return json.Marshal(v)
}

// detectEncoding extracts the charset from a Content-Type header.
func detectEncoding(h http.Header) string {
	ct := h.Get("Content-Type")
	if ct == "" {
		return ""
	}
	for _, part := range strings.Split(ct, ";") {
		part = strings.TrimSpace(part)
		if strings.HasPrefix(strings.ToLower(part), "charset=") {
			val := strings.TrimPrefix(part, "charset=")
			return strings.Trim(val, `"' `)
		}
	}
	return ""
}

// detectEncodingFromContent guesses the text encoding of a body: UTF-8 when
// valid, ISO-8859-1 otherwise (mirroring requests' default).
func detectEncodingFromContent(b []byte) string {
	if utf8.Valid(b) {
		return "utf-8"
	}
	return "latin-1"
}

// decodeText decodes bytes into a string using the given charset label.
func decodeText(b []byte, encoding string) string {
	switch strings.ToLower(strings.TrimSpace(encoding)) {
	case "utf-8", "utf8", "unicode-1-1-utf-8":
		return string(b)
	case "latin-1", "latin1", "iso-8859-1", "iso8859-1", "cp1252", "windows-1252", "ascii", "us-ascii":
		return latin1ToString(b)
	default:
		// Best-effort UTF-8 for unknown charsets.
		return string(b)
	}
}

// latin1ToString converts a byte slice to a string, mapping each byte to the
// rune with the same code point (ISO-8859-1 semantics).
func latin1ToString(b []byte) string {
	rs := make([]rune, len(b))
	for i, c := range b {
		rs[i] = rune(c)
	}
	return string(rs)
}

// parseLinks parses an RFC 5988 Link header into a rel -> url mapping.
func parseLinks(value string) map[string]string {
	links := map[string]string{}
	for _, part := range strings.Split(value, ",") {
		part = strings.TrimSpace(part)
		var link, rel string
		if i := strings.Index(part, "<"); i >= 0 {
			if j := strings.Index(part[i:], ">"); j >= 0 {
				link = part[i+1 : i+j]
			}
		}
		for _, f := range strings.Split(part, ";") {
			f = strings.TrimSpace(f)
			if strings.HasPrefix(strings.ToLower(f), "rel=") {
				rel = strings.Trim(strings.TrimPrefix(f, "rel="), `"' `)
			}
		}
		if rel != "" && link != "" {
			links[rel] = link
		}
	}
	return links
}

// matchesNoProxy reports whether host matches an entry in a comma-separated
// no_proxy list. Entries may be a bare domain, ".example.com", or "*".
func matchesNoProxy(host, noProxy string) bool {
	if noProxy == "" {
		return false
	}
	for _, entry := range strings.Split(noProxy, ",") {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}
		if entry == "*" {
			return true
		}
		entry = strings.TrimPrefix(entry, ".")
		if host == entry || strings.HasSuffix(host, "."+entry) {
			return true
		}
	}
	return false
}
