package logger

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"math"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"
)

// The conformance test checks that every backend writes the same JSON for the
// same log call.

var fixedTime = time.Date(2026, 11, 12, 14, 35, 3, 189_654_321, time.FixedZone("ICT", 7*60*60))

const fixedTimeJSON = "2026-11-12T14:35:03.189+07:00"

type textAndString struct{}

func (textAndString) String() string               { return "STRING" }
func (textAndString) MarshalText() ([]byte, error) { return []byte("TEXT"), nil }

// formatterError has a verbose %+v form, like errors from github.com/pkg/errors.
type formatterError struct{}

func (formatterError) Error() string                 { return "boom" }
func (formatterError) Format(s fmt.State, verb rune) { _, _ = io.WriteString(s, "VERBOSE boom") }

type jsonError struct{}

func (jsonError) Error() string                { return "boom" }
func (jsonError) MarshalJSON() ([]byte, error) { return []byte(`{"code":42}`), nil }

type ptrError struct{ msg string }

func (e *ptrError) Error() string { return e.msg }

type failingMarshaler struct{}

func (failingMarshaler) MarshalJSON() ([]byte, error) { return nil, errors.New("nope") }

type panickingMarshaler struct{}

func (panickingMarshaler) MarshalJSON() ([]byte, error) { panic("kaboom") }

type user struct{ id string }

func (u user) LogValue() slog.Value {
	return slog.GroupValue(slog.String("id", u.id), slog.Int("n", 2))
}

type emptyValuer struct{}

func (emptyValuer) LogValue() slog.Value { return slog.GroupValue() }

