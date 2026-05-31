package telegram

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"pkm-daemon/internal/fsm"
	"pkm-daemon/internal/obsidian"
	"pkm-daemon/internal/storage"

	telebot "gopkg.in/telebot.v3"
)

func (b *Bot) handleText(c telebot.Context) error {
	senderID := c.Sender().ID
	text := c.Text()

	session, err := b.fsm.Get(senderID)
	if err == nil && session.State == fsm.StateWaitReason {
		taskUUID := session.ContextData
		b.log.Info("Received FSM input for task failure", slog.String("task_uuid", taskUUID))

		filePath, err := b.db.GetFilePathByTaskUUID(taskUUID)
		if err != nil {
			return c.Send("❌ Не удалось найти файл задачи в базе.")
		}
		task, _ := b.db.GetTaskByID(taskUUID)

		if err := obsidian.FailTaskStatus(filePath, task.Content, text); err != nil {
			b.log.Error("Failed to edit MD file for failure", slog.Any("error", err))
			return c.Send("❌ Ошибка при редактировании .md файла.")
		}

		_ = b.db.UpdateTaskStatus(taskUUID, "Failed")
		_ = b.fsm.Clear(senderID)

		return c.Send("Статус задачи обновлен ✅\n<i>Причина записана в файл.</i>", telebot.ModeHTML)
	}

	b.log.Info("Processing standard note via AI", slog.Int64("user_id", senderID))

	mockFolders := []string{"00_Медиа", "01_Задачи", "02_Проекты", "03_Идеи", "Inbox"}
	mockTags := []string{"работа", "идея", "покупки", "важно"}

	aiResult, err := b.ai.AnalyzeNote(text, mockFolders, mockTags)
	if err != nil {
		b.log.Error("AI Analysis failed", slog.Any("error", err))
		return c.Send("❌ Ошибка при анализе текста ИИ.")
	}

	safeTitle := strings.Map(func(r rune) rune {
		if strings.ContainsRune(`\/:*?"<>|`, r) {
			return '_'
		}
		return r
	}, aiResult.Title)

	yamlTags := " []"
	if len(aiResult.Tags) > 0 {
		yamlTags = "\n  - " + strings.Join(aiResult.Tags, "\n  - ")
	}

	noteBody := text
	if aiResult.IsTask {
		noteBody = "- [ ] " + text
	}

	yamlFrontmatter := fmt.Sprintf(`---
date: %s
time: %s
status: active
is_task: %t
tags:%s
---
`, time.Now().Format("2006-01-02"), time.Now().Format("15:04:05"), aiResult.IsTask, yamlTags)

	fullNoteContent := yamlFrontmatter + "\n" + noteBody

	targetDir := filepath.Join(b.cfg.ObsidianPath, aiResult.Folder)
	_ = os.MkdirAll(targetDir, 0755)
	filePath := filepath.Join(targetDir, safeTitle+".md")
	
	if err := obsidian.AtomicWrite(filePath, []byte(fullNoteContent)); err != nil {
		return c.Send("❌ Ошибка записи файла в хранилище.")
	}

	msgID := int64(c.Message().ID)
	msgRecord := &storage.TelegramMessage{
		MessageID:      msgID,
		ChatID:         c.Chat().ID,
		TelegramUserID: senderID,
		FilePath:       filePath,
	}
	_ = b.db.SaveMessage(msgRecord)

	if aiResult.IsTask {
		taskUUID := fmt.Sprintf("task-%d", time.Now().UnixNano())
		taskRecord := &storage.TaskLedger{
			TaskUUID:     taskUUID,
			MessageID:    msgID,
			KanbanStatus: "To Do",
			Content:      noteBody,
			Deadline:     time.Now().Add(24 * time.Hour),
		}
		_ = b.db.SaveTask(taskRecord)

		// ИНТЕГРАЦИЯ ЭТАПА 7: Если ИИ нашел время, парсим и ставим напоминание
		if aiResult.ReminderTime != "" {
			triggerTime, parseErr := time.Parse(time.RFC3339, aiResult.ReminderTime)
			if parseErr != nil {
				b.log.Warn("Failed to parse reminder time from AI", slog.String("raw_time", aiResult.ReminderTime), slog.Any("error", parseErr))
			} else {
				reminder := &storage.Reminder{
					TaskUUID:       taskUUID,
					TriggerTime:    triggerTime,
					MessagePayload: noteBody,
					Status:         "pending",
				}
				if err := b.db.CreateReminder(reminder); err != nil {
					b.log.Error("Failed to save reminder to DB", slog.Any("error", err))
				} else {
					b.log.Info("Reminder scheduled successfully", slog.Time("trigger_time", triggerTime))
				}
			}
		}
	}

	if senderID == b.cfg.GuestID {
		adminMsg := fmt.Sprintf("👤 <b>Guest добавил заметку:</b> %s\n📁 Папка: %s", safeTitle, aiResult.Folder)
		_, _ = b.bot.Send(&telebot.User{ID: b.cfg.AdminID}, adminMsg, telebot.ModeHTML)
		return c.Send("Заметка успешно создана ✅")
	}

	reply := fmt.Sprintf("✅ Сохранено в папку <b>%s</b>\n📝 Заголовок: <i>%s</i>\n✅ Задача: %t", aiResult.Folder, safeTitle, aiResult.IsTask)
	if aiResult.ReminderTime != "" {
		reply += fmt.Sprintf("\n⏰ Будильник: %s", aiResult.ReminderTime)
	}
	return c.Send(reply, telebot.ModeHTML)
}

func (b *Bot) handleTasksCommand(c telebot.Context) error {
	if c.Sender().ID != b.cfg.AdminID {
		return c.Send("⛔ Доступ к дашборду задач есть только у Admin.")
	}

	tasks, err := b.db.GetActiveTasks()
	if err != nil || len(tasks) == 0 {
		return c.Send("🎉 Актуальных задач нет! Вы великолепны.")
	}

	for _, task := range tasks {
		menu := &telebot.ReplyMarkup{}
		btnDone := menu.Data("✅ Выполнено", "t_done", task.TaskUUID)
		btnFail := menu.Data("❌ Провалено", "t_fail", task.TaskUUID)
		
		menu.Inline(menu.Row(btnDone, btnFail))

		text := fmt.Sprintf("📌 <b>Задача:</b>\n%s", strings.ReplaceAll(task.Content, "- [ ] ", ""))
		_ = c.Send(text, menu, telebot.ModeHTML)
	}
	return nil
}

func (b *Bot) handleTaskDoneCallback(c telebot.Context) error {
	taskUUID := c.Data()
	
	filePath, err := b.db.GetFilePathByTaskUUID(taskUUID)
	if err == nil {
		task, _ := b.db.GetTaskByID(taskUUID)
		_ = obsidian.UpdateTaskStatus(filePath, task.Content, true)
		_ = b.db.UpdateTaskStatus(taskUUID, "Done")
	}

	_ = c.Edit("✅ <b>Выполнено!</b>", telebot.ModeHTML)
	return c.Respond(&telebot.CallbackResponse{Text: "Статус обновлен"})
}

func (b *Bot) handleTaskFailCallback(c telebot.Context) error {
	taskUUID := c.Data()

	err := b.fsm.Set(c.Sender().ID, fsm.StateWaitReason, taskUUID)
	if err != nil {
		return c.Respond(&telebot.CallbackResponse{Text: "Ошибка БД", ShowAlert: true})
	}

	_ = c.Edit("❌ <i>Отмена задачи...</i>\n\n<b>Укажи причину провала в следующем сообщении:</b>", telebot.ModeHTML)
	return c.Respond()
}