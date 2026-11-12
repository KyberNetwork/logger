package logger_test

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"log/slog"
	"os"

	"github.com/KyberNetwork/logger/v2"
)

func ExampleNew() {
	l, err := logger.New(logger.Config{
		Backend: logger.BackendZap,
		Level:   "debug",
		File:    logger.FileConfig{Path: "/var/log/app/app.log", MaxSizeMB: 100, MaxBackups: 5},
	})
	if err != nil {
		log.Fatal(err)
	}
	defer func() {
		if err := l.Close(); err != nil {
			log.Print(err)
		}
	}()

	// Make it the default for slog.Info and friends, and for the log package.
	slog.SetDefault(l.Logger)

	slog.Info("server started", "port", 8080)
	l.Debug("cache warmed", "entries", 1024)
}

// A Config can be loaded from a file or environment with any library that
// understands json, yaml or mapstructure tags.
func ExampleConfig() {
	data := []byte(
		`{"backend": "zerolog", "level": "warn", "file": {"path": "app.log", "max_age_days": 7}}`,
	)

	var cfg logger.Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		log.Fatal(err)
	}
	fmt.Printf("%+v\n", cfg)
	// Output: {Backend:zerolog Level:warn Format: DisableCaller:false Console: File:{Path:app.log MaxSizeMB:0 MaxAgeDays:7 MaxBackups:0 Compress:false}}
}

func ExampleLogger_SetLevel() {
	l, err := logger.New(logger.Config{Level: "info"})
	if err != nil {
		log.Fatal(err)
	}

	// For example, from an admin endpoint or a config reload.
	l.SetLevel(slog.LevelDebug)
	fmt.Println(l.Level())
	// Output: DEBUG
}

func ExampleContextWith() {
	l, err := logger.New(logger.Config{})
	if err != nil {
		log.Fatal(err)
	}

	handle := func(ctx context.Context) {
		// Includes request_id, without passing it around.
		l.InfoContext(ctx, "route found", "hops", 3)
	}
	ctx := logger.ContextWith(context.Background(), "request_id", "r-42")
	handle(ctx)
}

// A middleware adds the trace ID of every record's context, whatever the
// backend.
func ExampleWithMiddleware() {
	type traceKey struct{}

	traceID := func(next slog.Handler) slog.Handler {
		return traceHandler{Handler: next, id: func(ctx context.Context) (string, bool) {
			id, ok := ctx.Value(traceKey{}).(string)
			return id, ok
		}}
	}
	l, err := logger.New(
		logger.Config{Backend: logger.BackendZerolog},
		logger.WithMiddleware(traceID),
	)
	if err != nil {
		log.Fatal(err)
	}

	ctx := context.WithValue(context.Background(), traceKey{}, "4bf92f3577b34da6")
	l.InfoContext(ctx, "payment accepted")
}

type traceHandler struct {
	slog.Handler
	id func(context.Context) (string, bool)
}

func (h traceHandler) Handle(ctx context.Context, r slog.Record) error {
	if id, ok := h.id(ctx); ok {
		r.AddAttrs(slog.String("trace_id", id))
	}
	return h.Handler.Handle(ctx, r)
}

func (h traceHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return traceHandler{Handler: h.Handler.WithAttrs(attrs), id: h.id}
}

func (h traceHandler) WithGroup(name string) slog.Handler {
	return traceHandler{Handler: h.Handler.WithGroup(name), id: h.id}
}

// WithWriter sends console output anywhere, for example to a test's output or
// a network connection.
func ExampleWithWriter() {
	l, err := logger.New(logger.Config{Format: logger.FormatText}, logger.WithWriter(os.Stderr))
	if err != nil {
		log.Fatal(err)
	}
	l.Warn("disk almost full", "free_gb", 3)
}