var conformanceCases = []struct {
	name string
	log  func(context.Context, *slog.Logger)
	want string // the record without "time" and "caller"
}{
	{
		name: "scalars",
		log: func(ctx context.Context, l *slog.Logger) {
			l.InfoContext(ctx, "m", "s", "x", "i", math.MinInt64, "u", uint64(math.MaxUint64), "f", 1.5, "b", true)
		},
		want: `{"level":"info","msg":"m","s":"x","i":-9223372036854775808,"u":18446744073709551615,"f":1.5,"b":true}`,
	},
	{
		name: "escaping",
		log: func(ctx context.Context, l *slog.Logger) {
			l.InfoContext(ctx, "a \"quoted\"\nmessage", "s", "<tag> & \t ünïcode \u2028 \x00 \xff")
		},
		want: `{"level":"info","msg":"a \"quoted\"\nmessage","s":"<tag> & \t ünïcode \u2028 \u0000 \ufffd"}`,
	},
	{
		name: "non-finite floats",
		log: func(ctx context.Context, l *slog.Logger) {
			l.InfoContext(ctx, "m", "nan", math.NaN(), "inf", math.Inf(1), "ninf", math.Inf(-1))
		},
		want: `{"level":"info","msg":"m","nan":"NaN","inf":"+Inf","ninf":"-Inf"}`,
	},
	{
		name: "durations and times",
		log: func(ctx context.Context, l *slog.Logger) {
			l.InfoContext(ctx, "m", "d", 1500*time.Millisecond, "t", fixedTime, slog.Group("g", "d", time.Minute))
		},
		want: `{"level":"info","msg":"m","d":"1.5s","t":"` + fixedTimeJSON + `","g":{"d":"1m0s"}}`,
	},
	{
		name: "any values",
		log: func(ctx context.Context, l *slog.Logger) {
			l.InfoContext(ctx, "m",
				"text", textAndString{},
				"bytes", []byte("hi"),
				"nil", nil,
				"map", map[string]any{"a": 1, "html": "<b>"},
				"slice", []int{1, 2},
				"lvl", slog.LevelWarn+1,
			)
		},
		want: `{"level":"info","msg":"m","text":"TEXT","bytes":"aGk=","nil":null,"map":{"a":1,"html":"<b>"},"slice":[1,2],"lvl":"WARN+1"}`,
	},
	{
		name: "errors",
		log: func(ctx context.Context, l *slog.Logger) {
			l.ErrorContext(ctx, "m",
				"err", errors.New("plain"),
				"wrapped", fmt.Errorf("ctx: %w", errors.New("inner")),
				"formatter", formatterError{},
				"json", jsonError{},
				"typed_nil", (*ptrError)(nil),
			)
		},
		want: `{"level":"error","msg":"m","err":"plain","wrapped":"ctx: inner","formatter":"boom","json":{"code":42},"typed_nil":"<nil>"}`,
	},
	{
		name: "marshal failures",
		log: func(ctx context.Context, l *slog.Logger) {
			l.InfoContext(ctx, "m", "failing", failingMarshaler{}, "panicking", panickingMarshaler{})
		},
		want: `{"level":"info","msg":"m",` +
			`"failing":"!ERROR:json: error calling MarshalJSON for type *logger.failingMarshaler: nope",` +
			`"panicking":"!PANIC: kaboom"}`,
	},
	{
		name: "log valuers",
		log: func(ctx context.Context, l *slog.Logger) {
			l.With("owner", user{"u1"}).InfoContext(ctx, "m", "user", user{"u2"}, "empty", emptyValuer{})
		},
		want: `{"level":"info","msg":"m","owner":{"id":"u1","n":2},"user":{"id":"u2","n":2}}`,
	},
	{
		name: "groups",
		log: func(ctx context.Context, l *slog.Logger) {
			l.InfoContext(ctx, "m",
				slog.Group("g", "a", 1, slog.Group("h", "b", 2)),
				slog.Group("", "inline", 3),
				slog.Group("empty"),
				slog.Group("outer", slog.Group("inner")),
				slog.Attr{},
			)
		},
		want: `{"level":"info","msg":"m","g":{"a":1,"h":{"b":2}},"inline":3}`,
	},
	{
		name: "with and with group",
		log: func(ctx context.Context, l *slog.Logger) {
			l.With("a", 1).WithGroup("g").With("b", 2).WithGroup("h").InfoContext(ctx, "m", "c", 3)
		},
		want: `{"level":"info","msg":"m","a":1,"g":{"b":2,"h":{"c":3}}}`,
	},
	{
		name: "empty trailing group",
		log: func(ctx context.Context, l *slog.Logger) {
			l.With("a", 1).WithGroup("g").With("b", 2).WithGroup("h").InfoContext(ctx, "m")
		},
		want: `{"level":"info","msg":"m","a":1,"g":{"b":2}}`,
	},
	{
		name: "empty nested groups",
		log: func(ctx context.Context, l *slog.Logger) {
			l.With("a", 1).WithGroup("g").WithGroup("h").InfoContext(ctx, "m", slog.Attr{})
		},
		want: `{"level":"info","msg":"m","a":1}`,
	},
	{
		name: "groups of empty attributes",
		log: func(ctx context.Context, l *slog.Logger) {
			l.InfoContext(ctx, "m", "a", 1,
				slog.Group("x", slog.Attr{}),
				slog.Group("y", "e", emptyValuer{}),
				slog.Group("z", slog.Group("", slog.Attr{}), slog.Group("w", "e", emptyValuer{})),
				slog.Group("v", slog.Attr{}, "b", 2),
			)
		},
		want: `{"level":"info","msg":"m","a":1,"v":{"b":2}}`,
	},
	{
		name: "with group and a record of empty groups",
		log: func(ctx context.Context, l *slog.Logger) {
			l.With("a", 1).WithGroup("g").InfoContext(ctx, "m", slog.Group("x", slog.Attr{}))
		},
		want: `{"level":"info","msg":"m","a":1}`,
	},
	{
		name: "with group and with empty groups",
		log: func(ctx context.Context, l *slog.Logger) {
			l.With("a", 1).WithGroup("g").With(slog.Group("x", slog.Attr{}), slog.Group("y", "e", emptyValuer{})).InfoContext(ctx, "m")
		},
		want: `{"level":"info","msg":"m","a":1}`,
	},
	{
		name: "with empty groups before with group",
		log: func(ctx context.Context, l *slog.Logger) {
			l.With(slog.Group("x", slog.Attr{})).WithGroup("g").With("b", 2).InfoContext(ctx, "m", slog.Group("y", "e", emptyValuer{}))
		},
		want: `{"level":"info","msg":"m","g":{"b":2}}`,
	},
	{
		name: "context attributes",
		log: func(ctx context.Context, l *slog.Logger) {
			ctx = ContextWith(ctx, "request_id", "r1")
			l.WithGroup("g").InfoContext(ctx, "m", "a", 1)
		},
		want: `{"level":"info","msg":"m","g":{"a":1,"request_id":"r1"}}`,
	},
	{
		name: "custom levels",
		log: func(ctx context.Context, l *slog.Logger) {
			l.Log(ctx, slog.LevelInfo+2, "m")
		},
		want: `{"level":"info","msg":"m"}`,
	},
	{
		name: "below debug",
		log: func(ctx context.Context, l *slog.Logger) {
			l.Log(ctx, slog.LevelDebug-4, "m")
		},
		want: `{"level":"debug","msg":"m"}`,
	},
	{
		name: "above error",
		log: func(ctx context.Context, l *slog.Logger) {
			l.Log(ctx, slog.LevelError+4, "m")
		},
		want: `{"level":"error","msg":"m"}`,
	},
}

