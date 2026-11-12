package logger

import (
	"context"
	"log/slog"
)

type contextKey struct{}

// ContextWith returns a copy of ctx carrying attributes that are added to every
// record logged with it through a logger from [New], for example with
// slog.InfoContext. args are key-value pairs and slog.Attr values, as in
// slog.Logger.With. Attributes from earlier ContextWith calls on ctx are kept.
//
// The attributes are added to the record, so they are nested in any group
// opened with slog.Logger.WithGroup.
func ContextWith(ctx context.Context, args ...any) context.Context {
	attrs := slog.Group("", args...).Value.Group()
	if len(attrs) == 0 {
		return ctx
	}
	prev := contextAttrs(ctx)
	merged := make([]slog.Attr, 0, len(prev)+len(attrs))
	merged = append(merged, prev...)
	merged = append(merged, attrs...)
	return context.WithValue(ctx, contextKey{}, merged)
}

func contextAttrs(ctx context.Context) []slog.Attr {
	if ctx == nil {
		return nil
	}
	attrs, _ := ctx.Value(contextKey{}).([]slog.Attr)
	return attrs
}

// contextHandler adds the attributes stored by ContextWith to each record.
type contextHandler struct {
	next slog.Handler
}

func (h contextHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.next.Enabled(ctx, level)
}

func (h contextHandler) Handle(ctx context.Context, r slog.Record) error {
	if attrs := contextAttrs(ctx); len(attrs) > 0 {
		r = r.Clone()
		r.AddAttrs(attrs...)
	}
	return h.next.Handle(ctx, r)
}

func (h contextHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return contextHandler{next: h.next.WithAttrs(attrs)}
}

func (h contextHandler) WithGroup(name string) slog.Handler {
	return contextHandler{next: h.next.WithGroup(name)}
}
