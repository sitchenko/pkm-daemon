package scheduler

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"pkm-daemon/internal/storage"

	telebot "gopkg.in/telebot.v3"
)

// cleanMarkdownForTelegram удаляет символы Obsidian, чтобы не ломать HTML-парсер
func cleanMarkdownForTelegram(s string) string {
	replacements := []string{"**", "*", "_", "[[", "]]", "#"}
	for _, r := range replacements {
		s = strings.ReplaceAll(s, r, "")
	}
	return s
}

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
					cleanPayload := cleanMarkdownForTelegram(r.MessagePayload)
					payload := fmt.Sprintf("⏰ <b>Напоминание:</b>\n%s", cleanPayload)
					
					markup := &telebot.ReplyMarkup{}
					btnAck := markup.Data("✅ Ознакомлен", "btn_ack_reminder", fmt.Sprintf("%d", r.ID))
					markup.Inline(markup.Row(btnAck))

					opts := &telebot.SendOptions{
						ParseMode:   telebot.ModeHTML,
						ReplyMarkup: markup,
					}

					msg, err := bot.Send(&telebot.User{ID: adminID}, payload, opts)
					if err != nil {
						logger.Error("Failed to send reminder to Telegram", slog.Uint64("reminder_id", uint64(r.ID)), slog.Any("error", err))
						continue
					}

					r.Status = "fired"
					r.TelegramMessageID = int64(msg.ID)
					if err := db.UpdateReminder(&r); err != nil {
						logger.Error("Failed to update reminder after firing", slog.Uint64("reminder_id", uint64(r.ID)), slog.Any("error", err))
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
					// Delete old message
					if r.TelegramMessageID > 0 {
						oldMsg := &telebot.Message{ID: int(r.TelegramMessageID), Chat: &telebot.Chat{ID: adminID}}
						_ = bot.Delete(oldMsg)
					}

					cleanPayload := cleanMarkdownForTelegram(r.MessagePayload)
					payload := fmt.Sprintf("🚨 <b>ПОВТОРНОЕ НАПОМИНАНИЕ:</b>\n%s\n\n<i>Пожалуйста, подтвердите прочтение!</i>", cleanPayload)
					
					markup := &telebot.ReplyMarkup{}
					btnAck := markup.Data("✅ Ознакомлен", "btn_ack_reminder", fmt.Sprintf("%d", r.ID))
					markup.Inline(markup.Row(btnAck))

					opts := &telebot.SendOptions{
						ParseMode:           telebot.ModeHTML,
						ReplyMarkup:         markup,
						DisableNotification: false, // Force sound and vibration
					}

					msg, err := bot.Send(&telebot.User{ID: adminID}, payload, opts)
					if err != nil {
						logger.Error("Failed to send escalation to Telegram", slog.Uint64("reminder_id", uint64(r.ID)), slog.Any("error", err))
						continue
					}

					// Only 1 escalation, so we mark it as expired.
					r.Status = "expired"
					r.TelegramMessageID = int64(msg.ID)
					if err := db.UpdateReminder(&r); err != nil {
						logger.Error("Failed to update escalated reminder", slog.Uint64("reminder_id", uint64(r.ID)), slog.Any("error", err))
					} else {
						logger.Info("Reminder escalated successfully", slog.Uint64("reminder_id", uint64(r.ID)))
					}
				}
			}
		}
	}
}