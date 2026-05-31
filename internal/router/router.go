package router

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"pkm-daemon/internal/ai"
	"pkm-daemon/internal/vfs"
)

// RouteAndSave принимает решение от ИИ, бинарные данные Markdown и физически сохраняет/перемещает файлы в хранилище.
func RouteAndSave(result ai.AnalysisResult, markdownData []byte, vaultPath string) error {
	// Базовая защита: удаляем слеши в начале, чтобы путь не стал абсолютным (относительно корня диска)
	targetFolder := strings.TrimPrefix(result.TargetFolder, "/")
	targetFolder = strings.TrimPrefix(targetFolder, "\\")

	// Формируем имя файла с префиксом даты
	datePrefix := time.Now().Format("2006-01-02")
	fileNameWithExt := fmt.Sprintf("%s_%s.md", datePrefix, result.FileName)

	if result.Action == "reorganize" && result.ClusterName != "" && result.TargetFileToMove != "" {
		// ЛОГИКА REORGANIZE (Кластеризация)
		
		// 1. Формируем путь к новому кластеру (папке)
		clusterFolder := filepath.Join(vaultPath, targetFolder, result.ClusterName)

		// 2. Формируем полные пути для старого файла (исходный и целевой)
		oldFileFullPath := filepath.Join(vaultPath, result.TargetFileToMove)
		oldFileName := filepath.Base(oldFileFullPath)
		newOldFileLocation := filepath.Join(clusterFolder, oldFileName)

		// 3. Формируем полный путь для новой заметки
		newNoteFullPath := filepath.Join(clusterFolder, fileNameWithExt)

		// Выполняем физические операции через VFS (безопасные методы с Exponential Backoff)
		
		// Перемещаем старый файл в новую папку (папка создастся автоматически внутри SafeMove)
		if err := vfs.SafeMove(oldFileFullPath, newOldFileLocation); err != nil {
			return fmt.Errorf("failed to move old file to cluster: %w", err)
		}

		// Записываем новую заметку в эту же папку
		if err := vfs.AtomicWrite(newNoteFullPath, markdownData); err != nil {
			return fmt.Errorf("failed to write new note to cluster: %w", err)
		}

		return nil
	}

	// ЛОГИКА CREATE (Стандартное создание)
	
	// Формируем полный путь к новой заметке
	fullPath := filepath.Join(vaultPath, targetFolder, fileNameWithExt)

	// Записываем файл через VFS (папки по пути будут созданы автоматически)
	if err := vfs.AtomicWrite(fullPath, markdownData); err != nil {
		return fmt.Errorf("failed to write note: %w", err)
	}

	return nil
}