# logger

[![Go Reference](https://pkg.go.dev/badge/github.com/KyberNetwork/logger/v2.svg)](https://pkg.go.dev/github.com/KyberNetwork/logger/v2)

`logger` builds a standard [`*slog.Logger`](https://pkg.go.dev/log/slog) backed by
[slog](https://pkg.go.dev/log/slog), [zap](https://github.com/uber-go/zap) or
[zerolog](https://github.com/rs/zerolog), from one small config.

- **One API.** Code logs through `log/slog`. It never imports this package or a
  backend, so switching backends is a config change.
- **One output format.** Every backend writes the same JSON for the same call,
  so dashboards and alerts survive a backend switch. A conformance test suite
  enforces this.
- **Correct callers.** The `caller` field is right however you log: through
  `slog.Info`, `logger.With(...)`, the `log` package, or middleware.
- **No globals.** `New` returns a logger and changes nothing else. You choose
  whether to install it with `slog.SetDefault`.

```sh
go get github.com/KyberNetwork/logger/v2
```

Requires Go 1.27.

## Quick start

```go
package main

import (
	"log"
	"log/slog"

	"github.com/KyberNetwork/logger/v2"
)

func main() {
	l, err := logger.New(logger.Config{Backend: logger.BackendZap, Level: "debug"})
	if err != nil {
		log.Fatal(err)
	}
	defer l.Close()
	slog.SetDefault(l.Logger) // also routes the log package through l

	slog.Info("server started", "port", 8080)
}
```

```json
{"time":"2026-11-12T14:35:03.189+07:00","level":"info","caller":"server/main.go:18","msg":"server started","port":8080}
```

## Logging

Logging is plain `log/slog`. Pass a `*slog.Logger` to the code that logs
(`l.Logger`, or `slog.Default()`), and use key-value pairs or `slog.Attr`s:

```go
func NewRouter(log *slog.Logger) *Router {
	return &Router{log: log.With("component", "router")}
}

r.log.Info("route found", "hops", 3, "took", time.Since(start))
r.log.Error("quote failed", "err", err, slog.Group("pool", "id", poolID, "dex", dex))
```

The `*Context` methods add request-scoped attributes stored with
`logger.ContextWith`, so they don't have to be passed down:

```go
ctx = logger.ContextWith(ctx, "request_id", reqID, "user", userID)

slog.InfoContext(ctx, "quote served") // includes request_id and user
```

`logger.Logger` embeds `*slog.Logger` and adds what only the owner of the logger
needs: `SetLevel`, `Level` and `Close`.

## Configuration

```go
type Config struct {
	Backend       Backend    // "slog" (default), "zap" or "zerolog"
	Level         string     // "debug", "info" (default), "warn" or "error"
	Format        Format     // "json" (default) or "text"
	DisableCaller bool       // omit the caller field
	Console       Console    // "stdout" (default), "stderr" or "none"
	File          FileConfig // a rotated log file, enabled when File.Path is set
}

type FileConfig struct {
	Path       string
	MaxSizeMB  int  // rotate at this size; default 100
	MaxAgeDays int  // delete rotated files older than this; default never
	MaxBackups int  // keep at most this many rotated files; default all
	Compress   bool // gzip rotated files
}
```

The zero `Config` logs JSON at info level to stdout with the slog backend.
String values are case-insensitive. `New` reports every invalid field in one
error, and opens the log file right away, so a bad path fails at startup
rather than losing logs.

`Config` has `json`, `yaml` and `mapstructure` tags, so it can be part of a
service's config file (and loaded with viper):

```yaml
log:
  backend: zerolog
  level: info
  format: json
  console: stdout
  file:
    path: /var/log/router/router.log
    max_size_mb: 100
    max_age_days: 7
    max_backups: 10
    compress: true
```

The console and the file share the level and the format.

### Changing the level at runtime

```go
l.SetLevel(slog.LevelDebug) // safe while logging; applies to every derived logger

level, err := logger.ParseLevel("warn") // for admin endpoints and config reloads
```

`ParseLevel` accepts `debug`, `info`, `warn` (or `warning`) and `error`, in any
case, and slog's offset syntax such as `info+2`.

## Output format

With `Format: "json"`, all three backends write the same JSON. The examples
below come from `l.With("service", "router").Info("request handled", "status", 200, "took", 1500*time.Microsecond, "err", errors.New("boom"))`:

```
slog     {"time":"2026-11-12T15:14:06.273+07:00","level":"info","caller":"api/handler.go:18","msg":"request handled","service":"router","status":200,"took":"1.5ms","err":"boom"}
zap      {"level":"info","time":"2026-11-12T15:14:06.274+07:00","caller":"api/handler.go:18","msg":"request handled","service":"router","status":200,"took":"1.5ms","err":"boom"}
zerolog  {"service":"router","time":"2026-11-12T15:14:06.274+07:00","level":"info","caller":"api/handler.go:18","msg":"request handled","status":200,"took":"1.5ms","err":"boom"}
```

| Field    | Value |
|----------|-------|
| `time`   | RFC 3339 with milliseconds, in the time's location. Omitted if the record time is zero. |
| `level`  | `debug`, `info`, `warn` or `error`. Other levels round down: `slog.LevelInfo+2` is `info`. |
| `caller` | `dir/file.go:line`. Omitted with `DisableCaller`, or when the record has no caller (see below). |
| `msg`    | The message. |
| others   | The attributes. Groups are nested objects. |

Attribute values are encoded as follows:

- Durations are strings such as `"1.5s"`.
- Times use the same layout as `time`.
- Errors are their `Error()` string, unless they implement `json.Marshaler`.
- NaN and infinities are `"NaN"`, `"+Inf"` and `"-Inf"`.
- A nil pointer error whose `Error` method panics is written as `"<nil>"`.
- Any other value is encoded as `encoding/json` encodes it, without HTML escaping.

Two things are **not** part of the format: the order of keys, which differs
between backends, and the `text` format. With `Format: "text"`, each backend
writes its own human-readable style. zap and zerolog color it when the console
is a terminal and `NO_COLOR` is unset:

```
slog     time=2026-11-12T15:14:06.274+07:00 level=info caller=api/handler.go:18 msg="request handled" service=router status=200 took=1.5ms err=boom
zap      2026-11-12T15:14:06.274+07:00	INFO	api/handler.go:18	request handled	{"service": "router", "status": 200, "took": "1.5ms", "err": "boom"}
zerolog  2026-11-12T15:14:06.274+07:00 INF api/handler.go:18 > request handled err=boom service=router status=200 took=1.5ms
```

Avoid the keys `time`, `level`, `msg` and `caller` for attributes. Duplicate keys
are written as is, and backends order them differently.

## Customization

### Console writer

`WithWriter` sends console output to any `io.Writer`, such as a buffer in tests:

```go
var buf bytes.Buffer
l, err := logger.New(logger.Config{}, logger.WithWriter(&buf))
```

### Middleware

A middleware wraps the logger's `slog.Handler`. It works with every backend,
and sees records with the `ContextWith` attributes already added. This one adds
the OpenTelemetry trace ID of the context:

```go
type traceHandler struct{ slog.Handler }

func (h traceHandler) Handle(ctx context.Context, r slog.Record) error {
	if sc := trace.SpanContextFromContext(ctx); sc.IsValid() {
		r.AddAttrs(slog.String("trace_id", sc.TraceID().String()))
	}
	return h.Handler.Handle(ctx, r)
}

func (h traceHandler) WithAttrs(as []slog.Attr) slog.Handler { return traceHandler{h.Handler.WithAttrs(as)} }
func (h traceHandler) WithGroup(name string) slog.Handler    { return traceHandler{h.Handler.WithGroup(name)} }

l, err := logger.New(cfg, logger.WithMiddleware(func(next slog.Handler) slog.Handler {
	return traceHandler{next}
}))
```

Middleware can also filter, sample or redact records. With several middleware,
the first one listed sees records first.

### More outputs

`l.Handler()` is an ordinary `slog.Handler`, so it composes with any other:

```go
errorsToSentry := ... // any slog.Handler
both := slog.New(slog.NewMultiHandler(l.Handler(), errorsToSentry))
```

### Helpers that log for their caller

slog records the location of the function that calls a `*slog.Logger` method. A
helper that logs on behalf of its caller must capture the caller itself, or
every record will point at the helper:

```go
// Infof logs a formatted message at info level, with the caller's location.
func Infof(ctx context.Context, format string, args ...any) {
	l := slog.Default()
	if !l.Enabled(ctx, slog.LevelInfo) {
		return
	}
	var pcs [1]uintptr
	runtime.Callers(2, pcs[:]) // skip runtime.Callers and Infof
	r := slog.NewRecord(time.Now(), slog.LevelInfo, fmt.Sprintf(format, args...), pcs[0])
	_ = l.Handler().Handle(ctx, r)
}
```

## Notes

- **Context attributes and groups.** `ContextWith` attributes are added to the
  record, so `log.WithGroup("g").InfoContext(ctx, ...)` writes them inside `g`.
- **The `log` package.** After `slog.SetDefault`, `log.Print` goes through the
  logger at info level. It records a caller only if `log.Lshortfile` or
  `log.Llongfile` was set before `slog.SetDefault` was called.
- **zerolog.** The handler logs with `zerolog.NoLevel`, so
  `zerolog.SetGlobalLevel(zerolog.Disabled)` anywhere in the program silences it.
  Other global levels have no effect. Write errors are reported to
  `zerolog.ErrorHandler`, not returned from `Handle`.
- **Closing.** `Close` closes the log file. Records logged after `Close`
  reopen it.
- **slog backend.** It removes empty groups before passing records to slog's
  built-in handlers. Go 1.27's `slog.JSONHandler` writes invalid JSON (a missing
  comma) after a group whose attributes are all empty.

## Migrating from v1

v2 is a new module, `github.com/KyberNetwork/logger/v2`, so v1 and v2 can be
used side by side while a service migrates.

| v1 | v2 |
|----|----|
| `logger.InitLogger(cfg, logger.LoggerBackendZap)` | `l, err := logger.New(cfg)` then `slog.SetDefault(l.Logger)` |
| `logger.NewLogger(cfg, backend)` | `logger.New(cfg)` |
| `Configuration{EnableConsole, EnableJSONFormat, ConsoleLevel}` | `Config{Console, Format, Level}`; the console is on by default |
| `Configuration{EnableFile, FileJSONFormat, FileLevel, FileLocation}` | `Config{File: FileConfig{Path: ...}}`; the file uses `Level` and `Format` |
| `LoggerBackendZap`, `LoggerBackendLogrus` | `Backend: "zap"`, `"zerolog"` or `"slog"`; logrus is gone |
| `logger.Logger` (parameter type) | `*slog.Logger` |
| `logger.Get()` | `slog.Default()`, or pass the `*slog.Logger` explicitly |
| `logger.Info(msg)`, `Infoln(msg)` | `slog.Info(msg)` |
| `logger.Infof("took %v for %s", d, id)` | `slog.Info("done", "took", d, "id", id)`, or a helper as [above](#helpers-that-log-for-their-caller) |
| `logger.WithFields(logger.Fields{"k": v}).Info(msg)` | `slog.Info(msg, "k", v)` or `slog.With("k", v).Info(msg)` |
| `logger.Fatal(msg)` | `slog.Error(msg)` then `os.Exit(1)` |
| `logger.SetLogLevel("debug")` | `l.SetLevel(slog.LevelDebug)`, with `logger.ParseLevel` for strings |
| `logger.GetDelegate().(*zap.SugaredLogger)` | Nothing; see below |

**Levels.** An unknown level such as `"trace"` or `"fatal"` is now an error from
`New`; v1's zap backend silently used info.

**GetDelegate.** v2 has no underlying logger to hand out. The usual reason to
get one was to adapt it for a library, and most now accept slog. For
go-grpc-middleware v2 interceptors, whose `logging.Level` is a `slog.Level`:

```go
logging.UnaryServerInterceptor(logging.LoggerFunc(
	func(ctx context.Context, lvl logging.Level, msg string, fields ...any) {
		l.Log(ctx, slog.Level(lvl), msg, fields...)
	}), opts...)
```

**Output changes**, for anything that parses v1's zap JSON:

| | v1 (zap) | v2 |
|-|----------|----|
| Time key | `ts` | `time` |
| Time value | `2026-11-12T14:35:03.189+0700` | `2026-11-12T14:35:03.189+07:00` |
| Durations | seconds as a number, `1.5` | a string, `"1.5s"` |
| Errors with `%+v` (pkg/errors) | `"err"` plus an `"errVerbose"` stack | `"err"` only |
| Text format colors | always, even in files | only on a terminal |
| Levels | also `fatal` | `debug`, `info`, `warn`, `error` |

## Development

```sh
make fmt    # format with gofumpt, then golines
make lint   # lint and check formatting, as CI does
make test   # run the tests with the race detector, as CI does
make bench  # run the benchmarks
```

Code is formatted with [gofumpt](https://github.com/mvdan/gofumpt), a stricter
gofmt, and [golines](https://github.com/golangci/golines), which wraps lines
longer than 100 columns. Both, and
[golangci-lint](https://golangci-lint.run), are Go tools pinned in `tools.mod`,
so there's nothing to install: `make` builds them on first use, or run one
directly with `go tool -modfile=tools.mod <name>`. They live in `tools.mod`
rather than `go.mod` so that modules importing this one don't inherit their
dependencies.

golines splits a long call one argument per line, which separates slog
key/value pairs. Wrap long logging calls by hand to 100 columns, keeping each
pair together; golines leaves lines that fit alone.

golangci-lint is pinned to v2.15.0, the version CI runs, and gofumpt and golines
to the versions it embeds, so local formatting matches CI's check. When
upgrading golangci-lint, change `tools.mod` and `.github/workflows/ci.yaml`
together.
