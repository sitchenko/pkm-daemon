package sync

import (
	"fmt"
	"html"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/telebot.v3"

	"pkm-daemon/internal/storage"
	"pkm-daemon/internal/vfs"
)

// CompleteTask — ядро двунаправленной каскадной синхронизации
func CompleteTask(taskID string, db *storage.Storage, bot *telebot.Bot, vaultPath string) error {
	slog.Info("Starting Bi-directional Sync for task", slog.String("taskID", taskID))

	// 1. Получаем задачу из SQLite
	task, err := db.GetTaskByID(taskID)
	if err != nil {
		return fmt.Errorf("task %s not found in DB: %w", taskID, err)
	}

	// Защита от бесконечного цикла (Debouncer level 2)
	if strings.ToLower(task.KanbanStatus) == "done" {
		slog.Info("Task already marked as Done, skipping sync cascade", slog.String("taskID", taskID))
		return nil
	}

	// 2. Обновляем статус в SQLite
	if err := db.UpdateTaskStatus(taskID, "Done"); err != nil {
		return fmt.Errorf("failed to update SQLite status: %w", err)
	}

	// 3. Обновляем саму Заметку (через атомарную запись)
	if task.FilePath != "" {
		if err := replaceTaskStatusByContent(task.FilePath, task.Content); err != nil {
			slog.Error("Failed to update Note file", slog.String("file", task.FilePath), slog.Any("error", err))
		}
	}

	// 4. Логика Parent/Child
	isAllChildrenDone := false
	if task.ParentID != "" {
		siblings, err := db.GetTasksByParentID(task.ParentID)
		if err == nil && len(siblings) > 0 {
			allDone := true
			for _, sib := range siblings {
				if strings.ToLower(sib.KanbanStatus) != "done" && sib.TaskUUID != taskID {
					allDone = false
					break
				}
			}
			isAllChildrenDone = allDone
			if allDone {
				slog.Info("All child tasks completed, cascading to ParentID", slog.String("parentID", task.ParentID))
				_ = CompleteTask(task.ParentID, db, bot, vaultPath)
			}
		}
	}

	// 5. Обновляем глобальные файлы (Task Manager & Kanban)
	// Только если задача без родителя, ИЛИ если все её "братья" тоже закрыты
	if task.ParentID == "" || isAllChildrenDone {
		tasksFolderPath := filepath.Join(vaultPath, "01_Задачи")
		
		tmPath := filepath.Join(tasksFolderPath, "Task_Manager.md")
		if err := replaceTaskStatusByID(tmPath, taskID); err != nil {
			slog.Error("Failed to update Task_Manager.md", slog.Any("error", err))
		}

		kanbanPath := filepath.Join(tasksFolderPath, "🎯 Канбан.md")
		if err := updateKanbanFile(kanbanPath, taskID); err != nil {
			slog.Error("Failed to update Kanban.md", slog.Any("error", err))
		}
	}

	// 6. Telegram Update (Зачеркиваем исходное сообщение)
	if task.MessageID != 0 && bot != nil {
		tm, err := db.GetTelegramMessageByMessageID(task.MessageID)
		if err == nil && tm != nil {
			msg := &telebot.Message{
				ID:   int(tm.MessageID),
				Chat: &telebot.Chat{ID: tm.ChatID},
			}
			
			// Используем HTML-тег <s> для зачеркивания (намного безопаснее, чем MarkdownV2)
			strikethroughText := "<s>" + html.EscapeString(task.Content) + "</s>"
			
			// Edit без ReplyMarkup автоматически удаляет инлайн-кнопки
			_, err = bot.Edit(msg, strikethroughText, &telebot.SendOptions{
				ParseMode: telebot.ModeHTML,
			})
			if err != nil {
				slog.Error("Failed to edit Telegram message", slog.Any("error", err))
			} else {
				slog.Info("Telegram message successfully strike-through updated")
			}
		}
	}

	slog.Info("Task sync cascade complete", slog.String("taskID", taskID))
	return nil
}

// replaceTaskStatusByContent ищет чекбокс по ТЕКСТУ задачи (для заметок, где нет ID)
func replaceTaskStatusByContent(filePath, content string) error {
	data, err := os.ReadFile(filePath)
	if err != nil {
		if os.IsNotExist(err) { return nil }
		return err
	}

	lines := strings.Split(string(data), "\n")
	changed := false
	for i, line := range lines {
		if strings.Contains(line, content) && strings.Contains(line, "- [ ]") {
			lines[i] = strings.Replace(line, "- [ ]", "- [x]", 1)
			changed = true
			break
		}
	}

	if changed {
		return vfs.AtomicWrite(filePath, []byte(strings.Join(lines, "\n")))
	}
	return nil
}

// replaceTaskStatusByID ищет чекбокс по ID (для Task_Manager)
func replaceTaskStatusByID(filePath, taskID string) error {
	data, err := os.ReadFile(filePath)
	if err != nil {
		if os.IsNotExist(err) { return nil }
		return err
	}

	lines := strings.Split(string(data), "\n")
	changed := false
	searchStr := "Задача №" + taskID

	for i, line := range lines {
		if strings.Contains(line, searchStr) && strings.Contains(line, "- [ ]") {
			lines[i] = strings.Replace(line, "- [ ]", "- [x]", 1)
			changed = true
			break
		}
	}

	if changed {
		return vfs.AtomicWrite(filePath, []byte(strings.Join(lines, "\n")))
	}
	return nil
}

// updateKanbanFile вырезает задачу из '🎯 К выполнению' и вставляет в '✅ Готово'
func updateKanbanFile(kanbanPath, taskID string) error {
	data, err := os.ReadFile(kanbanPath)
	if err != nil {
		if os.IsNotExist(err) { return nil }
		return err
	}

	lines := strings.Split(string(data), "\n")
	var newLines []string
	var taskLines []string

	inTargetSection := false
	capturingTask := false

	for _, line := range lines {
		cleanLine := strings.TrimRight(line, "\r")

		if strings.HasPrefix(strings.TrimSpace(cleanLine), "## 🎯 К выполнению") {
			inTargetSection = true
			newLines = append(newLines, cleanLine)
			continue
		} else if strings.HasPrefix(cleanLine, "## ") {
			inTargetSection = false
		}

		if inTargetSection {
			if strings.Contains(cleanLine, "Задача №"+taskID) {
				capturingTask = true
				updatedLine := strings.Replace(cleanLine, "- [ ]", "- [x]", 1)
				taskLines = append(taskLines, updatedLine)
				continue
			}
			if capturingTask {
				// Захватываем прикрепленную ссылку
				if strings.HasPrefix(cleanLine, "\t*[[") || strings.HasPrefix(cleanLine, "  *[[") {
					taskLines = append(taskLines, cleanLine)
					continue
				} else {
					capturingTask = false
				}
			}
		}

		if !capturingTask {
			newLines = append(newLines, cleanLine)
		}
	}

	if len(taskLines) > 0 {
		var finalLines []string
		hasDoneSection := false
		inserted := false

		for _, line := range newLines {
			finalLines = append(finalLines, line)
			if strings.HasPrefix(strings.TrimSpace(line), "## ✅ Готово") {
				hasDoneSection = true
				finalLines = append(finalLines, taskLines...)
				inserted = true
			}
		}

		if !hasDoneSection {
			finalLines = append(finalLines, "", "## ✅ Готово")
			finalLines = append(finalLines, taskLines...)
		} else if !inserted {
			finalLines = append(finalLines, taskLines...)
		}

		return vfs.AtomicWrite(kanbanPath, []byte(strings.Join(finalLines, "\n")))
	}

	return nil
}