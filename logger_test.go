package logger

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func TestLevel(t *testing.T) {
	for _, b := range backends {
		t.Run(string(b), func(t *testing.T) {
			var buf buffer
			l, err := New(Config{Backend: b, Level: "warn"}, WithWriter(&buf))
			if err != nil {
				t.Fatal(err)
			}
			child := l.With("a", 1).WithGroup("g")

			l.Info("dropped")
			child.Info("dropped")
			l.Warn("kept")
			if got := len(buf.lines()); got != 1 {
				t.Fatalf("got %d records at warn level, want 1:\n%s", got, buf.String())
			}

			l.SetLevel(slog.LevelDebug)
			if l.Level() != slog.LevelDebug {
				t.Errorf("Level() = %v, want debug", l.Level())
			}
			child.Debug("kept")
			if got := len(buf.lines()); got != 2 {
				t.Fatalf("got %d records after SetLevel, want 2:\n%s", got, buf.String())
			}
		})
	}
}

func TestSetLevelConcurrent(t *testing.T) {
	for _, b := range backends {
		t.Run(string(b), func(t *testing.T) {
			l, buf := newTestLogger(t, b)
			var wg sync.WaitGroup
			for i := range 8 {
				wg.Go(func() {
					child := l.With("goroutine", i)
					for j := range 100 {
						child.Info("m", "j", j)
					}
				})
			}
			wg.Go(func() {
				for i := range 100 {
					l.SetLevel(slog.Level(i%2) * slog.LevelWarn)
				}
			})
			wg.Wait()
			for _, line := range buf.lines() {
				decode(t, line) // every line is a whole record
			}
		})
	}
}

func TestFile(t *testing.T) {
	for _, b := range backends {
		t.Run(string(b), func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "nested", "dir", "app.log")
			var console buffer
			l, err := New(Config{Backend: b, File: FileConfig{Path: path}}, WithWriter(&console))
			if err != nil {
				t.Fatal(err)
			}
			if _, err := os.Stat(path); err != nil {
				t.Fatalf("New did not create the log file: %v", err)
			}
			l.Info("m", "a", 1)
			if err := l.Close(); err != nil {
				t.Fatal(err)
			}

			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if got, want := string(data), console.String(); got != want {
				t.Errorf("file and console differ:\nfile    %q\nconsole %q", got, want)
			}
		})
	}
}

func TestFileError(t *testing.T) {
	dir := t.TempDir()
	notDir := filepath.Join(dir, "file")
	if err := os.WriteFile(notDir, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := New(Config{File: FileConfig{Path: filepath.Join(notDir, "app.log")}})
	if err == nil || !strings.HasPrefix(err.Error(), "logger: open log file") {
		t.Errorf("New = %v, want an error opening the file", err)
	}
}

func TestConsoleNone(t *testing.T) {
	l, err := New(Config{Console: ConsoleNone})
	if err != nil {
		t.Fatal(err)
	}
	if l.Enabled(t.Context(), slog.LevelError) {
		t.Error("a logger without outputs is enabled")
	}
	l.Error("m")
	if err := l.Close(); err != nil {
		t.Error(err)
	}
}

func TestTextFormat(t *testing.T) {
	for _, b := range backends {
		t.Run(string(b), func(t *testing.T) {
			var buf buffer
			l, err := New(Config{Backend: b, Format: FormatText}, WithWriter(&buf))
			if err != nil {
				t.Fatal(err)
			}
			l.Info("hello world", "answer", 42)
			out := buf.String()
			if strings.Count(out, "\n") != 1 {
				t.Errorf("want one line, got %q", out)
			}
			for _, want := range []string{"hello world", "answer", "42", "logger_test.go:"} {
				if !strings.Contains(out, want) {
					t.Errorf("output %q does not contain %q", out, want)
				}
			}
			if strings.Contains(out, "\x1b[") {
				t.Errorf("output to a buffer is colored: %q", out)
			}
		})
	}
}

func TestTextColor(t *testing.T) {
	for _, b := range []Backend{BackendZap, BackendZerolog} {
		t.Run(string(b), func(t *testing.T) {
			var buf bytes.Buffer
			h := newHandler(b, &buf, FormatText, handlerOptions{level: slog.LevelInfo, color: true})
			slog.New(h).Info("m")
			if !strings.Contains(buf.String(), "\x1b[") {
				t.Errorf("output is not colored: %q", buf.String())
			}
		})
	}
}

func TestUseColor(t *testing.T) {
	f, err := os.CreateTemp(t.TempDir(), "out")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = f.Close() })
	if useColor(f) {
		t.Error("useColor(regular file) = true")
	}
	if useColor(&bytes.Buffer{}) {
		t.Error("useColor(buffer) = true")
	}
}

func TestMiddleware(t *testing.T) {
	var calls []string
	record := func(name string) Middleware {
		return func(next slog.Handler) slog.Handler {
			return mapHandler{next: next, f: func(r slog.Record) slog.Record {
				calls = append(calls, name)
				var keys []string
				r.Attrs(func(a slog.Attr) bool {
					keys = append(keys, a.Key)
					return true
				})
				if got := strings.Join(keys, ","); got != "a,request_id" {
					t.Errorf("%s saw attributes %s, want a,request_id", name, got)
				}
				return r
			}}
		}
	}
	l, buf := newTestLogger(t, BackendSlog, WithMiddleware(record("first"), record("second")), WithMiddleware(record("third")))
	l.InfoContext(ContextWith(t.Context(), "request_id", "r1"), "m", "a", 1)

	if got := strings.Join(calls, ","); got != "first,second,third" {
		t.Errorf("middleware ran in order %s, want first,second,third", got)
	}
	if rec := buf.record(t); rec["request_id"] != "r1" {
		t.Errorf("record = %v, want request_id r1", rec)
	}
}

func TestNilOptions(t *testing.T) {
	var buf buffer
	l, err := New(Config{}, nil, WithWriter(&buf), WithWriter(nil), WithMiddleware(), WithMiddleware(nil))
	if err != nil {
		t.Fatal(err)
	}
	l.Info("m")
	if len(buf.lines()) != 1 {
		t.Errorf("got %q, want one record", buf.String())
	}
}

type failingWriter struct{}

func (failingWriter) Write([]byte) (int, error) { return 0, errors.New("disk full") }

func TestHandleWriteError(t *testing.T) {
	// zerolog reports write errors to zerolog.ErrorHandler instead.
	for _, b := range []Backend{BackendSlog, BackendZap} {
		t.Run(string(b), func(t *testing.T) {
			l, err := New(Config{Backend: b}, WithWriter(failingWriter{}))
			if err != nil {
				t.Fatal(err)
			}
			r := slog.NewRecord(fixedTime, slog.LevelInfo, "m", 0)
			if err := l.Handler().Handle(context.Background(), r); err == nil {
				t.Error("Handle succeeded, want the write error")
			}
		})
	}
}
