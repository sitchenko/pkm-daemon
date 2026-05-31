package router

import (
	"fmt"
	"log/slog"
	"path/filepath"
	"strings"
	"time"

	"pkm-daemon/internal/ai"
	"pkm-daemon/internal/storage"
	"pkm-daemon/internal/tasks"
	"pkm-daemon/internal/vfs"
)

// handleTaskRegistration разбивает заметку на подзадачи и регистрирует их
func handleTaskRegistration(result ai.AnalysisResult, noteNameForLink, newNoteFullPath, vaultPath string, db *storage.Storage) {
	if !result.IsTask {
		return
	}

	// Генерируем единый ParentID для всей группы задач в рамках одной заметки
	baseID := time.Now().Format("150405")

	if len(result.Tasks) > 0 {
		for i, taskText := range result.Tasks {
			suffix := fmt.Sprintf("-%d", i+1)
			if err := tasks.RegisterTask(taskText, noteNameForLink, newNoteFullPath, vaultPath, baseID, suffix, db, slog.Default()); err != nil {
				slog.Default().Error("Kanban Engine Error", slog.Any("error", err))
			}
		}
	} else {
		if err := tasks.RegisterTask(result.Content, noteNameForLink, newNoteFullPath, vaultPath, baseID, "", db, slog.Default()); err != nil {
			slog.Default().Error("Kanban Engine Error", slog.Any("error", err))
		}
	}
}

// RouteAndSave принимает зависимость БД для проброса в Канбан-движок
func RouteAndSave(result ai.AnalysisResult, markdownData []byte, vaultPath string, db *storage.Storage) error {
	targetFolder := strings.TrimPrefix(result.TargetFolder, "/")
	targetFolder = strings.TrimPrefix(targetFolder, "\\")

	datePrefix := time.Now().Format("2006-01-02")
	fileNameWithExt := fmt.Sprintf("%s_%s.md", datePrefix, result.FileName)
	noteNameForLink := strings.TrimSuffix(fileNameWithExt, ".md")

	if result.Action == "reorganize" && result.ClusterName != "" && result.TargetFileToMove != "" {
		clusterFolder := filepath.Join(vaultPath, targetFolder, result.ClusterName)

		oldFileFullPath := filepath.Join(vaultPath, result.TargetFileToMove)
		oldFileName := filepath.Base(oldFileFullPath)
		newOldFileLocation := filepath.Join(clusterFolder, oldFileName)

		newNoteFullPath := filepath.Join(clusterFolder, fileNameWithExt)

		if err := vfs.SafeMove(oldFileFullPath, newOldFileLocation); err != nil {
			return fmt.Errorf("failed to move old file: %w", err)
		}

		if err := vfs.AtomicWrite(newNoteFullPath, markdownData); err != nil {
			return fmt.Errorf("failed to write new note: %w", err)
		}

		handleTaskRegistration(result, noteNameForLink, newNoteFullPath, vaultPath, db)
		return nil
	}

	fullPath := filepath.Join(vaultPath, targetFolder, fileNameWithExt)

	if err := vfs.AtomicWrite(fullPath, markdownData); err != nil {
		return fmt.Errorf("failed to write note: %w", err)
	}

	handleTaskRegistration(result, noteNameForLink, fullPath, vaultPath, db)
	return nil
}