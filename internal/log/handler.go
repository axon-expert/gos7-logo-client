package log

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"
	"unicode"
)

type SimpleHandler struct {
	Level  slog.Level
	Writer io.Writer
	groups []string
}

var _ slog.Handler = (*SimpleHandler)(nil)

func (h SimpleHandler) WithAttrs([]slog.Attr) slog.Handler { return h }

func (h SimpleHandler) WithGroup(group string) slog.Handler {
	return SimpleHandler{
		Level:  h.Level,
		Writer: h.Writer,
		groups: append(h.groups, group),
	}
}

func (h SimpleHandler) Enabled(_ context.Context, level slog.Level) bool {
	return level >= h.Level
}

func (h SimpleHandler) Handle(_ context.Context, record slog.Record) error {
	sb := new(strings.Builder)
	_, _ = fmt.Fprint(sb, record.Time.Format("2006-01-02 15:04:05.000"))
	_, _ = fmt.Fprint(sb, " \033[01m")
	switch {
	case record.Level < slog.LevelInfo:
		_, _ = fmt.Fprint(sb, "\033[34mDEBU")
	case record.Level < slog.LevelWarn:
		_, _ = fmt.Fprint(sb, "\033[32mINFO")
	case record.Level < slog.LevelError:
		_, _ = fmt.Fprint(sb, "\033[93mWARN")
	default:
		_, _ = fmt.Fprint(sb, "\033[31mERRO")
	}
	if len(h.groups) > 0 {
		_, _ = fmt.Fprint(sb, " \033[90m")
		_, _ = fmt.Fprint(sb, strings.Join(h.groups, ".")+":")
	}
	_, _ = fmt.Fprint(sb, "\033[0m ")
	_, _ = fmt.Fprint(sb, strings.TrimRightFunc(record.Message, unicode.IsSpace))
	_, _ = fmt.Fprint(sb, "\033[90m")
	record.Attrs(func(attr slog.Attr) bool {
		_, _ = fmt.Fprint(sb, " ", attr)
		return true
	})
	_, _ = fmt.Fprint(sb, "\033[0m\n")
	writer := h.Writer
	if writer == nil {
		writer = os.Stderr
	}
	_, err := io.WriteString(writer, sb.String())
	return err
}
