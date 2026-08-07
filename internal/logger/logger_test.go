package logger

import (
	"bytes"
	"context"
	"log/slog"
	"strings"
	"testing"
)

// mockHandler is a simple mock for slog.Handler
type mockHandler struct {
	enabled bool
	handled bool
	attrs   []slog.Attr
	group   string
}

func (m *mockHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return m.enabled
}

func (m *mockHandler) Handle(ctx context.Context, r slog.Record) error {
	m.handled = true
	return nil
}

func (m *mockHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	newMock := *m
	newMock.attrs = append(newMock.attrs, attrs...)
	return &newMock
}

func (m *mockHandler) WithGroup(name string) slog.Handler {
	newMock := *m
	newMock.group = name
	return &newMock
}

func TestMultiHandler_Enabled(t *testing.T) {
	tests := []struct {
		name     string
		handlers []slog.Handler
		want     bool
	}{
		{
			name:     "both disabled",
			handlers: []slog.Handler{&mockHandler{enabled: false}, &mockHandler{enabled: false}},
			want:     false,
		},
		{
			name:     "one enabled",
			handlers: []slog.Handler{&mockHandler{enabled: false}, &mockHandler{enabled: true}},
			want:     true,
		},
		{
			name:     "both enabled",
			handlers: []slog.Handler{&mockHandler{enabled: true}, &mockHandler{enabled: true}},
			want:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := &MultiHandler{handlers: tt.handlers}
			if got := m.Enabled(context.Background(), slog.LevelInfo); got != tt.want {
				t.Errorf("MultiHandler.Enabled() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestMultiHandler_Handle(t *testing.T) {
	h1 := &mockHandler{enabled: false}
	h2 := &mockHandler{enabled: true}

	m := &MultiHandler{handlers: []slog.Handler{h1, h2}}
	r := slog.Record{Level: slog.LevelInfo}
	err := m.Handle(context.Background(), r)

	if err != nil {
		t.Errorf("MultiHandler.Handle() returned unexpected error: %v", err)
	}
	if h1.handled {
		t.Errorf("Expected h1 to not handle the record")
	}
	if !h2.handled {
		t.Errorf("Expected h2 to handle the record")
	}
}

func TestMultiHandler_WithAttrs(t *testing.T) {
	h1 := &mockHandler{}
	m := &MultiHandler{handlers: []slog.Handler{h1}}

	attrs := []slog.Attr{slog.String("key", "value")}
	newHandler := m.WithAttrs(attrs)

	newMulti, ok := newHandler.(*MultiHandler)
	if !ok {
		t.Fatalf("Expected *MultiHandler, got %T", newHandler)
	}

	mock, ok := newMulti.handlers[0].(*mockHandler)
	if !ok {
		t.Fatalf("Expected *mockHandler, got %T", newMulti.handlers[0])
	}
	if len(mock.attrs) != 1 || mock.attrs[0].Key != "key" {
		t.Errorf("Attributes were not correctly passed down")
	}
}

func TestMultiHandler_WithGroup(t *testing.T) {
	h1 := &mockHandler{}
	m := &MultiHandler{handlers: []slog.Handler{h1}}

	newHandler := m.WithGroup("testGroup")

	newMulti, ok := newHandler.(*MultiHandler)
	if !ok {
		t.Fatalf("Expected *MultiHandler, got %T", newHandler)
	}

	mock, ok := newMulti.handlers[0].(*mockHandler)
	if !ok {
		t.Fatalf("Expected *mockHandler, got %T", newMulti.handlers[0])
	}
	if mock.group != "testGroup" {
		t.Errorf("Expected group 'testGroup', got %v", mock.group)
	}
}

func TestNewLogger(t *testing.T) {
	var buf bytes.Buffer
	logger := New(&buf)

	if logger == nil {
		t.Fatal("Expected logger to be created, got nil")
	}

	logger.Info("test message", "key", "value")

	// Verify JSON output
	output := buf.String()
	if !strings.Contains(output, "test message") {
		t.Errorf("Expected JSON output to contain message, got: %s", output)
	}
	if !strings.Contains(output, "\"key\":\"value\"") {
		t.Errorf("Expected JSON output to contain attribute, got: %s", output)
	}
}
