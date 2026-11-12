package logger

import (
	"bytes"
	"encoding/json"
	"strings"
	"sync"
	"testing"
)

var backends = []Backend{BackendSlog, BackendZap, BackendZerolog}

// buffer is a goroutine-safe bytes.Buffer.
type buffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *buffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *buffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

func (b *buffer) Reset() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.buf.Reset()
}

// lines returns the non-empty lines written so far.
func (b *buffer) lines() []string {
	var out []string
	for _, l := range strings.Split(b.String(), "\n") {
		if l != "" {
			out = append(out, l)
		}
	}
	return out
}

// records decodes every line written so far as a JSON object.
func (b *buffer) records(t *testing.T) []map[string]any {
	t.Helper()
	var out []map[string]any
	for _, l := range b.lines() {
		out = append(out, decode(t, l))
	}
	return out
}

// record decodes the only line written so far.
func (b *buffer) record(t *testing.T) map[string]any {
	t.Helper()
	recs := b.records(t)
	if len(recs) != 1 {
		t.Fatalf("got %d records, want 1:\n%s", len(recs), b.String())
	}
	return recs[0]
}

// decode parses a JSON line, keeping numbers exact.
func decode(t *testing.T, line string) map[string]any {
	t.Helper()
	d := json.NewDecoder(strings.NewReader(line))
	d.UseNumber()
	var m map[string]any
	if err := d.Decode(&m); err != nil {
		t.Fatalf("invalid JSON %q: %v", line, err)
	}
	return m
}

// newTestLogger returns a logger for backend b that writes JSON at debug
// level to the returned buffer.
func newTestLogger(t *testing.T, b Backend, opts ...Option) (*Logger, *buffer) {
	t.Helper()
	var buf buffer
	opts = append([]Option{WithWriter(&buf)}, opts...)
	l, err := New(Config{Backend: b, Level: "debug"}, opts...)
	if err != nil {
		t.Fatal(err)
	}
	return l, &buf
}
