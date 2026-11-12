package logger

import (
	"errors"
	"fmt"
	"log/slog"
	"strings"
)

// Backend is the library that encodes and writes log records.
type Backend string

// Supported backends.
const (
	BackendSlog    Backend = "slog"    // log/slog's built-in handlers; the default
	BackendZap     Backend = "zap"     // go.uber.org/zap
	BackendZerolog Backend = "zerolog" // github.com/rs/zerolog
)

// Format is the encoding of log records.
type Format string

// Supported formats.
const (
	FormatJSON Format = "json" // one JSON object per line; the default
	FormatText Format = "text" // human-readable, colored on a terminal
)

// Console is the stream that console output goes to.
type Console string

// Supported console streams.
const (
	ConsoleStdout Console = "stdout" // the default
	ConsoleStderr Console = "stderr"
	ConsoleNone   Console = "none" // no console output
)

// Config describes a logger. The zero value logs JSON at info level to stdout
// with the slog backend.
//
// String fields are case-insensitive. The struct tags let Config be loaded
// with encoding/json, YAML libraries and mapstructure (viper).
type Config struct {
	// Backend selects the logging library. Default: BackendSlog.
	Backend Backend `json:"backend" yaml:"backend" mapstructure:"backend"`

	// Level is the minimum level logged: "debug", "info", "warn" (or
	// "warning") or "error". Default: "info". See [ParseLevel].
	Level string `json:"level" yaml:"level" mapstructure:"level"`

	// Format is the encoding of every output. Default: FormatJSON.
	Format Format `json:"format" yaml:"format" mapstructure:"format"`

	// DisableCaller omits the caller's file and line from records.
	DisableCaller bool `json:"disable_caller" yaml:"disable_caller" mapstructure:"disable_caller"`

	// Console is where console output goes. Default: ConsoleStdout.
	Console Console `json:"console" yaml:"console" mapstructure:"console"`

	// File adds a rotated log file as a second output.
	File FileConfig `json:"file" yaml:"file" mapstructure:"file"`
}

// FileConfig describes a log file output, rotated by
// [gopkg.in/natefinch/lumberjack.v2]. Zero values use lumberjack's defaults.
type FileConfig struct {
	// Path is the file to write to. The file output is enabled when Path is
	// not empty. Missing directories are created.
	Path string `json:"path" yaml:"path" mapstructure:"path"`

	// MaxSizeMB is the size in megabytes at which the file is rotated.
	// Default: 100.
	MaxSizeMB int `json:"max_size_mb" yaml:"max_size_mb" mapstructure:"max_size_mb"`

	// MaxAgeDays is how many days rotated files are kept. Default: forever.
	MaxAgeDays int `json:"max_age_days" yaml:"max_age_days" mapstructure:"max_age_days"`

	// MaxBackups is how many rotated files are kept. Default: all.
	MaxBackups int `json:"max_backups" yaml:"max_backups" mapstructure:"max_backups"`

	// Compress gzips rotated files.
	Compress bool `json:"compress" yaml:"compress" mapstructure:"compress"`
}

// settings is a validated Config with defaults applied.
type settings struct {
	backend   Backend
	level     slog.Level
	format    Format
	addCaller bool
	console   Console
	file      FileConfig
}

func (c Config) settings() (settings, error) {
	s := settings{
		backend:   Backend(normalize(string(c.Backend), string(BackendSlog))),
		format:    Format(normalize(string(c.Format), string(FormatJSON))),
		console:   Console(normalize(string(c.Console), string(ConsoleStdout))),
		addCaller: !c.DisableCaller,
		file:      c.File,
	}

	var errs []error
	switch s.backend {
	case BackendSlog, BackendZap, BackendZerolog:
	default:
		errs = append(errs, fmt.Errorf("logger: unknown backend %q (want slog, zap or zerolog)", c.Backend))
	}
	switch s.format {
	case FormatJSON, FormatText:
	default:
		errs = append(errs, fmt.Errorf("logger: unknown format %q (want json or text)", c.Format))
	}
	switch s.console {
	case ConsoleStdout, ConsoleStderr, ConsoleNone:
	default:
		errs = append(errs, fmt.Errorf("logger: unknown console %q (want stdout, stderr or none)", c.Console))
	}
	if strings.TrimSpace(c.Level) == "" {
		s.level = slog.LevelInfo
	} else if level, err := ParseLevel(c.Level); err != nil {
		errs = append(errs, err)
	} else {
		s.level = level
	}
	if c.File.MaxSizeMB < 0 || c.File.MaxAgeDays < 0 || c.File.MaxBackups < 0 {
		errs = append(errs, errors.New("logger: file rotation limits must not be negative"))
	}
	return s, errors.Join(errs...)
}

func normalize(s, def string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	if s == "" {
		return def
	}
	return s
}
