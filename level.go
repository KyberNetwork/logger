package logger

import (
	"fmt"
	"log/slog"
	"strings"
)

// ParseLevel parses a level name: "debug", "info", "warn" (or "warning") or
// "error", case-insensitively. It also accepts slog's offset syntax, such as
// "info+2".
func ParseLevel(s string) (slog.Level, error) {
	name := strings.ToLower(strings.TrimSpace(s))
	if name == "warning" {
		name = "warn"
	}
	var level slog.Level
	if err := level.UnmarshalText([]byte(name)); err != nil {
		return 0, fmt.Errorf("logger: invalid level %q (want debug, info, warn or error)", s)
	}
	return level, nil
}

// levelName is the "level" value written for l: the name of the nearest
// standard level at or below l.
func levelName(l slog.Level) string {
	switch {
	case l < slog.LevelInfo:
		return "debug"
	case l < slog.LevelWarn:
		return "info"
	case l < slog.LevelError:
		return "warn"
	default:
		return "error"
	}
}
