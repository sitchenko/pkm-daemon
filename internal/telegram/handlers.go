package telegram

import (
	"pkm-daemon/internal/fsm"
	"pkm-daemon/internal/obsidian"
	telebot "gopkg.in/telebot.v3"
)

func (b *Bot) handleText(c telebot.Context) error {
	senderID := c.Sender().ID
	text := c.Text()

	session, _ := b.fsm.Get(senderID)
	if session != nil && session.State == fsm.StateWaitReason {
		filePath, _ := b.db.GetFilePathByTaskUUID(session.ContextData)
		task, _ := b.db.GetTaskByID(session.ContextData)
		_ = obsidian.FailTaskStatus(filePath, task.Content, text)
		_ = b.db.UpdateTaskStatus(session.ContextData, "Failed")
		_ = b.fsm.Clear(senderID)
		return c.Send("Статус задачи обновлен (Провалено) ✅")
	}

	// ВРЕМЕННАЯ ЗАГЛУШКА ДЛЯ КОМПИЛЯЦИИ: Бот пока просто отправляет текст в ИИ с пустым списком
	_, err := b.ai.AnalyzeNote(text, []string{})
	if err != nil {
		return c.Send("Ошибка ИИ")
	}

	return c.Send("Заметка отправлена в ИИ (Режим отладки) ✅")
}

func (b *Bot) handleTasksCommand(c telebot.Context) error { return nil }
func (b *Bot) handleTaskDoneCallback(c telebot.Context) error { return nil }
func (b *Bot) handleTaskFailCallback(c telebot.Context) error { return nil }