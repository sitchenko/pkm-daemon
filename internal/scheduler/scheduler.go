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
			
			// 1. Process pending reminders
			reminders, err := db.GetPendingReminders(now)
			if err != nil {
				logger.Error("Failed to fetch pending reminders", slog.Any("error", err))
			} else {
				for _, r := range reminders {
					payload := fmt.Sprintf("⏰ <b>Напоминание:</b>\n%s", r.MessagePayload)
					
					markup := &telebot.ReplyMarkup{}
					btnAck := markup.Data("✅ Ознакомлен", "btn_ack_reminder", fmt.Sprintf("%d", r.ID))
					markup.Inline(markup.Row(btnAck))

					_, err := bot.Send(&telebot.User{ID: adminID}, payload, telebot.ModeHTML, markup)
					if err != nil {
						logger.Error("Failed to send reminder to Telegram", slog.Uint64("reminder_id", uint64(r.ID)), slog.Any("error", err))
						continue
					}

					if err := db.MarkReminderFired(r.ID); err != nil {
						logger.Error("Failed to mark reminder as fired", slog.Uint64("reminder_id", uint64(r.ID)), slog.Any("error", err))
					} else {
						logger.Info("Reminder fired successfully", slog.Uint64("reminder_id", uint64(r.ID)))
					}
				}
			}

			// 2. Process escalations (fired > 10 mins ago, not acknowledged)
			escalations, err := db.GetRemindersForEscalation(now)
			if err != nil {
				logger.Error("Failed to fetch escalations", slog.Any("error", err))
			} else {
				for _, r := range escalations {
					// We can escalate up to 3 times, for example
					if r.EscalationLevel >= 3 {
						continue
					}

					payload := fmt.Sprintf("🚨 <b>ПОВТОРНОЕ НАПОМИНАНИЕ:</b>\n%s\n\n<i>Пожалуйста, подтвердите прочтение!</i>", r.MessagePayload)
					
					markup := &telebot.ReplyMarkup{}
					btnAck := markup.Data("✅ Ознакомлен", "btn_ack_reminder", fmt.Sprintf("%d", r.ID))
					markup.Inline(markup.Row(btnAck))

					// Force sound and vibration
					_, err := bot.Send(&telebot.User{ID: adminID}, payload, telebot.ModeHTML, markup, &telebot.SendOptions{DisableNotification: false})
					if err != nil {
						logger.Error("Failed to send escalation to Telegram", slog.Uint64("reminder_id", uint64(r.ID)), slog.Any("error", err))
						continue
					}

					// Update trigger time for the NEXT escalation in 10 mins if ignored again
					r.TriggerTime = now
					r.EscalationLevel += 1
					if err := db.UpdateReminder(&r); err != nil {
						logger.Error("Failed to update escalated reminder", slog.Uint64("reminder_id", uint64(r.ID)), slog.Any("error", err))
					} else {
						logger.Info("Reminder escalated successfully", slog.Uint64("reminder_id", uint64(r.ID)), slog.Int("level", r.EscalationLevel))
					}
				}
			}
		}
	}
}