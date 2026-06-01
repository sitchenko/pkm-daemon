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
// Возвращает строку с готовой Obsidian-ссылкой вида ![[Photo_YYYYMMDD_HHMMSS.ext]]
func DownloadAndSaveMedia(c telebot.Context, bot *telebot.Bot, vaultPath string) (string, error) {
	var fileID string
	var ext string
	var folder string
	var prefix string

	msg := c.Message()

	// 1. Определение типа медиафайла
	if msg.Photo != nil {
		// Берем фото в максимальном разрешении (telebot отдает массив, где последний элемент — лучший)
		fileID = msg.Photo.FileID
		ext = ".jpg"
		folder = "Photos"
		prefix = "Photo"
	} else if msg.Voice != nil {
		fileID = msg.Voice.FileID
		ext = ".ogg"
		folder = "Voice"
		prefix = "Voice"
	} else if msg.Video != nil {
		fileID = msg.Video.FileID
		ext = ".mp4"
		folder = "Videos"
		prefix = "Video"
	} else if msg.Document != nil {
		fileID = msg.Document.FileID
		if msg.Document.FileName != "" {
			ext = filepath.Ext(msg.Document.FileName)
		} else {
			ext = ".dat"
		}
		folder = "Files"
		prefix = "Doc"
	} else {
		return "", fmt.Errorf("unsupported media type")
	}

	// 2. Получение объекта файла через API Telegram
	file, err := bot.FileByID(fileID)
	if err != nil {
		return "", fmt.Errorf("failed to get file info by ID: %w", err)
	}

	// 3. Скачивание файла в поток
	readCloser, err := bot.File(&file)
	if err != nil {
		return "", fmt.Errorf("failed to download file stream: %w", err)
	}
	defer readCloser.Close()

	// 4. Чтение бинарных данных в память
	data, err := io.ReadAll(readCloser)
	if err != nil {
		return "", fmt.Errorf("failed to read file data into memory: %w", err)
	}

	// 5. Генерация имени и путей
	timestamp := time.Now().Format("20060102_150405")
	fileName := fmt.Sprintf("%s_%s%s", prefix, timestamp, ext)

	targetDir := filepath.Join(vaultPath, "00_Медиа", folder)
	fullPath := filepath.Join(targetDir, fileName)

	// 6. Безопасное сохранение бинарных данных через VFS
	if err := vfs.AtomicWrite(fullPath, data); err != nil {
		return "", fmt.Errorf("failed to save media file to vfs: %w", err)
	}

	// 7. Возврат стандартной ссылки Obsidian
	obsidianLink := fmt.Sprintf("![[%s]]", fileName)
	return obsidianLink, nil
}
