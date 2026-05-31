package tasks

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"pkm-daemon/internal/vfs"
)

const (
	TasksFolder     = "01_Задачи"
	TaskManagerFile = "Task_Manager.md"
	KanbanFile      = "🎯 Канбан.md"
)

func levenshtein(a, b string) int {
	s1, s2 := []rune(a), []rune(b)
	if len(s1) == 0 { return len(s2) }
	if len(s2) == 0 { return len(s1) }

	matrix := make([][]int, len(s1)+1)
	for i := range matrix {
		matrix[i] = make([]int, len(s2)+1)
		matrix[i][0] = i
	}
	for j := 0; j <= len(s2); j++ {
		matrix[0][j] = j
	}

	for i := 1; i <= len(s1); i++ {
		for j := 1; j <= len(s2); j++ {
			cost := 1
			if s1[i-1] == s2[j-1] { cost = 0 }
			replace := matrix[i-1][j-1] + cost
			insert := matrix[i][j-1] + 1
			delete := matrix[i-1][j] + 1

			min := replace
			if insert < min { min = insert }
			if delete < min { min = delete }
			matrix[i][j] = min
		}
	}
	return matrix[len(s1)][len(s2)]
}

func calculateSimilarity(a, b string) float64 {
	s1, s2 := []rune(a), []rune(b)
	maxLen := len(s1)
	if len(s2) > maxLen { maxLen = len(s2) }
	if maxLen == 0 { return 100.0 }
	dist := levenshtein(a, b)
	return (1.0 - float64(dist)/float64(maxLen)) * 100.0
}

// RegisterTask принимает idSuffix (например "-1", "-2"), чтобы ID задач, созданных в одну секунду, не пересекались.
func RegisterTask(content, noteName, vaultPath, idSuffix string, logger *slog.Logger) error {
	tasksDirPath := filepath.Join(vaultPath, TasksFolder)

	if err := os.MkdirAll(tasksDirPath, 0755); err != nil {
		return fmt.Errorf("failed to create tasks directory: %w", err)
	}

	tmPath := filepath.Join(tasksDirPath, TaskManagerFile)
	kanbanPath := filepath.Join(tasksDirPath, KanbanFile)

	tmData, err := os.ReadFile(tmPath)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to read Task_Manager.md: %w", err)
	}

	tmContentStr := string(tmData)
	taskRegex := regexp.MustCompile(`(?i)- \[[ xX]\] \*\*Задача №[0-9-]+\*\*:\s*(.*?)(?:<br>|$)`)

	lines := strings.Split(tmContentStr, "\n")
	for _, line := range lines {
		matches := taskRegex.FindStringSubmatch(line)
		if len(matches) > 1 {
			existingTask := strings.TrimSpace(matches[1])
			similarity := calculateSimilarity(strings.ToLower(content), strings.ToLower(existingTask))
			
			if similarity >= 80.0 {
				logger.Info("Задача пропущена как дубликат", 
					slog.String("task", content), 
					slog.Float64("similarity_percent", similarity))
				return nil
			}
		}
	}

	// Генерируем ID с суффиксом
	taskID := time.Now().Format("150405") + idSuffix

	tmNewLine := fmt.Sprintf("- [ ] **Задача №%s**: %s<br>📝 *[[%s]]*", taskID, content, noteName)

	if tmContentStr != "" && !strings.HasSuffix(tmContentStr, "\n") {
		tmContentStr += "\n"
	}
	if tmContentStr == "" {
		tmContentStr = "# Глобальный список задач\n\n"
	}
	tmContentStr += tmNewLine + "\n"

	if err := vfs.AtomicWrite(tmPath, []byte(tmContentStr)); err != nil {
		return fmt.Errorf("failed to write Task_Manager.md: %w", err)
	}

	kanbanData, err := os.ReadFile(kanbanPath)
	if err != nil {
		if os.IsNotExist(err) {
			kanbanData = []byte("---\nkanban-plugin: board\n---\n\n## 🎯 К выполнению\n")
		} else {
			return fmt.Errorf("failed to read 🎯 Канбан.md: %w", err)
		}
	}

	kanbanLines := strings.Split(string(kanbanData), "\n")
	kanbanNewLine := fmt.Sprintf("- [ ] **Задача №%s**: %s\n\t*[[%s]]*", taskID, content, noteName)

	var newKanbanLines []string
	inserted := false

	for _, line := range kanbanLines {
		cleanLine := strings.TrimRight(line, "\r")
		newKanbanLines = append(newKanbanLines, cleanLine)

		if !inserted && strings.HasPrefix(strings.TrimSpace(cleanLine), "## 🎯 К выполнению") {
			newKanbanLines = append(newKanbanLines, kanbanNewLine)
			inserted = true
		}
	}

	if !inserted {
		newKanbanLines = append(newKanbanLines, "", "## 🎯 К выполнению", kanbanNewLine)
	}

	kanbanContentStr := strings.Join(newKanbanLines, "\n")
	if err := vfs.AtomicWrite(kanbanPath, []byte(kanbanContentStr)); err != nil {
		return fmt.Errorf("failed to write 🎯 Канбан.md: %w", err)
	}

	logger.Info("Задача успешно зарегистрирована", slog.String("id", taskID))
	return nil
}