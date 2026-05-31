package telegram

import (
	"log/slog"

	telebot "gopkg.in/telebot.v3"
)

// authMiddleware проверяет ID отправителя. Защита "от дурака" и посторонних.
func (b *Bot) authMiddleware() telebot.MiddlewareFunc {
	return func(next telebot.HandlerFunc) telebot.HandlerFunc {
		return func(c telebot.Context) error {
			senderID := c.Sender().ID

			// Если ID не совпадает ни с Админом, ни с Гостем — блокируем
			if senderID != b.cfg.AdminID && senderID != b.cfg.GuestID {
				b.log.Warn("Unauthorized access attempt blocked",
					slog.Int64("user_id", senderID),
					slog.String("username", c.Sender().Username),
				)
				return c.Send("⛔ Доступ запрещен. Вы не зарегистрированы в системе PKM.")
			}

			// Если авторизация пройдена, передаем управление хендлеру
			return next(c)
		}
	}
}