func TestConformance(t *testing.T) {
	for _, tc := range conformanceCases {
		t.Run(tc.name, func(t *testing.T) {
			want := normalizeJSON(decode(t, tc.want))
			var callers []any
			for _, b := range backends {
				l, buf := newTestLogger(t, b, WithMiddleware(setTime(fixedTime)))
				l.SetLevel(slog.LevelDebug - 4)
				tc.log(t.Context(), l.Logger)

				got := buf.record(t)
				if got[TimeKey] != fixedTimeJSON {
					t.Errorf("%s: time = %v, want %s", b, got[TimeKey], fixedTimeJSON)
				}
				caller, _ := got[CallerKey].(string)
				if !strings.Contains(caller, "conformance_test.go:") {
					t.Errorf("%s: caller = %q, want a line in conformance_test.go", b, caller)
				}
				callers = append(callers, caller)
				delete(got, TimeKey)
				delete(got, CallerKey)

				if got := normalizeJSON(got); !reflect.DeepEqual(got, want) {
					t.Errorf("%s:\n got %s\nwant %s", b, marshalSorted(got), marshalSorted(want))
				}
			}
			for i := range callers {
				if callers[i] != callers[0] {
					t.Errorf("callers differ: %v", callers)
					break
				}
			}
		})
	}
}

// TestJSONNoHTMLEscaping checks the raw output, which decoding would hide.
func TestJSONNoHTMLEscaping(t *testing.T) {
	for _, b := range backends {
		t.Run(string(b), func(t *testing.T) {
			l, buf := newTestLogger(t, b)
			l.Info("<a&b>", "s", "<a&b>", "any", map[string]string{"k": "<a&b>"}, "err", errors.New("<a&b>"))
			if got := strings.Count(buf.String(), "<a&b>"); got != 4 {
				t.Errorf("found %d unescaped values, want 4:\n%s", got, buf)
			}
		})
	}
}

func TestZeroTime(t *testing.T) {
	for _, b := range backends {
		t.Run(string(b), func(t *testing.T) {
			l, buf := newTestLogger(t, b, WithMiddleware(setTime(time.Time{})))
			l.Info("m")
			if v, ok := buf.record(t)[TimeKey]; ok {
				t.Errorf("time = %v, want no time", v)
			}
		})
	}
}

// setTime returns a middleware that sets the time of every record to tm.
func setTime(tm time.Time) Middleware {
	return func(next slog.Handler) slog.Handler {
		return mapHandler{next: next, f: func(r slog.Record) slog.Record {
			r.Time = tm
			return r
		}}
	}
}

// mapHandler changes records before passing them to next.
type mapHandler struct {
	next slog.Handler
	f    func(slog.Record) slog.Record
}

func (h mapHandler) Enabled(ctx context.Context, l slog.Level) bool { return h.next.Enabled(ctx, l) }

func (h mapHandler) Handle(ctx context.Context, r slog.Record) error {
	return h.next.Handle(ctx, h.f(r))
}

func (h mapHandler) WithAttrs(as []slog.Attr) slog.Handler {
	return mapHandler{next: h.next.WithAttrs(as), f: h.f}
}

func (h mapHandler) WithGroup(name string) slog.Handler {
	return mapHandler{next: h.next.WithGroup(name), f: h.f}
}

// normalizeJSON converts the json.Numbers in v to int64, uint64 or float64, so
// that equal numbers compare equal however they were written.
func normalizeJSON(v any) any {
	switch v := v.(type) {
	case map[string]any:
		out := make(map[string]any, len(v))
		for k, e := range v {
			out[k] = normalizeJSON(e)
		}
		return out
	case []any:
		out := make([]any, len(v))
		for i, e := range v {
			out[i] = normalizeJSON(e)
		}
		return out
	case json.Number:
		if i, err := strconv.ParseInt(string(v), 10, 64); err == nil {
			return i
		}
		if u, err := strconv.ParseUint(string(v), 10, 64); err == nil {
			return u
		}
		f, _ := v.Float64()
		return f
	default:
		return v
	}
}

func marshalSorted(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		return fmt.Sprint(v)
	}
	return string(b)
}
