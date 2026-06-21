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

var (
	reCheckboxCompleted = regexp.MustCompile(`(?i)-\s+\[[xX]\]\s+(?:\*\*)?Задача\s+№([0-9-]+)`)
	reTaskID            = regexp.MustCompile(`(?i)Задача\s+№([0-9-]+)`)
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
	if err != nil { return nil, err }

	w := &Watcher{
		watcher:   fw,
		db:        db,
		logger:    logger,
		vaultPath: vaultPath,
	}
	w.debouncer = NewDebouncer(500*time.Millisecond, w.handleFileChange)

	err = filepath.Walk(vaultPath, func(path string, info os.FileInfo, err error) error {
		if info != nil && info.IsDir() {
			if strings.HasPrefix(info.Name(), ".") { return filepath.SkipDir }
			return fw.Add(path)
		}
		return nil
	})
	if err != nil { return nil, err }

	return w, nil
}

func (w *Watcher) SetBot(bot *telebot.Bot) { w.bot = bot }

func (w *Watcher) Start() {
	for {
		select {
		case event, ok := <-w.watcher.Events:
			if !ok { return }
			
			// Obsidian часто "пересоздает" файлы вместо Write, поэтому слушаем и Create
			isWrite := event.Op&fsnotify.Write == fsnotify.Write
			isCreate := event.Op&fsnotify.Create == fsnotify.Create
			
			if (isWrite || isCreate) && strings.HasSuffix(event.Name, ".md") {
				w.debouncer.Add(event.Name)
			}
		case err, ok := <-w.watcher.Errors:
			if !ok { return }
			w.logger.Error("Watcher error", slog.Any("error", err))
		}
	}
}

func (w *Watcher) Close() error {
	w.debouncer.Stop()
	return w.watcher.Close()
}

func checkTaskAndSync(taskID string, w *Watcher) {
	task, err := w.db.GetTaskByID(taskID)
	// Если задача в базе всё еще Pending — значит это ручное изменение в Obsidian!
	if err == nil && strings.ToLower(task.KanbanStatus) != "done" {
		w.logger.Info("Detected manual task completion in Obsidian", slog.String("taskID", taskID))
		if w.bot != nil {
			err = sync.ChangeTaskStatusAtomic(taskID, "Done", "", w.db, w.vaultPath)
			if err != nil {
				w.logger.Error("Failed to cascade sync completed task", slog.String("taskID", taskID), slog.Any("error", err))
			}
		} else {
			w.logger.Warn("Bot instance not set in watcher, skipping sync")
		}
	}
}

func (w *Watcher) handleFileChange(filePath string) {
	w.logger.Info("File change detected by Watcher", slog.String("file", filePath))

	data, err := os.ReadFile(filePath)
	if err != nil { return }
	content := string(data)
	fileName := filepath.Base(filePath)

	if fileName == "🎯 Канбан.md" || fileName == "Канбан.md" {
		// 1. Канбан: Ищем выполненные чекбоксы (если пользователь нажал галочку)
		for _, match := range reCheckboxCompleted.FindAllStringSubmatch(content, -1) {
			checkTaskAndSync(match[1], w)
		}
		
		// 2. Канбан: Ловим ПЕРЕТАСКИВАНИЯ в колонку "Готово"
		parts := strings.Split(content, "## ✅ Готово")
		if len(parts) > 1 {
			for _, match := range reTaskID.FindAllStringSubmatch(parts[1], -1) {
				checkTaskAndSync(match[1], w)
			}
		}
	} else if fileName == "Task_Manager.md" {
		// Task Manager: Ищем крестики
		for _, match := range reCheckboxCompleted.FindAllStringSubmatch(content, -1) {
			checkTaskAndSync(match[1], w)
		}
	} else {
		// Обычная заметка: Ищем совпадения текста с базой
		lines := strings.Split(content, "\n")
		tasks, err := w.db.GetActiveTasks()
		if err == nil {
			for _, line := range lines {
				clean := strings.TrimSpace(line)
				if strings.HasPrefix(clean, "- [x]") || strings.HasPrefix(clean, "- [X]") {
					taskText := strings.TrimSpace(clean[5:])
					for _, t := range tasks {
						if strings.HasSuffix(filePath, filepath.Base(t.FilePath)) && strings.Contains(taskText, strings.TrimSpace(t.Content)) {
							checkTaskAndSync(t.TaskUUID, w)
						}
					}
				}
			}
		}
	}
}