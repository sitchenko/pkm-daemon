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

func (b *Bot) handleText(c telebot.Context) error {
	text := c.Text()
	if text == "" {
		return nil
	}

	b.log.Info("Received text message", slog.String("user", c.Sender().Username))

	// Даем моментальную обратную связь
	loadingMsg, err := b.bot.Send(c.Chat(), "⏳ Анализирую заметку (Gemini)...")
	if err != nil {
		b.log.Error("Failed to send loading message", slog.Any("error", err))
	}

	// Асинхронно обрабатываем тяжелые операции
	go func() {
		// Обязательный перехват паники
		defer func() {
			if r := recover(); r != nil {
				b.log.Error("Panic in handleText goroutine", slog.Any("panic", r))
				if loadingMsg != nil {
					b.bot.Edit(loadingMsg, "❌ Критическая ошибка при обработке (Panic)!")
				}
			}
		}()

		scannedPaths, err := vfs.ScanVault(b.cfg.ObsidianPath)
		if err != nil {
			b.log.Error("VFS Scan failed", slog.Any("error", err))
			if loadingMsg != nil { b.bot.Edit(loadingMsg, "❌ Ошибка: не удалось просканировать хранилище.") }
			return
		}

		aiResult, err := b.ai.AnalyzeNote(text, scannedPaths)
		if err != nil {
			b.log.Error("AI Analysis failed", slog.Any("error", err))
			if loadingMsg != nil { b.bot.Edit(loadingMsg, fmt.Sprintf("❌ Ошибка ИИ:\n%v", err)) }
			return
		}

		mdContent, err := markdown.GenerateNote(*aiResult)
		if err != nil {
			b.log.Error("Markdown generation failed", slog.Any("error", err))
			if loadingMsg != nil { b.bot.Edit(loadingMsg, "❌ Ошибка генерации Markdown.") }
			return
		}

		// Сохраняем файлы и забираем сгенерированный baseID
		baseID, err := router.RouteAndSave(*aiResult, mdContent, b.cfg.ObsidianPath, b.db)
		if err != nil {
			b.log.Error("Router failed", slog.Any("error", err))
			if loadingMsg != nil { b.bot.Edit(loadingMsg, "❌ Ошибка записи файлов.") }
			return
		}

		// Формируем Inline-кнопки на базе полученного baseID
		var markup *telebot.ReplyMarkup
		if len(aiResult.Tasks) > 0 {
			markup = &telebot.ReplyMarkup{}
			var rows []telebot.Row
			for i, taskText := range aiResult.Tasks {
				taskUUID := fmt.Sprintf("%s-%d", baseID, i+1)
				// Укорачиваем текст для кнопки, если он слишком длинный
				btn := markup.Data(fmt.Sprintf("✅ %s", shortenText(taskText, 35)), "btn_done", taskUUID)
				rows = append(rows, markup.Row(btn))
			}
			markup.Inline(rows...)
		}

		// Завершаем работу с пользователем
		if loadingMsg != nil {
			_, err = b.bot.Edit(loadingMsg, "✅ Заметка успешно создана и сохранена!", markup)
			if err != nil {
				b.log.Error("Failed to edit final message", slog.Any("error", err))
			}

			// Записываем исходное сообщение в лог, чтобы 15-й этап мог зачеркивать текст!
			msgLink := storage.TelegramMessage{
				MessageID:      int64(loadingMsg.ID),
				ChatID:         loadingMsg.Chat.ID,
				TelegramUserID: c.Sender().ID,
				FilePath:       filepath.Join(b.cfg.ObsidianPath, aiResult.TargetFolder, aiResult.FileName+".md"),
			}
			b.db.SaveMessage(&msgLink)

			// Привязываем кнопки к MessageID в SQLite
			for i := range aiResult.Tasks {
				taskUUID := fmt.Sprintf("%s-%d", baseID, i+1)
				b.db.UpdateTaskMessageID(taskUUID, int64(loadingMsg.ID))
			}
		}
	}()

	return nil
}

func shortenText(s string, max int) string {
	runes := []rune(s)
	if len(runes) > max {
		return string(runes[:max-3]) + "..."
	}
	return s
}