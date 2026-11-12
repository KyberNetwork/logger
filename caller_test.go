package logger

import (
	"log"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// here returns the location of its caller in the format of the caller field.
// The caller tests log here() as the message, on the same line as the log
// call, so the message must equal the caller.
func here() string {
	_, file, line, _ := runtime.Caller(1)
	return trimmedPath(file, line)
}

var callPaths = []struct {
	name string
	log  func(*testing.T, *slog.Logger)
}{
	{"Info", func(_ *testing.T, l *slog.Logger) { l.Info(here()) }},
	{"InfoContext", func(t *testing.T, l *slog.Logger) { l.InfoContext(t.Context(), here()) }},
	{"Log", func(t *testing.T, l *slog.Logger) { l.Log(t.Context(), slog.LevelWarn, here()) }},
	{
		"LogAttrs",
		func(t *testing.T, l *slog.Logger) { l.LogAttrs(t.Context(), slog.LevelError, here()) },
	},
	{"With", func(_ *testing.T, l *slog.Logger) { l.With("a", 1).Info(here()) }},
	{"WithGroup", func(_ *testing.T, l *slog.Logger) { l.WithGroup("g").Info(here(), "a", 1) }},
	{
		"ContextWith",
		func(t *testing.T, l *slog.Logger) { l.InfoContext(ContextWith(t.Context(), "a", 1), here()) },
	},
	{"slog.Info", func(t *testing.T, l *slog.Logger) {
		setDefault(t, l, log.LstdFlags)
		slog.Info(here())
	}},
	{"slog.Default", func(t *testing.T, l *slog.Logger) {
		setDefault(t, l, log.LstdFlags)
		slog.Default().With("a", 1).Info(here())
	}},
	{"log.Print", func(t *testing.T, l *slog.Logger) {
		// slog records the caller of the log package only if the log flags
		// ask for a file name when slog.SetDefault is called.
		setDefault(t, l, log.Lshortfile)
		log.Print(here())
	}},
}

func TestCaller(t *testing.T) {
	for _, b := range backends {
		for _, p := range callPaths {
			t.Run(string(b)+"/"+p.name, func(t *testing.T) {
				l, buf := newTestLogger(t, b)
				p.log(t, l.Logger)
				checkCaller(t, buf.record(t))
			})
		}
	}
}

// TestCallerFanOutAndMiddleware checks the caller written to the console and
// the file, through middleware.
func TestCallerFanOutAndMiddleware(t *testing.T) {
	for _, b := range backends {
		t.Run(string(b), func(t *testing.T) {
			var console buffer
			path := filepath.Join(t.TempDir(), "app.log")
			l, err := New(Config{Backend: b, File: FileConfig{Path: path}},
				WithWriter(&console),
				WithMiddleware(passThrough, passThrough),
			)
			if err != nil {
				t.Fatal(err)
			}
			l.With("a", 1).InfoContext(ContextWith(t.Context(), "b", 2), here())
			if err := l.Close(); err != nil {
				t.Fatal(err)
			}

			checkCaller(t, console.record(t))
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			checkCaller(t, decode(t, strings.TrimSpace(string(data))))
		})
	}
}

func TestDisableCaller(t *testing.T) {
	for _, b := range backends {
		t.Run(string(b), func(t *testing.T) {
			var buf buffer
			l, err := New(Config{Backend: b, DisableCaller: true}, WithWriter(&buf))
			if err != nil {
				t.Fatal(err)
			}
			l.Info("m")
			if v, ok := buf.record(t)[CallerKey]; ok {
				t.Errorf("caller = %v, want none", v)
			}
		})
	}
}

// TestNoPC checks that records without a PC, such as those of log.Print by
// default, have no caller.
func TestNoPC(t *testing.T) {
	for _, b := range backends {
		t.Run(string(b), func(t *testing.T) {
			l, buf := newTestLogger(t, b)
			setDefault(t, l.Logger, log.LstdFlags)
			log.Print("m")
			if v, ok := buf.record(t)[CallerKey]; ok {
				t.Errorf("caller = %v, want none", v)
			}
		})
	}
}

func checkCaller(t *testing.T, rec map[string]any) {
	t.Helper()
	msg, _ := rec[MessageKey].(string)
	if !strings.Contains(msg, "/caller_test.go:") {
		t.Fatalf("message %q is not a location in caller_test.go", msg)
	}
	if rec[CallerKey] != msg {
		t.Errorf("caller = %v, want %s", rec[CallerKey], msg)
	}
}

// setDefault makes l the default logger, with the given flags for the log
// package, until the end of the test.
func setDefault(t *testing.T, l *slog.Logger, logFlags int) {
	prev, w, flags := slog.Default(), log.Writer(), log.Flags()
	t.Cleanup(func() {
		slog.SetDefault(prev)
		log.SetOutput(w)
		log.SetFlags(flags)
	})
	log.SetFlags(logFlags)
	slog.SetDefault(l)
}

func passThrough(next slog.Handler) slog.Handler {
	return mapHandler{next: next, f: func(r slog.Record) slog.Record { return r }}
}
