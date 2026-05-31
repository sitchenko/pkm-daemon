package watcher

import (
	"sync"
	"time"
)

// Debouncer ограничивает частоту вызовов для одного и того же пути файла.
type Debouncer struct {
	mu       sync.Mutex
	timers   map[string]*time.Timer
	duration time.Duration
	callback func(path string)
}

// NewDebouncer создает новый экземпляр дебаунсера с указанной задержкой.
func NewDebouncer(duration time.Duration, callback func(path string)) *Debouncer {
	return &Debouncer{
		timers:   make(map[string]*time.Timer),
		duration: duration,
		callback: callback,
	}
}

// Add добавляет или сбрасывает таймер для конкретного файла.
func (d *Debouncer) Add(path string) {
	d.mu.Lock()
	defer d.mu.Unlock()

	// Если таймер уже есть, останавливаем его
	if timer, exists := d.timers[path]; exists {
		timer.Stop()
	}

	// Запускаем новый таймер
	d.timers[path] = time.AfterFunc(d.duration, func() {
		d.mu.Lock()
		delete(d.timers, path)
		d.mu.Unlock()
		d.callback(path)
	})
}

// Stop принудительно останавливает все активные таймеры.
func (d *Debouncer) Stop() {
	d.mu.Lock()
	defer d.mu.Unlock()
	for _, timer := range d.timers {
		timer.Stop()
	}
}