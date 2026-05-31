package router

import (
	"fmt"
	"log/slog"
	"path/filepath"
	"strings"
	"time"

	"pkm-daemon/internal/ai"
	"pkm-daemon/internal/tasks"
	"pkm-daemon/internal/vfs"
)

// handleTaskRegistration разбивает заметку на отдельные подзадачи и регистрирует их в Канбане
func handleTaskRegistration(result ai.AnalysisResult, noteNameForLink, vaultPath string) {
	if !result.IsTask {
		return
	}

	// Если есть массив независимых подзадач — регистрируем каждую отдельно
	if len(result.Tasks) > 0 {
		for i, taskText := range result.Tasks {
			// Добавляем суффикс -1, -2 для задач, созданных в одну секунду
			suffix := fmt.Sprintf("-%d", i+1)
			if err := tasks.RegisterTask(taskText, noteNameForLink, vaultPath, suffix, slog.Default()); err != nil {
				slog.Default().Error("Kanban Engine Error", slog.Any("error", err))
			}
		}
	} else {
		// Резервный вариант, если ИИ вернул IsTask = true, но массив пустой
		if err := tasks.RegisterTask(result.Content, noteNameForLink, vaultPath, "", slog.Default()); err != nil {
			slog.Default().Error("Kanban Engine Error", slog.Any("error", err))
		}
	}
}

func RouteAndSave(result ai.AnalysisResult, markdownData []byte, vaultPath string) error {
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

		handleTaskRegistration(result, noteNameForLink, vaultPath)
		return nil
	}

	fullPath := filepath.Join(vaultPath, targetFolder, fileNameWithExt)

	if err := vfs.AtomicWrite(fullPath, markdownData); err != nil {
		return fmt.Errorf("failed to write note: %w", err)
	}

	handleTaskRegistration(result, noteNameForLink, vaultPath)
	return nil
}