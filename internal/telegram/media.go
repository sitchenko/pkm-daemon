package telegram

import (
	"fmt"
	"io"
	"path/filepath"
	"time"

	"gopkg.in/telebot.v3"

	"pkm-daemon/internal/vfs"
)

// DownloadAndSaveMedia определяет тип файла, скачивает его и сохраняет в хранилище Obsidian.
// Возвращает строку с готовой ссылкой, бинарные данные, MIME-тип и ошибку.
func DownloadAndSaveMedia(c telebot.Context, bot *telebot.Bot, vaultPath string) (string, []byte, string, error) {
	var fileID string
	var ext string
	var folder string
	var prefix string
	var mimeType string

	msg := c.Message()

	if msg.Photo != nil {
		fileID = msg.Photo.FileID
		ext = ".jpg"
		folder = "📸 Фото"
		prefix = "Photo"
		mimeType = "image/jpeg"
	} else if msg.Voice != nil {
		fileID = msg.Voice.FileID
		ext = ".ogg"
		folder = "🎙️ Голосовые"
		prefix = "Voice"
		mimeType = "audio/ogg"
	} else if msg.Video != nil {
		fileID = msg.Video.FileID
		ext = ".mp4"
		folder = "🎥 Видео"
		prefix = "Video"
		mimeType = "video/mp4"
	} else if msg.Document != nil {
		fileID = msg.Document.FileID
		if msg.Document.FileName != "" {
			ext = filepath.Ext(msg.Document.FileName)
		} else {
			ext = ".dat"
		}
		folder = "📁 Файлы"
		prefix = "Doc"
		mimeType = msg.Document.MIME
		if mimeType == "" {
			mimeType = "application/octet-stream"
		}
	} else {
		return "", nil, "", fmt.Errorf("unsupported media type")
	}

	file, err := bot.FileByID(fileID)
	if err != nil {
		return "", nil, "", fmt.Errorf("failed to get file info by ID: %w", err)
	}

	readCloser, err := bot.File(&file)
	if err != nil {
		return "", nil, "", fmt.Errorf("failed to download file stream: %w", err)
	}
	defer readCloser.Close()

	data, err := io.ReadAll(readCloser)
	if err != nil {
		return "", nil, "", fmt.Errorf("failed to read file data into memory: %w", err)
	}

	// ИСПРАВЛЕНИЕ: Добавляем ID сообщения, чтобы имена файлов не конфликтовали в рамках одной секунды
	msgID := msg.ID
	timestamp := time.Now().Format("20060102_150405")
	fileName := fmt.Sprintf("%s_%s_%d%s", prefix, timestamp, msgID, ext)

	targetDir := filepath.Join(vaultPath, "00_Медиа", folder)
	fullPath := filepath.Join(targetDir, fileName)

	if err := vfs.AtomicWrite(fullPath, data); err != nil {
		return "", nil, "", fmt.Errorf("failed to save media file to vfs: %w", err)
	}

	obsidianLink := fmt.Sprintf("![[%s]]", fileName)
	return obsidianLink, data, mimeType, nil
}
