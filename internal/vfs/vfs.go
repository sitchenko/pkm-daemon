package vfs

import (
	"fmt"
	"io/fs"
	"math/rand"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	maxRetries    = 5
	baseDelayMs   = 50
	maxDelayMs    = 1000
)

// ScanVault рекурсивно обходит директорию Obsidian, игнорируя системные папки (начинающиеся с точки).
// Возвращает список относительных путей ко всем файлам и папкам.
func ScanVault(rootPath string) ([]string, error) {
	var paths []string

	err := filepath.WalkDir(rootPath, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			// Если нет прав на папку, просто пропускаем ее
			return nil
		}

		// Игнорируем сам корень
		if path == rootPath {
			return nil
		}

		// Вычисляем относительный путь
		relPath, err := filepath.Rel(rootPath, path)
		if err != nil {
			return nil
		}

		// Защита: пропускаем любые директории, начинающиеся с точки (.obsidian, .trash, .git)
		if d.IsDir() && strings.HasPrefix(d.Name(), ".") {
			return filepath.SkipDir // Полностью пропускаем обход этой ветки
		}

		// Если это файл, и он начинается с точки - просто игнорируем
		if !d.IsDir() && strings.HasPrefix(d.Name(), ".") {
			return nil
		}

		paths = append(paths, relPath)
		return nil
	})

	if err != nil {
		return nil, fmt.Errorf("failed to scan vault: %w", err)
	}

	return paths, nil
}

// AtomicWrite гарантирует безопасную запись файла даже при агрессивной работе Google Drive.
func AtomicWrite(targetPath string, data []byte) error {
	dir := filepath.Dir(targetPath)
	
	// Создаем целевую директорию, если ее нет
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create target directory: %w", err)
	}

	// Генерируем уникальное имя для временного файла, чтобы избежать конфликтов при параллельной записи
	base := filepath.Base(targetPath)
	tempPath := filepath.Join(dir, fmt.Sprintf("%s.%d.tmp", base, rand.Int63()))

	f, err := os.OpenFile(tempPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0644)
	if err != nil {
		return fmt.Errorf("failed to create temp file: %w", err)
	}

	// Отложенная очистка мусора на случай паники или провала переименования
	var renameDone bool
	defer func() {
		if !renameDone {
			_ = os.Remove(tempPath)
		}
	}()

	// Пишем данные
	if _, err := f.Write(data); err != nil {
		f.Close()
		return fmt.Errorf("failed to write data: %w", err)
	}

	// Форсируем сброс буферов ОС на диск
	if err := f.Sync(); err != nil {
		f.Close()
		return fmt.Errorf("failed to sync data to disk: %w", err)
	}
	f.Close() // Обязательно закрываем файл ДО переименования (иначе Windows заблокирует файл)

	// Механизм Exponential Backoff для переименования (обход блокировок Google Drive)
	return withRetry(func() error {
		err := os.Rename(tempPath, targetPath)
		if err == nil {
			renameDone = true
		}
		return err
	})
}

// SafeMove безопасно перемещает файл из oldPath в newPath с учетом блокировок диска.
func SafeMove(oldPath, newPath string) error {
	dir := filepath.Dir(newPath)
	
	// Создаем целевую директорию перед перемещением, если ее нет
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create target directory for move: %w", err)
	}

	return withRetry(func() error {
		return os.Rename(oldPath, newPath)
	})
}

// withRetry выполняет функцию с экспоненциальной задержкой в случае ошибки (Access Denied / Sharing Violation).
func withRetry(operation func() error) error {
	var lastErr error
	delay := time.Duration(baseDelayMs) * time.Millisecond

	for attempt := 0; attempt < maxRetries; attempt++ {
		err := operation()
		if err == nil {
			return nil // Успех
		}
		
		lastErr = err
		
		// Если это не последняя попытка, ждем перед следующим шагом
		if attempt < maxRetries-1 {
			time.Sleep(delay)
			
			// Экспоненциальный рост + небольшой джиттер (шорт-рандом), чтобы избежать Thundering Herd
			delay *= 2
			jitter := time.Duration(rand.Intn(50)) * time.Millisecond
			delay += jitter
			
			if delay > time.Duration(maxDelayMs)*time.Millisecond {
				delay = time.Duration(maxDelayMs) * time.Millisecond
			}
		}
	}

	return fmt.Errorf("operation failed after %d retries, last error: %w", maxRetries, lastErr)
}