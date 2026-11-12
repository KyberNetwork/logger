package logger

import (
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"
)

func newBenchLogger(b *testing.B, backend Backend) *Logger {
	b.Helper()
	l, err := New(Config{Backend: backend}, WithWriter(io.Discard))
	if err != nil {
		b.Fatal(err)
	}
	return l
}

func BenchmarkInfo(b *testing.B) {
	err := errors.New("boom")
	for _, backend := range backends {
		b.Run(string(backend), func(b *testing.B) {
			l := newBenchLogger(b, backend)
			b.ReportAllocs()
			b.RunParallel(func(pb *testing.PB) {
				for pb.Next() {
					l.Info("request handled", "path", "/v1/route", "status", 200, "took", 3*time.Millisecond, "err", err)
				}
			})
		})
	}
}

func BenchmarkInfoWith(b *testing.B) {
	for _, backend := range backends {
		b.Run(string(backend), func(b *testing.B) {
			l := newBenchLogger(b, backend).With("service", "router", "version", "2.0.0").WithGroup("req")
			b.ReportAllocs()
			b.RunParallel(func(pb *testing.PB) {
				for pb.Next() {
					l.Info("request handled", "path", "/v1/route", "status", 200)
				}
			})
		})
	}
}

func BenchmarkDisabled(b *testing.B) {
	for _, backend := range backends {
		b.Run(string(backend), func(b *testing.B) {
			l := newBenchLogger(b, backend)
			b.ReportAllocs()
			for b.Loop() {
				l.Debug("dropped", "path", "/v1/route")
			}
		})
	}
}

func BenchmarkLogAttrs(b *testing.B) {
	for _, backend := range backends {
		b.Run(string(backend), func(b *testing.B) {
			l := newBenchLogger(b, backend)
			b.ReportAllocs()
			for b.Loop() {
				l.LogAttrs(b.Context(), slog.LevelInfo, "request handled", slog.String("path", "/v1/route"), slog.Int("status", 200))
			}
		})
	}
}
