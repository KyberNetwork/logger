package logger

import (
	"encoding/json"
	"log/slog"
	"strings"
	"testing"
)

func TestParseLevel(t *testing.T) {
	tests := []struct {
		in   string
		want slog.Level
	}{
		{"debug", slog.LevelDebug},
		{"INFO", slog.LevelInfo},
		{" Warn ", slog.LevelWarn},
		{"warning", slog.LevelWarn},
		{"error", slog.LevelError},
		{"info+2", slog.LevelInfo + 2},
		{"DEBUG-4", slog.LevelDebug - 4},
	}
	for _, tt := range tests {
		got, err := ParseLevel(tt.in)
		if err != nil || got != tt.want {
			t.Errorf("ParseLevel(%q) = %v, %v; want %v", tt.in, got, err, tt.want)
		}
	}
	for _, in := range []string{"", "trace", "fatal", "info+", "3"} {
		if _, err := ParseLevel(in); err == nil {
			t.Errorf("ParseLevel(%q) succeeded, want an error", in)
		}
	}
}

func TestLevelName(t *testing.T) {
	tests := []struct {
		in   slog.Level
		want string
	}{
		{slog.LevelDebug - 4, "debug"},
		{slog.LevelDebug, "debug"},
		{slog.LevelInfo - 1, "debug"},
		{slog.LevelInfo, "info"},
		{slog.LevelInfo + 2, "info"},
		{slog.LevelWarn, "warn"},
		{slog.LevelError, "error"},
		{slog.LevelError + 4, "error"},
	}
	for _, tt := range tests {
		if got := levelName(tt.in); got != tt.want {
			t.Errorf("levelName(%v) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestConfigDefaults(t *testing.T) {
	s, err := Config{}.settings()
	if err != nil {
		t.Fatal(err)
	}
	want := settings{
		backend:   BackendSlog,
		level:     slog.LevelInfo,
		format:    FormatJSON,
		addCaller: true,
		console:   ConsoleStdout,
	}
	if s != want {
		t.Errorf("settings = %+v, want %+v", s, want)
	}
}

func TestConfigCaseInsensitive(t *testing.T) {
	s, err := Config{
		Backend: " Zap",
		Level:   "WARNING",
		Format:  "TEXT",
		Console: "Stderr",
	}.settings()
	if err != nil {
		t.Fatal(err)
	}
	if s.backend != BackendZap || s.level != slog.LevelWarn || s.format != FormatText ||
		s.console != ConsoleStderr {
		t.Errorf("settings = %+v", s)
	}
}

func TestConfigErrors(t *testing.T) {
	_, err := New(Config{
		Backend: "logrus",
		Level:   "loud",
		Format:  "xml",
		Console: "printer",
		File:    FileConfig{Path: "x.log", MaxSizeMB: -1},
	})
	if err == nil {
		t.Fatal("New succeeded, want an error")
	}
	for _, want := range []string{`backend "logrus"`, `level "loud"`, `format "xml"`, `console "printer"`, "negative"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not mention %s", err, want)
		}
	}
}

func TestConfigUnmarshal(t *testing.T) {
	const data = `{
		"backend": "zerolog",
		"level": "debug",
		"format": "text",
		"disable_caller": true,
		"console": "stderr",
		"file": {"path": "/var/log/app.log", "max_size_mb": 50, "max_age_days": 7, "max_backups": 3, "compress": true}
	}`
	var got Config
	if err := json.Unmarshal([]byte(data), &got); err != nil {
		t.Fatal(err)
	}
	want := Config{
		Backend:       BackendZerolog,
		Level:         "debug",
		Format:        FormatText,
		DisableCaller: true,
		Console:       ConsoleStderr,
		File: FileConfig{
			Path:       "/var/log/app.log",
			MaxSizeMB:  50,
			MaxAgeDays: 7,
			MaxBackups: 3,
			Compress:   true,
		},
	}
	if got != want {
		t.Errorf("got %+v\nwant %+v", got, want)
	}
}
