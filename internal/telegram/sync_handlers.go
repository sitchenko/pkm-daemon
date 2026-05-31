package telegram

import (
	"log/slog"
	"strings"

	"gopkg.in/telebot.v3"

	"pkm-daemon/internal/storage"
	"pkm-daemon/internal/sync"
)

// BtnTaskDone — инлайн-кнопка для пометки задачи выполненной
var BtnTaskDone = telebot.InlineButton{Unique: "btn_done"}

// RegisterSyncHandlers регистрирует логику инлайн-кнопок для 15 этапа.
func RegisterSyncHandlers(bot *telebot.Bot, db *storage.Storage, vaultPath string, logger *slog.Logger) {
	bot.Handle(&BtnTaskDone, func(c telebot.Context) error {
		taskID := strings.TrimSpace(c.Data())
		if taskID == "" {
			return c.Respond(&telebot.CallbackResponse{
				Text:      "Ошибка: пустой ID задачи",
				ShowAlert: true,
			})
		}

		logger.Info("Inline button pressed: complete task", slog.String("taskID", taskID))

		// Запускаем двунаправленную синхронизацию
		err := sync.CompleteTask(taskID, db, bot, vaultPath)
		if err != nil {
			logger.Error("Sync Engine error", slog.Any("error", err))
			return c.Respond(&telebot.CallbackResponse{
				Text:      "Ошибка синхронизации!",
				ShowAlert: true,
			})
		}

		return c.Respond(&telebot.CallbackResponse{
			Text: "✅ Задача выполнена и синхронизирована!",
		})
	})
}