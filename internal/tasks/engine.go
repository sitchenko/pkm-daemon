package tasks

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"pkm-daemon/internal/storage"
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

func RegisterTask(content, noteName, noteFullPath, vaultPath, baseID, idSuffix string, db *storage.Storage, logger *slog.Logger) error {
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
	taskRegex := regexp.MustCompile(`(?i)- \[[ xX/]\] (?:<span[^>]*>)?\*\*Задача №[0-9-]+\*\*(?:</span>)?:\s*(.*?)(?:<br>|$)`)

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

	taskID := baseID + idSuffix

	// Extract dates from text to calculate priority
	dateRegex := regexp.MustCompile(`\b(\d{2}\.\d{2}\.\d{4}|\d{4}-\d{2}-\d{2})\b`)
	matches := dateRegex.FindAllStringSubmatch(content, -1)
	
	now := time.Now()
	minDaysDiff := 9999
	
	for _, match := range matches {
		dateStr := match[1]
		var parsedTime time.Time
		var err error
		if strings.Contains(dateStr, ".") {
			parsedTime, err = time.Parse("02.01.2006", dateStr)
		} else {
			parsedTime, err = time.Parse("2006-01-02", dateStr)
		}
		
		if err == nil {
			year1, month1, day1 := now.Date()
			today := time.Date(year1, month1, day1, 0, 0, 0, 0, now.Location())
			year2, month2, day2 := parsedTime.Date()
			target := time.Date(year2, month2, day2, 0, 0, 0, 0, now.Location())
			
			days := int(target.Sub(today).Hours() / 24)
			if days >= 0 && days < minDaysDiff {
				minDaysDiff = days
			}
		}
	}

	priority := 0
	lowerContent := strings.ToLower(content)
	if strings.Contains(lowerContent, "срочн") || strings.Contains(lowerContent, "важн") || strings.Contains(lowerContent, "priority") || strings.Contains(lowerContent, "!") {
		priority = 2
	} else if strings.Contains(lowerContent, "средне") || strings.Contains(lowerContent, "medium") {
		priority = 1
	}

	// Smart priority assignment based on dates
	if minDaysDiff <= 1 {
		priority = 2 // Today or tomorrow
	} else if minDaysDiff <= 5 && priority < 1 {
		priority = 1 // Within 5 days
	}

	taskLabel := fmt.Sprintf("**Задача №%s**", taskID)
	if priority == 2 {
		taskLabel = fmt.Sprintf(`<span style="color: #FF3B30">%s</span>`, taskLabel)
	} else if priority == 1 {
		taskLabel = fmt.Sprintf(`<span style="color: #FFCC00">%s</span>`, taskLabel)
	}

	tmNewLine := fmt.Sprintf("- [ ] %s: %s<br>📝 *[[%s]]*", taskLabel, content, noteName)

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
	kanbanNewLine := fmt.Sprintf("- [ ] %s: %s\n\t*[[%s]]*", taskLabel, content, noteName)

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

	// ==========================================
	// 5. Запись в локальную базу SQLite
	// ==========================================
	ledgerEntry := storage.TaskLedger{
		TaskUUID:     taskID,
		ParentID:     baseID,
		Content:      content,
		KanbanStatus: "pending",
		FilePath:     noteFullPath,
		Priority:     priority,
	}

	if err := db.SaveTask(&ledgerEntry); err != nil {
		logger.Error("Failed to save task to SQLite index", slog.String("id", taskID), slog.Any("error", err))
	} else {
		logger.Info("Задача успешно зарегистрирована в БД", slog.String("id", taskID), slog.String("parent_id", baseID))
	}

	return nil
}