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

func CompleteTask(taskID string, db *storage.Storage, bot *telebot.Bot, vaultPath string) error {
	slog.Info("Starting Bi-directional Sync for task", slog.String("taskID", taskID))

	task, err := db.GetTaskByID(taskID)
	if err != nil { return err }

	if strings.ToLower(task.KanbanStatus) == "done" { return nil }

	db.UpdateTaskStatus(taskID, "Done")

	// 1. Обновляем саму Заметку (поиск по тексту)
	if task.FilePath != "" {
		replaceTaskStatusByContent(task.FilePath, task.Content)
	}

	// 2. БЕЗУСЛОВНО обновляем Task Manager и Kanban
	tasksFolderPath := filepath.Join(vaultPath, "01_Задачи")
	
	tmPath := filepath.Join(tasksFolderPath, "Task_Manager.md")
	replaceTaskStatusByID(tmPath, taskID)

	kanbanPath := filepath.Join(tasksFolderPath, "🎯 Канбан.md")
	updateKanbanFile(kanbanPath, taskID)

	// 3. ДИНАМИЧЕСКИЙ Telegram Update
	if task.MessageID != 0 && bot != nil {
		updateTelegramMessage(task.MessageID, db, bot)
	}
	return nil
}

func updateTelegramMessage(msgID int64, db *storage.Storage, bot *telebot.Bot) {
	tm, err := db.GetTelegramMessageByMessageID(msgID)
	if err != nil || tm == nil { return }
	tasks, err := db.GetTasksByMessageID(msgID)
	if err != nil { return }

	var msgText strings.Builder
	msgText.WriteString("✅ Заметка успешно создана и сохранена!\n\n")

	var markup telebot.ReplyMarkup
	var rows []telebot.Row

	for _, t := range tasks {
		if strings.ToLower(t.KanbanStatus) == "done" {
			msgText.WriteString(fmt.Sprintf("✅ <s>%s</s>\n", html.EscapeString(t.Content)))
		} else {
			msgText.WriteString(fmt.Sprintf("⏳ %s\n", html.EscapeString(t.Content)))
			btnText := "✅ " + t.Content
			if len([]rune(btnText)) > 35 { btnText = string([]rune(btnText)[:32]) + "..." }
			btn := markup.Data(btnText, "btn_done", t.TaskUUID)
			rows = append(rows, markup.Row(btn))
		}
	}

	msg := &telebot.Message{ID: int(tm.MessageID), Chat: &telebot.Chat{ID: tm.ChatID}}
	
	if len(rows) > 0 {
		markup.Inline(rows...)
		_, err = bot.Edit(msg, msgText.String(), &markup, telebot.ModeHTML)
	} else {
		_, err = bot.Edit(msg, msgText.String(), telebot.ModeHTML)
	}

	if err != nil { slog.Error("Failed to edit Telegram message", slog.Any("error", err)) }
}

func replaceTaskStatusByContent(filePath, contentStr string) error {
	data, err := os.ReadFile(filePath)
	if err != nil { return nil }

	lines := strings.Split(string(data), "\n")
	changed := false
	for i, line := range lines {
		if strings.Contains(line, contentStr) && strings.Contains(line, "- [ ]") {
			lines[i] = strings.Replace(line, "- [ ]", "- [x]", 1)
			changed = true
			break
		}
	}

	if changed { return vfs.AtomicWrite(filePath, []byte(strings.Join(lines, "\n"))) }
	return nil
}

func replaceTaskStatusByID(filePath, taskID string) error {
	data, err := os.ReadFile(filePath)
	if err != nil { return nil }

	content := string(data)
	if !strings.Contains(content, taskID) { return nil } // Оптимизация

	content = strings.Replace(content, "- [ ] **Задача №"+taskID, "- [x] **Задача №"+taskID, 1)
	content = strings.Replace(content, "- [ ] Задача №"+taskID, "- [x] Задача №"+taskID, 1)

	return vfs.AtomicWrite(filePath, []byte(content))
}

func updateKanbanFile(kanbanPath, taskID string) error {
	data, err := os.ReadFile(kanbanPath)
	if err != nil { return nil }
	
	lines := strings.Split(string(data), "\n")
	var newLines []string
	var taskLines []string
	capturing := false
	searchStr := "Задача №" + taskID

	for _, line := range lines {
		cleanLine := strings.TrimRight(line, "\r")
		
		// Если нашли задачу - начинаем захват
		if strings.Contains(cleanLine, searchStr) {
			capturing = true
			updatedLine := strings.Replace(cleanLine, "- [ ]", "- [x]", 1)
			taskLines = append(taskLines, updatedLine)
			continue
		}
		
		if capturing {
			// Захватываем вложенные элементы (ссылки на заметку)
			if strings.HasPrefix(cleanLine, "\t") || strings.HasPrefix(cleanLine, "  ") {
				taskLines = append(taskLines, cleanLine)
				continue
			} else {
				capturing = false
			}
		}
		
		if !capturing {
			newLines = append(newLines, cleanLine)
		}
	}
	
	if len(taskLines) == 0 { return nil }

	var finalLines []string
	hasDone, inserted := false, false
	
	for _, line := range newLines {
		finalLines = append(finalLines, line)
		if strings.HasPrefix(strings.TrimSpace(line), "## ✅ Готово") {
			hasDone = true
			finalLines = append(finalLines, taskLines...)
			inserted = true
		}
	}
	
	if !hasDone {
		finalLines = append(finalLines, "", "## ✅ Готово")
		finalLines = append(finalLines, taskLines...)
	} else if !inserted {
		finalLines = append(finalLines, taskLines...)
	}
	
	return vfs.AtomicWrite(kanbanPath, []byte(strings.Join(finalLines, "\n")))
}