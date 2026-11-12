// Package logger builds [log/slog] loggers backed by slog, zap or zerolog, with
// one output format whichever backend is chosen.
//
// The logging API is the standard library's [*slog.Logger]. This package only
// configures it: it picks the backend that encodes and writes records, the
// level, the format and the outputs. Code that logs depends on log/slog, not
// on this package or on a backend, so switching backends is a config change.
//
// # Quick start
//
//	l, err := logger.New(logger.Config{Backend: logger.BackendZap, Level: "debug"})
//	if err != nil {
//		return err
//	}
//	defer l.Close()
//	slog.SetDefault(l.Logger)
//
//	slog.Info("server started", "port", 8080)
//
// # Output format
//
// With [FormatJSON], every backend writes the same JSON for the same record:
//
//	{"time":"2026-11-12T14:35:03.189+07:00","level":"info","caller":"server/main.go:42","msg":"server started","port":8080}
//
// The fields are:
//
//   - "time" ([TimeKey]): RFC 3339 with millisecond precision, in the
//     time's location. It is omitted when the record time is zero.
//   - "level" ([LevelKey]): "debug", "info", "warn" or "error". Other levels
//     are rounded down to one of these, so slog.LevelInfo+2 is "info".
//   - "caller" ([CallerKey]): the last directory and file name, and the line
//     number. It is omitted when [Config].DisableCaller is set or the record
//     has no program counter.
//   - "msg" ([MessageKey]): the message.
//   - The attributes. Groups are nested objects. Durations are strings like
//     "1.5s", times use the layout of "time", errors are their Error()
//     string unless they implement json.Marshaler, NaN and infinities are
//     the strings "NaN", "+Inf" and "-Inf", and other values are encoded
//     with encoding/json, without HTML escaping.
//
// The order of keys is not part of this contract and differs between backends.
// [FormatText] is meant for people reading a terminal; each backend renders
// it in its own style.
//
// # Request-scoped attributes
//
// [ContextWith] stores attributes in a context. Records logged with that
// context, through the *Context methods of slog.Logger, include them:
//
//	ctx = logger.ContextWith(ctx, "request_id", id)
//	slog.InfoContext(ctx, "route found", "hops", 3)
package logger
