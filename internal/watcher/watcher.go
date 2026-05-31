package watcher

import (
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/fsnotify/fsnotify"

	"pkm-daemon/internal/storage"
)

type Watcher struct {
	watcher   *fsnotify.Watcher
	debouncer *Debouncer
	db        *storage.Storage
	logger    *slog.Logger
	vaultPath string
}

func NewWatcher(vaultPath string, db *storage.Storage, logger *slog.Logger) (*Watcher, error) {
	fw, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}

	w := &Watcher{
		watcher:   fw,
		db:        db,
		logger:    logger,
		vaultPath: vaultPath,
	}

	// Инициализируем Debouncer с задержкой 2 секунды
	w.debouncer = NewDebouncer(2*time.Second, w.processFile)

	// Рекурсивно добавляем все существующие директории в watcher
	if err := w.watchDirectories(vaultPath); err != nil {
		fw.Close()
		return nil, err
	}

	return w, nil
}

func (w *Watcher) watchDirectories(root string) error {
	return filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil // Игнорируем ошибки доступа
		}

		if !d.IsDir() {
			return nil
		}

		// Игнорируем системные и скрытые директории (.obsidian, .git и т.д.)
		if d.IsDir() && strings.HasPrefix(d.Name(), ".") && path != root {
			return filepath.SkipDir
		}

		if err := w.watcher.Add(path); err != nil {
			w.logger.Warn("Failed to add directory to watcher", slog.String("path", path), slog.Any("error", err))
		} else {
			w.logger.Debug("Watching directory", slog.String("path", path))
		}

		return nil
	})
}

func (w *Watcher) Start() {
	w.logger.Info("File system watcher started", slog.String("root", w.vaultPath))

	for {
		select {
		case event, ok := <-w.watcher.Events:
			if !ok {
				return
			}

			// Если была создана новая директория — динамически добавляем её в наблюдение
			if event.Has(fsnotify.Create) {
				info, err := os.Stat(event.Name)
				if err == nil && info.IsDir() {
					if !strings.HasPrefix(info.Name(), ".") {
						_ = w.watcher.Add(event.Name)
						w.logger.Info("Dynamically added new directory to watcher", slog.String("path", event.Name))
					}
				}
			}

			// Отслеживаем только изменения Markdown-файлов
			if strings.HasSuffix(event.Name, ".md") {
				if event.Has(fsnotify.Write) || event.Has(fsnotify.Create) || event.Has(fsnotify.Rename) {
					w.debouncer.Trigger(event.Name)
				}
			}

		case err, ok := <-w.watcher.Errors:
			if !ok {
				return
			}
			w.logger.Error("Watcher encountered an error", slog.Any("error", err))
		}
	}
}

func (w *Watcher) processFile(path string) {
	info, err := os.Stat(path)
	if os.IsNotExist(err) {
		w.logger.Debug("File deleted or moved out, skipping index update", slog.String("path", path))
		return
	}
	if err != nil {
		w.logger.Error("Failed to stat file during debounce trigger", slog.String("path", path), slog.Any("error", err))
		return
	}

	// Файл существует. Обновляем его информацию в БД SQLite.
	index := &storage.VaultIndex{
		FilePath:     path,
		LastModified: info.ModTime(),
		SizeBytes:    info.Size(),
	}

	if err := w.db.UpsertVaultIndex(index); err != nil {
		w.logger.Error("Failed to update VaultIndex in DB", slog.String("path", path), slog.Any("error", err))
	} else {
		w.logger.Info("VaultIndex synchronized successfully", slog.String("path", path), slog.Int64("size", info.Size()))
	}
}

func (w *Watcher) Close() error {
	w.logger.Info("Stopping file system watcher...")
	return w.watcher.Close()
}