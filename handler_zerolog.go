package logger

import (
	"context"
	"io"
	"log/slog"

	"github.com/rs/zerolog"
)

// zerologHandler is a slog.Handler that writes through a zerolog.Logger.
//
// It writes the built-in fields itself instead of using zerolog's level,
// timestamp and message helpers, which depend on zerolog's package-level
// settings. Events are therefore logged with zerolog.NoLevel.
type zerologHandler struct {
	logger    zerolog.Logger // its context carries the attributes added before the first group
	level     slog.Leveler
	addCaller bool
	keys      zerologKeys
	groups    []group
}

type zerologKeys struct {
	time, level, msg, caller string
}

func newZerologHandler(w io.Writer, format Format, o handlerOptions) *zerologHandler {
	keys := zerologKeys{time: TimeKey, level: LevelKey, msg: MessageKey, caller: CallerKey}
	if format == FormatText {
		// ConsoleWriter finds the built-in fields by zerolog's field names.
		keys = zerologKeys{
			time:   zerolog.TimestampFieldName,
			level:  zerolog.LevelFieldName,
			msg:    zerolog.MessageFieldName,
			caller: zerolog.CallerFieldName,
		}
		w = zerolog.ConsoleWriter{Out: w, NoColor: !o.color, TimeFormat: timeLayout}
	}
	return &zerologHandler{
		logger:    zerolog.New(zerolog.SyncWriter(w)),
		level:     o.level,
		addCaller: o.addCaller,
		keys:      keys,
	}
}

func (h *zerologHandler) Enabled(_ context.Context, level slog.Level) bool {
	return level >= h.level.Level()
}

func (h *zerologHandler) Handle(_ context.Context, r slog.Record) error {
	e := h.logger.Log()
	if e == nil {
		return nil
	}
	if !r.Time.IsZero() {
		e.Str(h.keys.time, r.Time.Format(timeLayout))
	}
	e.Str(h.keys.level, levelName(r.Level))
	if h.addCaller && r.PC != 0 {
		e.Str(h.keys.caller, callerOf(r.PC))
	}
	e.Str(h.keys.msg, r.Message)

	attrs := recordAttrs(r)
	if groups := nonEmptyGroups(h.groups, attrs); len(groups) > 0 {
		addZerologGroup(e, groups, attrs)
	} else {
		addZerologAttrs(e, attrs)
	}
	e.Send()
	return nil
}

func (h *zerologHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	attrs = resolveAttrs(attrs)
	if len(attrs) == 0 {
		return h
	}
	h2 := *h
	if len(h.groups) == 0 {
		h2.logger = h.logger.With().EmbedObject(zerologAttrs(attrs)).Logger()
	} else {
		h2.groups = withGroupAttrs(h.groups, attrs)
	}
	return &h2
}

func (h *zerologHandler) WithGroup(name string) slog.Handler {
	if name == "" {
		return h
	}
	h2 := *h
	h2.groups = withGroup(h.groups, name)
	return &h2
}

// zerologAttrs embeds attributes into a zerolog context.
type zerologAttrs []slog.Attr

func (as zerologAttrs) MarshalZerologObject(e *zerolog.Event) {
	addZerologAttrs(e, as)
}

// addZerologGroup writes groups[0] as a dictionary holding its attributes and
// either the next group or, in the innermost group, the record's attributes.
func addZerologGroup(e *zerolog.Event, groups []group, attrs []slog.Attr) {
	d := e.CreateDict()
	addZerologAttrs(d, groups[0].attrs)
	if len(groups) > 1 {
		addZerologGroup(d, groups[1:], attrs)
	} else {
		addZerologAttrs(d, attrs)
	}
	e.Dict(groups[0].name, d)
}

func addZerologAttrs(e *zerolog.Event, attrs []slog.Attr) {
	for _, a := range attrs {
		addZerologAttr(e, a)
	}
}

func addZerologAttr(e *zerolog.Event, a slog.Attr) {
	switch v := a.Value; v.Kind() {
	case slog.KindGroup:
		if a.Key == "" {
			addZerologAttrs(e, v.Group())
			return
		}
		d := e.CreateDict()
		addZerologAttrs(d, v.Group())
		e.Dict(a.Key, d)
	case slog.KindString:
		e.Str(a.Key, v.String())
	case slog.KindInt64:
		e.Int64(a.Key, v.Int64())
	case slog.KindUint64:
		e.Uint64(a.Key, v.Uint64())
	case slog.KindFloat64:
		e.Float64(a.Key, v.Float64())
	case slog.KindBool:
		e.Bool(a.Key, v.Bool())
	case slog.KindDuration:
		e.Str(a.Key, v.Duration().String())
	case slog.KindTime:
		e.Str(a.Key, v.Time().Format(timeLayout))
	default:
		addZerologAny(e, a.Key, v.Any())
	}
}

func addZerologAny(e *zerolog.Event, key string, v any) {
	if s, raw := encodeAny(v); raw == nil {
		e.Str(key, s)
	} else {
		e.RawJSON(key, raw)
	}
}
