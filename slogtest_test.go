package logger

import (
	"encoding/json"
	"log/slog"
	"testing"
	"testing/slogtest"
)

// TestSlogtest checks each backend, through the full handler chain built by
// New, against the slog.Handler contract.
func TestSlogtest(t *testing.T) {
	for _, b := range backends {
		t.Run(string(b), func(t *testing.T) {
			var buf *buffer
			slogtest.Run(t,
				func(t *testing.T) slog.Handler {
					var l *Logger
					l, buf = newTestLogger(t, b)
					return l.Handler()
				},
				func(t *testing.T) map[string]any {
					lines := buf.lines()
					if len(lines) != 1 {
						t.Fatalf("got %d lines, want 1:\n%s", len(lines), buf)
					}
					var m map[string]any
					if err := json.Unmarshal([]byte(lines[0]), &m); err != nil {
						t.Fatal(err)
					}
					return m
				},
			)
		})
	}
}
