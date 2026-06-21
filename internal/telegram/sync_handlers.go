package telegram

import (
	"log/slog"
	"os"

	"gopkg.in/telebot.v3"

	"pkm-daemon/internal/fsm"
	"pkm-daemon/internal/storage"
)

// BtnDeleteNote — инлайн-кнопка для удаления заметки
var BtnDeleteNote = telebot.InlineButton{Unique: "btn_del_note"}

// RegisterSyncHandlers регистрирует логику инлайн-кнопок для 15 этапа.
func RegisterSyncHandlers(bot *telebot.Bot, db *storage.Storage, vaultPath string, logger *slog.Logger, fsmManager *fsm.Manager) {
	bot.Handle(&BtnDeleteNote, func(c telebot.Context) error {
		msgID := int64(c.Message().ID)
		filePath, err := db.DeleteNoteData(msgID)
		if err != nil {
			logger.Error("Failed to delete note data", slog.Any("error", err))
			return c.Respond(&telebot.CallbackResponse{
				Text:      "Ошибка при удалении из БД!",
				ShowAlert: true,
			})
		}

		err = os.Remove(filePath)
		if err != nil && !os.IsNotExist(err) {
			logger.Error("Failed to delete physical file", slog.Any("error", err))
		}

		_, err = bot.Edit(c.Message(), "🗑️ Заметка отменена и удалена")
		if err != nil {
			logger.Error("Failed to edit message on delete", slog.Any("error", err))
		}

		return c.Respond(&telebot.CallbackResponse{Text: "Заметка удалена!"})
	})
}
