package telegram

import (
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"gopkg.in/telebot.v3"

	"pkm-daemon/internal/markdown"
	"pkm-daemon/internal/router"
	"pkm-daemon/internal/storage"
	"pkm-daemon/internal/vfs"
)

var albumCache sync.Map
var albumCacheMu sync.Mutex

type albumData struct {
	Contexts   []telebot.Context
	LoadingMsg *telebot.Message
	Timer      *time.Timer
	mu         sync.Mutex
}

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

	go b.processNotePipelineAsync(c, text, nil, "", loadingMsg)
	return nil
}

func (b *Bot) handleMedia(c telebot.Context) error {
	b.log.Info("Received media message", slog.String("user", c.Sender().Username))

	groupID := c.Message().AlbumID
	isAlbum := groupID != ""
	if !isAlbum {
		groupID = fmt.Sprintf("single_%d", c.Message().ID)
	}

	albumCacheMu.Lock()
	val, loaded := albumCache.Load(groupID)
	var album *albumData

	if !loaded {
		album = &albumData{}
		albumCache.Store(groupID, album)
	} else {
		album = val.(*albumData)
	}
	albumCacheMu.Unlock()

	album.mu.Lock()
	album.Contexts = append(album.Contexts, c)

	if !loaded {
		loadingMsg, err := b.bot.Send(c.Chat(), "📥 Принимаю медиафайл(ы)...")
		if err != nil {
			b.log.Error("Failed to send loading message", slog.Any("error", err))
		}
		album.LoadingMsg = loadingMsg

		duration := 2 * time.Second
		if !isAlbum {
			duration = 10 * time.Millisecond
		}

		album.Timer = time.AfterFunc(duration, func() {
			albumCache.Delete(groupID)
			b.processAlbum(album)
		})
	} else {
		if isAlbum && album.Timer != nil {
			album.Timer.Reset(2 * time.Second)
		}
	}
	album.mu.Unlock()

	return nil
}

func (b *Bot) processAlbum(album *albumData) {
	album.mu.Lock()
	contexts := album.Contexts
	loadingMsg := album.LoadingMsg
	album.mu.Unlock()

	if len(contexts) == 0 {
		return
	}

	if loadingMsg != nil {
		b.bot.Edit(loadingMsg, "📥 Сохраняю файлы локально...")
	}

	var links []string
	var finalCaption string
	var primaryMediaBytes []byte
	var primaryMimeType string

	for _, c := range contexts {
		obsidianLink, data, mimeType, err := DownloadAndSaveMedia(c, b.bot, b.cfg.ObsidianPath)
		if err != nil {
			b.log.Error("Failed to download media", slog.Any("error", err))
			continue
		}
		links = append(links, obsidianLink)

		if cap := c.Message().Caption; cap != "" {
			finalCaption = cap
		}

		// Если это аудио, войс или кружок - отдаем ИИ на транскрибацию
		if c.Message().Voice != nil || c.Message().Audio != nil || c.Message().VideoNote != nil {
			primaryMediaBytes = data
			primaryMimeType = mimeType
			if finalCaption == "" {
				if c.Message().Voice != nil {
					finalCaption = "Голосовая заметка"
				}
				if c.Message().Audio != nil {
					finalCaption = "Аудиозапись/Музыка"
				}
				if c.Message().VideoNote != nil {
					finalCaption = "Видеосообщение (Кружок)"
				}
			}
		} else if len(primaryMediaBytes) == 0 {
			// Если звука нет, передаем первое фото/видео для анализа картинки
			primaryMediaBytes = data
			primaryMimeType = mimeType
		}
	}

	if len(links) == 0 {
		if loadingMsg != nil {
			b.bot.Edit(loadingMsg, "❌ Ошибка скачивания файлов.")
		}
		return
	}

	if finalCaption == "" {
		finalCaption = "Медиафайл(ы)"
	}

	combinedText := fmt.Sprintf("%s\n\n%s", strings.Join(links, "\n"), finalCaption)

	if loadingMsg != nil {
		b.bot.Edit(loadingMsg, "⏳ Медиа сохранено. Анализирую (Gemini)...")
	}

	b.processNotePipelineAsync(contexts[0], combinedText, primaryMediaBytes, primaryMimeType, loadingMsg)
}

func (b *Bot) processNotePipelineAsync(c telebot.Context, text string, mediaBytes []byte, mimeType string, loadingMsg *telebot.Message) {
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

	aiResult, err := b.ai.AnalyzeNote(text, scannedPaths, mediaBytes, mimeType)
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

	baseID, finalFilePath, err := router.RouteAndSave(aiResult, mdContent, b.cfg.ObsidianPath, b.db)
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
		finalText := "✅ Заметка успешно создана и сохранена!"
		msg := c.Message()
		if msg.Photo != nil || msg.Voice != nil || msg.Video != nil || msg.Document != nil || msg.VideoNote != nil || msg.Audio != nil || len(mediaBytes) > 0 {
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
			FilePath:       finalFilePath,
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
