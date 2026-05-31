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

// NewDebouncer создает новый экземпляр дебаунсера.
func NewDebouncer(duration time.Duration, callback func(path string)) *Debouncer {
	return &Debouncer{
		timers:   make(map[string]*time.Timer),
		duration: duration,
		callback: callback,
	}
}

// Trigger регистрирует событие для пути. Если таймер уже есть, он сбрасывается.
func (d *Debouncer) Trigger(path string) {
	d.mu.Lock()
	defer d.mu.Unlock()

	// Если таймер уже запущен для этого файла, просто обновляем его (сбрасываем отсчет)
	if timer, exists := d.timers[path]; exists {
		timer.Reset(d.duration)
		return
	}

	// Если таймера нет, создаем новый
	d.timers[path] = time.AfterFunc(d.duration, func() {
		// Когда таймер срабатывает, удаляем его из мапы...
		d.mu.Lock()
		delete(d.timers, path)
		d.mu.Unlock()
		
		// ...и вызываем полезную нагрузку
		d.callback(path)
	})
}