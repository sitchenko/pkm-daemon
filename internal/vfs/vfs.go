package vfs

import (
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	maxRetries  = 5
	baseDelayMs = 50
	maxDelayMs  = 1000
)

// ScanVault рекурсивно сканирует хранилище, собирая структуру Markdown файлов.
func ScanVault(rootPath string) ([]string, error) {
	var files []string

	err := filepath.Walk(rootPath, func(path string, info os.FileInfo, err error) error {
		// ИГНОРИРУЕМ ОШИБКИ: Если файл (например, desktop.ini) заблокирован или исчез
		// во время сканирования, мы просто пропускаем его и идем дальше.
		if err != nil {
			return nil
		}

		// Пропускаем скрытые папки (например, .obsidian, .git)
		if info.IsDir() && strings.HasPrefix(info.Name(), ".") {
			return filepath.SkipDir
		}

		// Сохраняем только Markdown файлы
		if !info.IsDir() && strings.HasSuffix(info.Name(), ".md") {
			relPath, _ := filepath.Rel(rootPath, path)
			files = append(files, relPath)
		}
		return nil
	})

	return files, err
}

// AtomicWrite гарантирует безопасную запись файла даже при агрессивной работе Google Drive.
// Отлично пишет сырые бинарные данные ([]byte) без искажений.
func AtomicWrite(targetPath string, data []byte) error {
	dir := filepath.Dir(targetPath)

	// Обязательно создаем папки, если ИИ или маршрутизатор придумал новую
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("vfs: failed to create directory %s: %w", dir, err)
	}

	tmpFile := targetPath + ".tmp"
	if err := os.WriteFile(tmpFile, data, 0644); err != nil {
		return fmt.Errorf("vfs: failed to write temp file: %w", err)
	}

	err := withRetry(func() error {
		return os.Rename(tmpFile, targetPath)
	})

	if err != nil {
		os.Remove(tmpFile) // Очистка мусора при ошибке
		return fmt.Errorf("vfs: failed to atomically rename file: %w", err)
	}

	return nil
}

// SafeMove переносит файл, откатываясь к копированию, если Rename не сработал.
func SafeMove(oldPath, newPath string) error {
	dir := filepath.Dir(newPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	err := withRetry(func() error {
		return os.Rename(oldPath, newPath)
	})

	if err != nil {
		// Fallback к чтению и записи, если rename не работает между дисками
		data, readErr := os.ReadFile(oldPath)
		if readErr != nil {
			return fmt.Errorf("failed rename and fallback read: %v, %v", err, readErr)
		}
		if writeErr := AtomicWrite(newPath, data); writeErr != nil {
			return fmt.Errorf("failed rename and fallback write: %v, %v", err, writeErr)
		}
		os.Remove(oldPath)
	}

	return nil
}

// withRetry выполняет операцию с экспоненциальной задержкой и джиттером.
func withRetry(operation func() error) error {
	var err error
	for i := 0; i < maxRetries; i++ {
		err = operation()
		if err == nil {
			return nil
		}

		delay := time.Duration(baseDelayMs<<i) * time.Millisecond
		if delay > maxDelayMs*time.Millisecond {
			delay = maxDelayMs * time.Millisecond
		}

		// Защита от panic: rand.Intn требует значение > 0
		jitterMax := int(delay / 2)
		if jitterMax <= 0 {
			jitterMax = 1
		}
		jitter := time.Duration(rand.Intn(jitterMax))

		time.Sleep(delay + jitter)
	}
	return err
}
