package logger

import (
	"context"
	"io"
	"log/slog"
	"math"
)

// handlerOptions are the settings shared by every backend handler.
type handlerOptions struct {
	level     slog.Leveler
	addCaller bool
	color     bool // text format only; ignored by the slog backend
}

func newSlogHandler(w io.Writer, format Format, o handlerOptions) slog.Handler {
	opts := &slog.HandlerOptions{
		AddSource:   o.addCaller,
		Level:       o.level,
		ReplaceAttr: replaceAttr,
	}
	if format == FormatText {
		return slogHandler{slog.NewTextHandler(w, opts)}
	}
	return slogHandler{slog.NewJSONHandler(w, opts)}
}

// slogHandler removes empty attributes from groups before passing them to a
// handler of package slog. In a JSON object, slog writes a group whose
// attributes all turn out empty as {} if they are LogValuers, and drops the
// comma before the next attribute otherwise.
type slogHandler struct {
	slog.Handler
}

func (h slogHandler) Handle(ctx context.Context, r slog.Record) error {
	if hasGroupOrValuer(r) {
		pruned := slog.NewRecord(r.Time, r.Level, r.Message, r.PC)
		pruned.AddAttrs(recordAttrs(r)...)
		r = pruned
	}
	return h.Handler.Handle(ctx, r)
}

func (h slogHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return slogHandler{h.Handler.WithAttrs(resolveAttrs(attrs))}
}

func (h slogHandler) WithGroup(name string) slog.Handler {
	return slogHandler{h.Handler.WithGroup(name)}
}

func hasGroupOrValuer(r slog.Record) bool {
	found := false
	r.Attrs(func(a slog.Attr) bool {
		k := a.Value.Kind()
		found = k == slog.KindGroup || k == slog.KindLogValuer
		return !found
	})
	return found
}

// replaceAttr maps slog's built-in output onto the package's format. The zap
// and zerolog handlers produce the same result directly.
func replaceAttr(groups []string, a slog.Attr) slog.Attr {
	switch a.Value.Kind() {
	case slog.KindTime:
		return slog.String(a.Key, a.Value.Time().Format(timeLayout))
	case slog.KindDuration:
		return slog.String(a.Key, a.Value.Duration().String())
	case slog.KindFloat64:
		// JSON has no NaN or infinities; write them as zap and zerolog do.
		if f := a.Value.Float64(); math.IsNaN(f) || math.IsInf(f, 0) {
			return slog.String(a.Key, nonFiniteString(f))
		}
	case slog.KindAny:
		if len(groups) > 0 {
			break
		}
		switch v := a.Value.Any().(type) {
		case slog.Level:
			if a.Key == slog.LevelKey {
				return slog.String(LevelKey, levelName(v))
			}
		case *slog.Source:
			if a.Key == slog.SourceKey {
				if v.File == "" {
					return slog.Attr{} // the record has no PC
				}
				return slog.String(CallerKey, trimmedPath(v.File, v.Line))
			}
		}
	}
	return a
}

func nonFiniteString(f float64) string {
	switch {
	case math.IsNaN(f):
		return "NaN"
	case f > 0:
		return "+Inf"
	default:
		return "-Inf"
	}
}
