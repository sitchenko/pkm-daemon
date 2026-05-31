package logger

import (
	"context"
	"io"
	"log/slog"
	"os"
	"time"

	"github.com/lmittmann/tint"
)

// MultiHandler позволяет отправлять логи сразу в несколько обработчиков.
type MultiHandler struct {
	handlers []slog.Handler
}

func (m *MultiHandler) Enabled(ctx context.Context, level slog.Level) bool {
	for _, h := range m.handlers {
		if h.Enabled(ctx, level) {
			return true
		}
	}
	return false
}

func (m *MultiHandler) Handle(ctx context.Context, r slog.Record) error {
	for _, h := range m.handlers {
		if h.Enabled(ctx, r.Level) {
			if err := h.Handle(ctx, r); err != nil {
				return err
			}
		}
	}
	return nil
}

func (m *MultiHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	handlers := make([]slog.Handler, len(m.handlers))
	for i, h := range m.handlers {
		handlers[i] = h.WithAttrs(attrs)
	}
	return &MultiHandler{handlers: handlers}
}

func (m *MultiHandler) WithGroup(name string) slog.Handler {
	handlers := make([]slog.Handler, len(m.handlers))
	for i, h := range m.handlers {
		handlers[i] = h.WithGroup(name)
	}
	return &MultiHandler{handlers: handlers}
}

// New создает логгер с двумя уровнями вывода (Console + JSON File)
func New(jsonFile io.Writer) *slog.Logger {
	// Уровень 2: Структурированный JSON-лог в файл (без изменений, чистый JSON)
	jsonHandler := slog.NewJSONHandler(jsonFile, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	})

	// Уровень 1: Читаемый текстовый вывод в STDOUT с красивой цветовой подсветкой (tint)
	textHandler := tint.NewHandler(os.Stdout, &tint.Options{
		Level:      slog.LevelInfo,
		TimeFormat: time.TimeOnly, // Вывод компактного времени (напр. 15:04:05)
	})

	multi := &MultiHandler{
		handlers: []slog.Handler{textHandler, jsonHandler},
	}

	return slog.New(multi)
}