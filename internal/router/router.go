package router

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"pkm-daemon/internal/ai"
	"pkm-daemon/internal/storage"
	"pkm-daemon/internal/tasks"
	"pkm-daemon/internal/vfs"
)

// RouteAndSave сохраняет заметку и регистрирует задачи.
// ВОЗВРАЩАЕТ: baseID (для кнопок), Итоговый абсолютный путь к файлу (для БД), и ошибку.
func RouteAndSave(aiResult *ai.AnalysisResult, content []byte, vaultPath string, db *storage.Storage) (string, string, error) {
	targetDir := filepath.Join(vaultPath, aiResult.TargetFolder)

	if err := os.MkdirAll(targetDir, 0755); err != nil {
		return "", "", fmt.Errorf("failed to create directory: %w", err)
	}

	// ИСПРАВЛЕНИЕ: Безусловный вызов TrimSuffix по рекомендации линтера S1017
	fileName := strings.TrimSuffix(aiResult.FileName, ".md")

	// Привязываем дату к файлу согласно ТЗ
	currentDate := time.Now().Format("2006-01-02")
	if !strings.HasPrefix(fileName, currentDate) {
		fileName = fmt.Sprintf("%s_%s", currentDate, fileName)
	}
	fileName += ".md"

	aiResult.FileName = fileName

	fullPath := filepath.Join(targetDir, fileName)

	if err := vfs.AtomicWrite(fullPath, content); err != nil {
		return "", "", fmt.Errorf("failed to write file: %w", err)
	}

	baseID := time.Now().Format("150405")

	if aiResult.IsTask && len(aiResult.Tasks) > 0 {
		for i, taskStr := range aiResult.Tasks {
			idSuffix := fmt.Sprintf("-%d", i+1)
			err := tasks.RegisterTask(taskStr, fileName, fullPath, vaultPath, baseID, idSuffix, db, slog.Default())
			if err != nil {
				return baseID, fullPath, fmt.Errorf("failed to register task %d: %w", i, err)
			}
		}
	}

	if len(aiResult.Reminders) > 0 {
		for _, rem := range aiResult.Reminders {
			t, err := time.Parse(time.RFC3339, rem.Time)
			if err == nil {
				_ = db.CreateReminder(&storage.Reminder{
					TaskUUID:       baseID,
					TriggerTime:    t,
					MessagePayload: rem.Text,
					Status:         "pending",
				})
			}
		}
	}

	return baseID, fullPath, nil
}
