package logger

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log/slog"
	"reflect"
	"runtime"

	"go.uber.org/zap/zapcore"
)

// Keys of the built-in fields of a JSON record, the same for every backend.
const (
	TimeKey    = "time"
	LevelKey   = "level"
	MessageKey = "msg"
	CallerKey  = "caller"
)

// timeLayout formats the record time and time attributes: RFC 3339 with
// millisecond precision.
const timeLayout = "2006-01-02T15:04:05.000Z07:00"

// The helpers below make the zap and zerolog handlers write values exactly as
// slog.JSONHandler does, after replaceAttr has been applied.

// callerOf formats the location of pc the way zap's ShortCallerEncoder does:
// "dir/file.go:line".
func callerOf(pc uintptr) string {
	f, _ := runtime.CallersFrames([]uintptr{pc}).Next()
	return trimmedPath(f.File, f.Line)
}

func trimmedPath(file string, line int) string {
	return zapcore.EntryCaller{Defined: true, File: file, Line: line}.TrimmedPath()
}

// encodeAny returns what slog.JSONHandler writes for a value of kind
// slog.KindAny: the Error() string of an error that is not a json.Marshaler,
// and otherwise the JSON encoding of v, returned in raw. If a method of v
// panics, it returns "<nil>" for a nil pointer and the panic message
// otherwise, as slog does.
func encodeAny(v any) (s string, raw []byte) {
	defer func() {
		if r := recover(); r != nil {
			raw = nil
			if rv := reflect.ValueOf(v); rv.Kind() == reflect.Pointer && rv.IsNil() {
				s = "<nil>"
				return
			}
			s = fmt.Sprintf("!PANIC: %v", r)
		}
	}()
	if err, ok := v.(error); ok {
		if _, ok := v.(json.Marshaler); !ok {
			return err.Error(), nil
		}
	}
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return "!ERROR:" + err.Error(), nil
	}
	return "", bytes.TrimSuffix(buf.Bytes(), []byte{'\n'})
}

// Every handler passes attributes through appendResolved before encoding them,
// so the encoders only see resolved values and never empty attributes.

// appendResolved appends a to dst with its value resolved, recursively in
// groups, unless a is empty: the zero Attr, or a group with no attributes
// left. slog handlers ignore empty attributes.
func appendResolved(dst []slog.Attr, a slog.Attr) []slog.Attr {
	a.Value = a.Value.Resolve()
	switch {
	case a.Value.Kind() == slog.KindGroup:
		var members []slog.Attr
		for _, m := range a.Value.Group() {
			members = appendResolved(members, m)
		}
		if len(members) == 0 {
			return dst
		}
		a.Value = slog.GroupValue(members...)
	case a.Key == "" && a.Value.Kind() == slog.KindAny && a.Value.Any() == nil:
		return dst
	}
	return append(dst, a)
}

func resolveAttrs(attrs []slog.Attr) []slog.Attr {
	out := make([]slog.Attr, 0, len(attrs))
	for _, a := range attrs {
		out = appendResolved(out, a)
	}
	return out
}

func recordAttrs(r slog.Record) []slog.Attr {
	attrs := make([]slog.Attr, 0, r.NumAttrs())
	r.Attrs(func(a slog.Attr) bool {
		attrs = appendResolved(attrs, a)
		return true
	})
	return attrs
}

// group is a group opened by Handler.WithGroup, with the attributes added to
// it by later WithAttrs calls. The zap and zerolog handlers encode attributes
// eagerly until the first WithGroup, and keep them in groups after that.
type group struct {
	name  string
	attrs []slog.Attr
}

func withGroup(groups []group, name string) []group {
	return append(groups[:len(groups):len(groups)], group{name: name})
}

func withGroupAttrs(groups []group, attrs []slog.Attr) []group {
	out := make([]group, len(groups))
	copy(out, groups)
	last := &out[len(out)-1]
	last.attrs = append(last.attrs[:len(last.attrs):len(last.attrs)], attrs...)
	return out
}

// nonEmptyGroups returns the open groups that a record with the given
// attributes writes: trailing groups that would be empty are dropped.
func nonEmptyGroups(groups []group, attrs []slog.Attr) []group {
	if len(attrs) > 0 {
		return groups
	}
	for i := len(groups) - 1; i >= 0; i-- {
		if len(groups[i].attrs) > 0 {
			return groups[:i+1]
		}
	}
	return nil
}
