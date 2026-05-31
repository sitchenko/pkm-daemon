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
			if event.Op&fsnotify.Write == fsnotify.Write && strings.HasSuffix(event.Name, ".md") {
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
			sync.CompleteTask(taskID, w.db, w.bot, w.vaultPath)
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
		// КАНБАН: Ловим перетаскивания в колонку "Готово" (даже если галочку не поставили)
		parts := strings.Split(content, "## ✅ Готово")
		if len(parts) > 1 {
			reID := regexp.MustCompile(`(?i)Задача\s+№([0-9-]+)`)
			for _, match := range reID.FindAllStringSubmatch(parts[1], -1) {
				checkTaskAndSync(match[1], w)
			}
		}
	} else if fileName == "Task_Manager.md" {
		// TASK MANAGER: Ловим поставленные крестики - [x]
		reID := regexp.MustCompile(`(?i)-\s+\[[xX]\]\s+(?:\*\*)?Задача\s+№([0-9-]+)`)
		for _, match := range reID.FindAllStringSubmatch(content, -1) {
			checkTaskAndSync(match[1], w)
		}
	} else {
		// ОБЫЧНАЯ ЗАМЕТКА: Здесь нет ID, ищем по соответствию текста с SQLite
		lines := strings.Split(content, "\n")
		tasks, err := w.db.GetActiveTasks()
		if err == nil {
			for _, line := range lines {
				clean := strings.TrimSpace(line)
				if strings.HasPrefix(clean, "- [x]") || strings.HasPrefix(clean, "- [X]") {
					taskText := strings.TrimSpace(clean[5:])
					for _, t := range tasks {
						// Если путь совпадает и текст из заметки содержится в БД
						if t.FilePath == filePath && strings.Contains(taskText, t.Content) {
							checkTaskAndSync(t.TaskUUID, w)
						}
					}
				}
			}
		}
	}
}