package requests

import (
	"fmt"
	"net/http"
	"sort"
	"strings"
	"sync"
)

// CaseInsensitiveDict preserves the original casing of keys while providing
// case-insensitive lookups, mirroring requests' own CaseInsensitiveDict used
// for HTTP headers and cookies.
type CaseInsensitiveDict struct {
	mu   sync.RWMutex
	keys map[string]string   // lowercased key -> original key
	data map[string][]string // original key -> values
}

// NewCaseInsensitiveDict returns an empty CaseInsensitiveDict.
func NewCaseInsensitiveDict() *CaseInsensitiveDict {
	return &CaseInsensitiveDict{
		keys: make(map[string]string),
		data: make(map[string][]string),
	}
}

func (d *CaseInsensitiveDict) normalize(key string) string {
	return strings.ToLower(strings.TrimSpace(key))
}

// Set stores a single value for key, replacing any previous values.
func (d *CaseInsensitiveDict) Set(key, value string) {
	lower := d.normalize(key)
	d.mu.Lock()
	defer d.mu.Unlock()
	if orig, ok := d.keys[lower]; ok {
		d.data[orig] = []string{value}
		return
	}
	d.keys[lower] = key
	d.data[key] = []string{value}
}

// Add appends a value for key, preserving any existing values.
func (d *CaseInsensitiveDict) Add(key, value string) {
	lower := d.normalize(key)
	d.mu.Lock()
	defer d.mu.Unlock()
	if orig, ok := d.keys[lower]; ok {
		d.data[orig] = append(d.data[orig], value)
		return
	}
	d.keys[lower] = key
	d.data[key] = []string{value}
}

// Del removes key regardless of casing.
func (d *CaseInsensitiveDict) Del(key string) {
	lower := d.normalize(key)
	d.mu.Lock()
	defer d.mu.Unlock()
	if orig, ok := d.keys[lower]; ok {
		delete(d.data, orig)
		delete(d.keys, lower)
	}
}

// Get returns the first value for key and whether it exists.
func (d *CaseInsensitiveDict) Get(key string) (string, bool) {
	lower := d.normalize(key)
	d.mu.RLock()
	defer d.mu.RUnlock()
	orig, ok := d.keys[lower]
	if !ok {
		return "", false
	}
	v := d.data[orig]
	if len(v) == 0 {
		return "", false
	}
	return v[0], true
}

// GetFirst returns the first value for key, or "" if absent.
func (d *CaseInsensitiveDict) GetFirst(key string) string {
	v, _ := d.Get(key)
	return v
}

// Values returns a copy of all values for key.
func (d *CaseInsensitiveDict) Values(key string) []string {
	lower := d.normalize(key)
	d.mu.RLock()
	defer d.mu.RUnlock()
	orig, ok := d.keys[lower]
	if !ok {
		return nil
	}
	out := make([]string, len(d.data[orig]))
	copy(out, d.data[orig])
	return out
}

// Has reports whether key is present.
func (d *CaseInsensitiveDict) Has(key string) bool {
	_, ok := d.Get(key)
	return ok
}

// Len returns the number of distinct keys.
func (d *CaseInsensitiveDict) Len() int {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return len(d.keys)
}

// Keys returns the keys in their original casing, sorted for determinism.
func (d *CaseInsensitiveDict) Keys() []string {
	d.mu.RLock()
	defer d.mu.RUnlock()
	out := make([]string, 0, len(d.keys))
	for _, orig := range d.keys {
		out = append(out, orig)
	}
	sort.Strings(out)
	return out
}

// Items returns a copy of the raw mapping (original casing -> values).
func (d *CaseInsensitiveDict) Items() map[string][]string {
	d.mu.RLock()
	defer d.mu.RUnlock()
	out := make(map[string][]string, len(d.data))
	for k, v := range d.data {
		out[k] = append([]string(nil), v...)
	}
	return out
}

// String returns a readable representation of the dict.
func (d *CaseInsensitiveDict) String() string {
	d.mu.RLock()
	defer d.mu.RUnlock()
	var b strings.Builder
	b.WriteString("{")
	keys := make([]string, 0, len(d.keys))
	for k := range d.keys {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for i, k := range keys {
		orig := d.keys[k]
		if i > 0 {
			b.WriteString(", ")
		}
		b.WriteString(fmt.Sprintf("%q: %q", orig, strings.Join(d.data[orig], ", ")))
	}
	b.WriteString("}")
	return b.String()
}

// Clone returns a deep copy of the dict.
func (d *CaseInsensitiveDict) Clone() *CaseInsensitiveDict {
	n := NewCaseInsensitiveDict()
	d.mu.RLock()
	defer d.mu.RUnlock()
	for lower, orig := range d.keys {
		n.keys[lower] = orig
		n.data[orig] = append([]string(nil), d.data[orig]...)
	}
	return n
}

// toHTTPHeader converts the dict into a net/http.Header.
func (d *CaseInsensitiveDict) toHTTPHeader() http.Header {
	d.mu.RLock()
	defer d.mu.RUnlock()
	h := make(http.Header, len(d.data))
	for orig, vals := range d.data {
		for _, v := range vals {
			h.Add(orig, v)
		}
	}
	return h
}

// caseInsensitiveDictFromHTTP builds a CaseInsensitiveDict from net/http.Header.
func caseInsensitiveDictFromHTTP(h http.Header) *CaseInsensitiveDict {
	d := NewCaseInsensitiveDict()
	for k, vals := range h {
		for _, v := range vals {
			d.Add(k, v)
		}
	}
	return d
}
