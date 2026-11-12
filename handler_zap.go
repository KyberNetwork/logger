package logger

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"runtime"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// zapHandler is a slog.Handler that writes through a zapcore.Core.
type zapHandler struct {
	core      zapcore.Core // carries the attributes added before the first group
	level     slog.Leveler
	addCaller bool
	groups    []group
}

func newZapHandler(w io.Writer, format Format, o handlerOptions) *zapHandler {
	cfg := zapcore.EncoderConfig{
		TimeKey:        TimeKey,
		LevelKey:       LevelKey,
		MessageKey:     MessageKey,
		CallerKey:      CallerKey,
		LineEnding:     zapcore.DefaultLineEnding,
		EncodeLevel:    zapcore.LowercaseLevelEncoder,
		EncodeTime:     zapcore.TimeEncoderOfLayout(timeLayout),
		EncodeDuration: zapcore.StringDurationEncoder,
		EncodeCaller:   zapcore.ShortCallerEncoder,
	}
	var enc zapcore.Encoder
	if format == FormatText {
		cfg.EncodeLevel = zapcore.CapitalLevelEncoder
		if o.color {
			cfg.EncodeLevel = zapcore.CapitalColorLevelEncoder
		}
		enc = zapcore.NewConsoleEncoder(cfg)
	} else {
		enc = zapcore.NewJSONEncoder(cfg)
	}
	// The handler filters by level; the core accepts everything it is given.
	core := zapcore.NewCore(enc, zapcore.Lock(zapcore.AddSync(w)), zapcore.DebugLevel)
	return &zapHandler{core: core, level: o.level, addCaller: o.addCaller}
}

func (h *zapHandler) Enabled(_ context.Context, level slog.Level) bool {
	return level >= h.level.Level()
}

func (h *zapHandler) Handle(_ context.Context, r slog.Record) error {
	ent := zapcore.Entry{
		Level:   zapLevel(r.Level),
		Time:    r.Time,
		Message: r.Message,
	}
	if h.addCaller && r.PC != 0 {
		f, _ := runtime.CallersFrames([]uintptr{r.PC}).Next()
		ent.Caller = zapcore.EntryCaller{
			Defined:  true,
			PC:       f.PC,
			File:     f.File,
			Line:     f.Line,
			Function: f.Function,
		}
	}
	attrs := recordAttrs(r)
	var fields []zapcore.Field
	if groups := nonEmptyGroups(h.groups, attrs); len(groups) > 0 {
		fields = []zapcore.Field{zap.Object(groups[0].name, zapGroup{groups: groups, attrs: attrs})}
	} else if len(attrs) > 0 {
		fields = []zapcore.Field{zap.Inline(zapAttrs(attrs))}
	}
	// The core accepts every level, so Check is skipped; unlike
	// CheckedEntry.Write, Core.Write returns write errors.
	return h.core.Write(ent, fields)
}

func (h *zapHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	attrs = resolveAttrs(attrs)
	if len(attrs) == 0 {
		return h
	}
	h2 := *h
	if len(h.groups) == 0 {
		h2.core = h.core.With([]zapcore.Field{zap.Inline(zapAttrs(attrs))})
	} else {
		h2.groups = withGroupAttrs(h.groups, attrs)
	}
	return &h2
}

func (h *zapHandler) WithGroup(name string) slog.Handler {
	if name == "" {
		return h
	}
	h2 := *h
	h2.groups = withGroup(h.groups, name)
	return &h2
}

func zapLevel(l slog.Level) zapcore.Level {
	switch {
	case l < slog.LevelInfo:
		return zapcore.DebugLevel
	case l < slog.LevelWarn:
		return zapcore.InfoLevel
	case l < slog.LevelError:
		return zapcore.WarnLevel
	default:
		return zapcore.ErrorLevel
	}
}

// zapAttrs encodes attributes into the current zap object.
type zapAttrs []slog.Attr

func (as zapAttrs) MarshalLogObject(enc zapcore.ObjectEncoder) error {
	for _, a := range as {
		addZapAttr(enc, a)
	}
	return nil
}

// zapGroup encodes groups[0] as an object holding its attributes and either
// the next group or, in the innermost group, the record's attributes.
type zapGroup struct {
	groups []group
	attrs  []slog.Attr
}

func (g zapGroup) MarshalLogObject(enc zapcore.ObjectEncoder) error {
	for _, a := range g.groups[0].attrs {
		addZapAttr(enc, a)
	}
	if len(g.groups) > 1 {
		return enc.AddObject(g.groups[1].name, zapGroup{groups: g.groups[1:], attrs: g.attrs})
	}
	for _, a := range g.attrs {
		addZapAttr(enc, a)
	}
	return nil
}

func addZapAttr(enc zapcore.ObjectEncoder, a slog.Attr) {
	switch v := a.Value; v.Kind() {
	case slog.KindGroup:
		if a.Key == "" {
			for _, ga := range v.Group() {
				addZapAttr(enc, ga)
			}
			return
		}
		_ = enc.AddObject(a.Key, zapAttrs(v.Group()))
	case slog.KindString:
		enc.AddString(a.Key, v.String())
	case slog.KindInt64:
		enc.AddInt64(a.Key, v.Int64())
	case slog.KindUint64:
		enc.AddUint64(a.Key, v.Uint64())
	case slog.KindFloat64:
		enc.AddFloat64(a.Key, v.Float64())
	case slog.KindBool:
		enc.AddBool(a.Key, v.Bool())
	case slog.KindDuration:
		enc.AddString(a.Key, v.Duration().String())
	case slog.KindTime:
		enc.AddString(a.Key, v.Time().Format(timeLayout))
	default:
		addZapAny(enc, a.Key, v.Any())
	}
}

func addZapAny(enc zapcore.ObjectEncoder, key string, v any) {
	s, raw := encodeAny(v)
	if raw == nil {
		enc.AddString(key, s)
		return
	}
	// zap's reflection encoder is encoding/json, which copies raw as is.
	if err := enc.AddReflected(key, json.RawMessage(raw)); err != nil {
		enc.AddString(key, "!ERROR:"+err.Error())
	}
}
