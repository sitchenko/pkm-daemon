package telegram

import (
	"fmt"
	"log/slog"
	"path/filepath"

	"gopkg.in/telebot.v3"

	"pkm-daemon/internal/markdown"
	"pkm-daemon/internal/router"
	"pkm-daemon/internal/storage"
	"pkm-daemon/internal/vfs"
)

// Обработчик простых текстовых сообщений
func (b *Bot) handleText(c telebot.Context) error {
	text := c.Text()
	if text == "" {
		return nil
	}

	b.log.Info("Received text message", slog.String("user", c.Sender().Username))

	loadingMsg, err := b.bot.Send(c.Chat(), "⏳ Анализирую заметку (Gemini)...")
	if err != nil {
		b.log.Error("Failed to send loading message", slog.Any("error", err))
	}

	go b.processNotePipelineAsync(c, text, loadingMsg)
	return nil
}

// Обработчик медиафайлов (Фото, Аудио, Видео, Документы)
func (b *Bot) handleMedia(c telebot.Context) error {
	b.log.Info("Received media message", slog.String("user", c.Sender().Username))

	loadingMsg, err := b.bot.Send(c.Chat(), "📥 Скачиваю медиафайл...")
	if err != nil {
		b.log.Error("Failed to send loading message", slog.Any("error", err))
	}

	go func() {
		defer func() {
			if r := recover(); r != nil {
				b.log.Error("Panic in handleMedia download goroutine", slog.Any("panic", r))
				if loadingMsg != nil {
					b.bot.Edit(loadingMsg, "❌ Критическая ошибка при скачивании медиа!")
				}
			}
		}()

		// 1. Скачиваем медиа
		obsidianLink, err := DownloadAndSaveMedia(c, b.bot, b.cfg.ObsidianPath)
		if err != nil {
			b.log.Error("Failed to download and save media", slog.Any("error", err))
			if loadingMsg != nil {
				b.bot.Edit(loadingMsg, "❌ Ошибка при сохранении медиафайла.")
			}
			return
		}

		// 2. Извлекаем или генерируем подпись
		caption := c.Message().Caption
		if c.Message().Voice != nil && caption == "" {
			caption = "Голосовая заметка"
		} else if caption == "" {
			caption = "Медиафайл"
		}

		// 3. Формируем комбинированный текст для ИИ
		combinedText := fmt.Sprintf("%s\n\n%s", obsidianLink, caption)

		if loadingMsg != nil {
			b.bot.Edit(loadingMsg, "⏳ Медиа скачано. Анализирую (Gemini)...")
		}

		// 4. Передаем в общий пайплайн
		b.processNotePipelineAsync(c, combinedText, loadingMsg)
	}()

	return nil
}

// Общий унифицированный пайплайн для анализа и сохранения (DRY)
func (b *Bot) processNotePipelineAsync(c telebot.Context, text string, loadingMsg *telebot.Message) {
	defer func() {
		if r := recover(); r != nil {
			b.log.Error("Panic in pipeline goroutine", slog.Any("panic", r))
			if loadingMsg != nil {
				b.bot.Edit(loadingMsg, "❌ Критическая ошибка при обработке (Panic)!")
			}
		}
	}()

	scannedPaths, err := vfs.ScanVault(b.cfg.ObsidianPath)
	if err != nil {
		b.log.Error("VFS Scan failed", slog.Any("error", err))
		if loadingMsg != nil {
			b.bot.Edit(loadingMsg, "❌ Ошибка: не удалось просканировать хранилище.")
		}
		return
	}

	aiResult, err := b.ai.AnalyzeNote(text, scannedPaths)
	if err != nil {
		b.log.Error("AI Analysis failed", slog.Any("error", err))
		if loadingMsg != nil {
			b.bot.Edit(loadingMsg, fmt.Sprintf("❌ Ошибка ИИ:\n%v", err))
		}
		return
	}

	mdContent, err := markdown.GenerateNote(*aiResult)
	if err != nil {
		b.log.Error("Markdown generation failed", slog.Any("error", err))
		if loadingMsg != nil {
			b.bot.Edit(loadingMsg, "❌ Ошибка генерации Markdown.")
		}
		return
	}

	baseID, err := router.RouteAndSave(*aiResult, mdContent, b.cfg.ObsidianPath, b.db)
	if err != nil {
		b.log.Error("Router failed", slog.Any("error", err))
		if loadingMsg != nil {
			b.bot.Edit(loadingMsg, "❌ Ошибка записи файлов.")
		}
		return
	}

	var markup *telebot.ReplyMarkup
	if len(aiResult.Tasks) > 0 {
		markup = &telebot.ReplyMarkup{}
		var rows []telebot.Row
		for i, taskText := range aiResult.Tasks {
			taskUUID := fmt.Sprintf("%s-%d", baseID, i+1)
			btn := markup.Data(fmt.Sprintf("✅ %s", shortenText(taskText, 35)), "btn_done", taskUUID)
			rows = append(rows, markup.Row(btn))
		}
		markup.Inline(rows...)
	}

	if loadingMsg != nil {
		// Динамическое сообщение в зависимости от типа входных данных
		finalText := "✅ Заметка успешно создана и сохранена!"
		if c.Message().Photo != nil || c.Message().Voice != nil || c.Message().Video != nil || c.Message().Document != nil {
			finalText = "✅ Медиа сохранено и заметка успешно создана!"
		}

		_, err = b.bot.Edit(loadingMsg, finalText, markup)
		if err != nil {
			b.log.Error("Failed to edit final message", slog.Any("error", err))
		}

		msgLink := storage.TelegramMessage{
			MessageID:      int64(loadingMsg.ID),
			ChatID:         loadingMsg.Chat.ID,
			TelegramUserID: c.Sender().ID,
			FilePath:       filepath.Join(b.cfg.ObsidianPath, aiResult.TargetFolder, aiResult.FileName+".md"),
		}
		b.db.SaveMessage(&msgLink)

		for i := range aiResult.Tasks {
			taskUUID := fmt.Sprintf("%s-%d", baseID, i+1)
			b.db.UpdateTaskMessageID(taskUUID, int64(loadingMsg.ID))
		}
	}
}

func shortenText(s string, max int) string {
	runes := []rune(s)
	if len(runes) > max {
		return string(runes[:max-3]) + "..."
	}
	return s
}
