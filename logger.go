package logger

import (
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"

	"gopkg.in/natefinch/lumberjack.v2"
)

// Logger is a *slog.Logger built by [New], with control over its level and
// outputs.
//
// Pass the embedded *slog.Logger to code that logs, or install it with
// slog.SetDefault. Only the owner of the Logger, typically main, needs the
// Logger itself, to change the level or to close the outputs.
type Logger struct {
	*slog.Logger

	level   *slog.LevelVar
	closers []io.Closer
}

// Option customizes a logger built by [New].
type Option func(*options)

type options struct {
	writer     io.Writer
	middleware []Middleware
}

// Middleware wraps the handler of a logger, to filter, enrich or redirect
// records whatever the backend. Records reach a middleware with the
// attributes from [ContextWith] already added.
type Middleware func(slog.Handler) slog.Handler

// WithWriter sends console output to w instead of the stream selected by
// [Config].Console. A nil w is ignored. The text format is colored only if w is
// a terminal.
func WithWriter(w io.Writer) Option {
	return func(o *options) {
		if w != nil {
			o.writer = w
		}
	}
}

// WithMiddleware wraps the logger's handler with mw. The first middleware is
// the outermost, so it sees records first.
func WithMiddleware(mw ...Middleware) Option {
	return func(o *options) {
		o.middleware = append(o.middleware, mw...)
	}
}

// New builds a logger from cfg. It reports every invalid field of cfg in one
// error, and fails if the log file can't be opened.
//
// New does not change any global state; call slog.SetDefault to make the
// logger the default.
func New(cfg Config, opts ...Option) (*Logger, error) {
	s, err := cfg.settings()
	if err != nil {
		return nil, err
	}
	var o options
	for _, opt := range opts {
		if opt != nil {
			opt(&o)
		}
	}

	level := new(slog.LevelVar)
	level.Set(s.level)
	ho := handlerOptions{level: level, addCaller: s.addCaller}

	var (
		handlers []slog.Handler
		closers  []io.Closer
	)
	if w := consoleWriter(s.console, o.writer); w != nil {
		consoleOpts := ho
		consoleOpts.color = s.format == FormatText && useColor(w)
		handlers = append(handlers, newHandler(s.backend, w, s.format, consoleOpts))
	}
	if s.file.Path != "" {
		f := &lumberjack.Logger{
			Filename:   s.file.Path,
			MaxSize:    s.file.MaxSizeMB,
			MaxAge:     s.file.MaxAgeDays,
			MaxBackups: s.file.MaxBackups,
			Compress:   s.file.Compress,
		}
		// lumberjack opens the file on the first write; do it now to report
		// errors from New rather than lose records later.
		if _, err := f.Write(nil); err != nil {
			return nil, fmt.Errorf("logger: open log file: %w", err)
		}
		closers = append(closers, f)
		handlers = append(handlers, newHandler(s.backend, f, s.format, ho))
	}

	var h slog.Handler
	switch len(handlers) {
	case 0:
		h = slog.DiscardHandler
	case 1:
		h = handlers[0]
	default:
		h = slog.NewMultiHandler(handlers...)
	}
	for i := len(o.middleware) - 1; i >= 0; i-- {
		if mw := o.middleware[i]; mw != nil {
			h = mw(h)
		}
	}
	h = contextHandler{next: h}

	return &Logger{Logger: slog.New(h), level: level, closers: closers}, nil
}

// Level returns the minimum level logged.
func (l *Logger) Level() slog.Level {
	return l.level.Level()
}

// SetLevel changes the minimum level logged, for every output and every logger
// derived from l. It is safe to call concurrently with logging.
func (l *Logger) SetLevel(level slog.Level) {
	l.level.Set(level)
}

// Close closes the log file, if any. Call it when the program stops logging,
// typically deferred in main. Console streams are left open. A record logged
// after Close reopens the file.
func (l *Logger) Close() error {
	errs := make([]error, 0, len(l.closers))
	for _, c := range l.closers {
		errs = append(errs, c.Close())
	}
	return errors.Join(errs...)
}

func newHandler(b Backend, w io.Writer, f Format, o handlerOptions) slog.Handler {
	switch b {
	case BackendZap:
		return newZapHandler(w, f, o)
	case BackendZerolog:
		return newZerologHandler(w, f, o)
	default:
		return newSlogHandler(w, f, o)
	}
}

func consoleWriter(c Console, override io.Writer) io.Writer {
	if override != nil {
		return override
	}
	switch c {
	case ConsoleStderr:
		return os.Stderr
	case ConsoleNone:
		return nil
	default:
		return os.Stdout
	}
}

// useColor reports whether text written to w should be colored: w is a
// terminal and the NO_COLOR convention (https://no-color.org) is not in use.
func useColor(w io.Writer) bool {
	if os.Getenv("NO_COLOR") != "" {
		return false
	}
	f, ok := w.(*os.File)
	if !ok {
		return false
	}
	fi, err := f.Stat()
	return err == nil && fi.Mode()&os.ModeCharDevice != 0
}
