package watcher

import (
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/fsnotify/fsnotify"
	"gopkg.in/telebot.v3"

	"pkm-daemon/internal/storage"
	"pkm-daemon/internal/sync"
)

type Watcher struct {
	watcher   *fsnotify.Watcher
	db        *storage.Storage
	logger    *slog.Logger
	debouncer *Debouncer
	bot       *telebot.Bot
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
	
	// Исправлено: передаем duration и callback
	w.debouncer = NewDebouncer(500*time.Millisecond, w.handleFileChange)

	// Рекурсивно подписываемся на папки
	err = filepath.Walk(vaultPath, func(path string, info os.FileInfo, err error) error {
		if info != nil && info.IsDir() {
			if strings.HasPrefix(info.Name(), ".") {
				return filepath.SkipDir
			}
			return fw.Add(path)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	return w, nil
}

// SetBot внедряет инстанс Telegram-бота для каскадной обратной синхронизации
func (w *Watcher) SetBot(bot *telebot.Bot) {
	w.bot = bot
}

func (w *Watcher) Start() {
	for {
		select {
		case event, ok := <-w.watcher.Events:
			if !ok {
				return
			}
			if event.Op&fsnotify.Write == fsnotify.Write && strings.HasSuffix(event.Name, ".md") {
				// Используем исправленный метод Add
				w.debouncer.Add(event.Name)
			}
		case err, ok := <-w.watcher.Errors:
			if !ok {
				return
			}
			w.logger.Error("Watcher error", slog.Any("error", err))
		}
	}
}

func (w *Watcher) Close() error {
	w.debouncer.Stop()
	return w.watcher.Close()
}

func (w *Watcher) handleFileChange(filePath string) {
	w.logger.Info("File change detected by Watcher", slog.String("file", filePath))

	data, err := os.ReadFile(filePath)
	if err != nil {
		w.logger.Error("Failed to read changed file", slog.String("file", filePath), slog.Any("error", err))
		return
	}

	content := string(data)
	// Регулярка для поиска выполненных задач напрямую в Markdown
	re := regexp.MustCompile(`(?i)-\s+\[[xX]\]\s+(?:\*\*)?Задача\s+№([0-9-]+)(?:\*\*)?:`)
	matches := re.FindAllStringSubmatch(content, -1)

	for _, match := range matches {
		if len(match) > 1 {
			taskID := match[1]
			task, err := w.db.GetTaskByID(taskID)
			if err != nil {
				continue // Задачи нет в базе
			}

			// Разрыв бесконечного цикла: если в SQLite статус pending, а в файле [x],
			// значит пользователь отметил галочку вручную. Инициируем каскад!
			if strings.ToLower(task.KanbanStatus) == "pending" {
				w.logger.Info("Detected manual task completion in Markdown", slog.String("taskID", taskID))
				
				if w.bot != nil {
					err = sync.CompleteTask(taskID, w.db, w.bot, w.vaultPath)
					if err != nil {
						w.logger.Error("Failed to cascade sync completed task", slog.String("taskID", taskID), slog.Any("error", err))
					}
				} else {
					w.logger.Warn("Bot instance not set in watcher, skipping sync")
				}
			}
		}
	}
}