package scheduler

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"pkm-daemon/internal/storage"

	telebot "gopkg.in/telebot.v3"
)

// Start запускает фоновый процесс проверки и отправки напоминаний
func Start(ctx context.Context, bot *telebot.Bot, db *storage.Storage, adminID int64, logger *slog.Logger) {
	ticker := time.NewTicker(30 * time.Second) // Проверяем базу каждые 30 секунд
	defer ticker.Stop()

	logger.Info("Scheduler started", slog.Duration("interval", 30*time.Second))

	for {
		select {
		case <-ctx.Done():
			logger.Info("Scheduler gracefully stopped")
			return
		case <-ticker.C:
			now := time.Now()
			reminders, err := db.GetPendingReminders(now)
			if err != nil {
				logger.Error("Failed to fetch pending reminders", slog.Any("error", err))
				continue
			}

			for _, r := range reminders {
				payload := fmt.Sprintf("⏰ <b>Напоминание:</b>\n%s", r.MessagePayload)
				
				// Отправляем сообщение Админу
				_, err := bot.Send(&telebot.User{ID: adminID}, payload, telebot.ModeHTML)
				if err != nil {
					logger.Error("Failed to send reminder to Telegram", slog.Uint64("reminder_id", uint64(r.ID)), slog.Any("error", err))
					continue
				}

				// Помечаем как выполненное
				if err := db.MarkReminderFired(r.ID); err != nil {
					logger.Error("Failed to mark reminder as fired", slog.Uint64("reminder_id", uint64(r.ID)), slog.Any("error", err))
				} else {
					logger.Info("Reminder fired successfully", slog.Uint64("reminder_id", uint64(r.ID)))
				}
			}
		}
	}
}