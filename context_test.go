package logger

import (
	"context"
	"log/slog"
	"testing"
)

func TestContextWith(t *testing.T) {
	l, buf := newTestLogger(t, BackendSlog)
	ctx1 := ContextWith(t.Context(), "a", 1)
	ctx2 := ContextWith(ctx1, slog.String("b", "x"), "c", true)

	l.InfoContext(ctx2, "m", "d", 2)
	rec := normalizeJSON(buf.record(t)).(map[string]any)
	for k, want := range map[string]any{"a": int64(1), "b": "x", "c": true, "d": int64(2)} {
		if rec[k] != want {
			t.Errorf("%s = %v, want %v", k, rec[k], want)
		}
	}

	// Deriving ctx2 did not change ctx1.
	buf.Reset()
	l.InfoContext(ctx1, "m")
	if rec := buf.record(t); rec["b"] != nil || rec["a"] == nil {
		t.Errorf("record = %v, want only a", rec)
	}

	// Logging without the context adds nothing.
	buf.Reset()
	l.Info("m")
	if rec := buf.record(t); rec["a"] != nil {
		t.Errorf("record = %v, want no a", rec)
	}
}

func TestContextWithNoArgs(t *testing.T) {
	ctx := t.Context()
	if got := ContextWith(ctx); got != ctx {
		t.Error("ContextWith with no arguments returned a new context")
	}
}

func TestContextAttrsNilContext(t *testing.T) {
	if attrs := contextAttrs(nil); attrs != nil { //nolint:staticcheck // a nil context must be tolerated
		t.Errorf("contextAttrs(nil) = %v", attrs)
	}
}

// TestContextAttrsNotShared checks that adding the context attributes to a
// record does not change the record seen by other handlers.
func TestContextAttrsNotShared(t *testing.T) {
	var seen []int
	h := contextHandler{next: mapHandler{next: slog.DiscardHandler, f: func(r slog.Record) slog.Record {
		seen = append(seen, r.NumAttrs())
		return r
	}}}
	r := slog.NewRecord(fixedTime, slog.LevelInfo, "m", 0)
	r.AddAttrs(slog.Int("a", 1))
	ctx := ContextWith(context.Background(), "b", 2)
	for range 2 {
		if err := h.Handle(ctx, r); err != nil {
			t.Fatal(err)
		}
	}
	if r.NumAttrs() != 1 || seen[0] != 2 || seen[1] != 2 {
		t.Errorf("record has %d attributes, handlers saw %v; want 1 and [2 2]", r.NumAttrs(), seen)
	}
}
