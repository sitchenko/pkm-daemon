package sync

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"pkm-daemon/internal/storage"
	"pkm-daemon/internal/vfs"
)

var (
	reCheckbox           = regexp.MustCompile(`- \[[ xX/]\]`)
	reCheckboxWithReason = regexp.MustCompile(`- \[[ xX/]\]( ❌ Провалено: .*?| ✅ Выполнено: .*?)?`)
)

func ChangeTaskStatusAtomic(taskID string, newStatus string, reason string, db *storage.Storage, vaultPath string) error {
	task, err := db.GetTaskByID(taskID)
	if err != nil {
		return err
	}

	if task.KanbanStatus == newStatus && newStatus != "Failed" && newStatus != "Done" {
		return nil
	}

	db.UpdateTaskStatus(taskID, newStatus)

	// Build replacement string for physical files
	var replacement string
	var reasonStr string

	if newStatus == "Done" {
		replacement = "- [x]"
		if reason != "" {
			reasonStr = fmt.Sprintf("\t✅ Выполнено: %s", reason)
		}
	} else if newStatus == "Failed" {
		replacement = "- [x]"
		reasonStr = fmt.Sprintf("\t❌ Провалено: %s", reason)
	} else if newStatus == "In Progress" {
		replacement = "- [/]"
	} else {
		replacement = "- [ ]"
	}

	// 1. Update main Note file
	if task.FilePath != "" {
		replaceTaskLine(task.FilePath, task.Content, replacement, reasonStr)
	}

	// 2. Update Task_Manager.md
	tasksFolderPath := filepath.Join(vaultPath, "01_Задачи")
	tmPath := filepath.Join(tasksFolderPath, "Task_Manager.md")
	replaceTaskLineInManager(tmPath, taskID, replacement, reasonStr)

	// 3. Update Kanban.md
	kanbanPath := filepath.Join(tasksFolderPath, "🎯 Канбан.md")
	moveTaskInKanban(kanbanPath, taskID, newStatus, replacement, reasonStr)

	return nil
}

func DeleteTaskAtomic(taskID string, db *storage.Storage, vaultPath string) error {
	task, err := db.GetTaskByID(taskID)
	if err != nil {
		return err
	}

	err = db.DeleteTask(taskID)
	if err != nil {
		return err
	}

	if task.FilePath != "" {
		deleteTaskLine(task.FilePath, task.Content)
	}

	tasksFolderPath := filepath.Join(vaultPath, "01_Задачи")
	tmPath := filepath.Join(tasksFolderPath, "Task_Manager.md")
	deleteTaskLineByID(tmPath, taskID)

	kanbanPath := filepath.Join(tasksFolderPath, "🎯 Канбан.md")
	deleteTaskLineByID(kanbanPath, taskID)

	return nil
}

func insertOrReplaceReason(lines []string, index int, reasonStr string) []string {
	if reasonStr == "" {
		return lines
	}
	// Check if next line already has a reason
	if index+1 < len(lines) && (strings.HasPrefix(strings.TrimSpace(lines[index+1]), "✅ Выполнено:") || strings.HasPrefix(strings.TrimSpace(lines[index+1]), "❌ Провалено:")) {
		lines[index+1] = reasonStr
		return lines
	}
	// Insert new line
	var newLines []string
	newLines = append(newLines, lines[:index+1]...)
	newLines = append(newLines, reasonStr)
	newLines = append(newLines, lines[index+1:]...)
	return newLines
}

func replaceTaskLine(filePath, contentStr, replacement string, reasonStr string) error {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil
	}
	lines := strings.Split(string(data), "\n")
	changed := false
	for i, line := range lines {
		// Clean line matching
		dbContent := strings.TrimSpace(strings.TrimPrefix(strings.TrimPrefix(strings.TrimSpace(contentStr), "- [ ]"), "- [/]"))
		if strings.Contains(line, dbContent) && (strings.Contains(line, "- [ ]") || strings.Contains(line, "- [x]") || strings.Contains(line, "- [/]")) {
			lines[i] = reCheckbox.ReplaceAllString(line, replacement)
			lines = insertOrReplaceReason(lines, i, reasonStr)
			changed = true
			break
		}
	}
	if changed {
		return vfs.AtomicWrite(filePath, []byte(strings.Join(lines, "\n")))
	}
	return nil
}

func replaceTaskLineInManager(filePath, taskID, replacement string, reasonStr string) error {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil
	}
	lines := strings.Split(string(data), "\n")
	changed := false
	searchStr := "**Задача №" + taskID
	searchStr2 := "Задача №" + taskID
	for i, line := range lines {
		if strings.Contains(line, searchStr) || strings.Contains(line, searchStr2) {
			lines[i] = reCheckboxWithReason.ReplaceAllString(line, replacement)
			lines = insertOrReplaceReason(lines, i, reasonStr)
			changed = true
			break
		}
	}
	if changed {
		return vfs.AtomicWrite(filePath, []byte(strings.Join(lines, "\n")))
	}
	return nil
}

func moveTaskInKanban(kanbanPath, taskID, newStatus, replacement string, reasonStr string) error {
	data, err := os.ReadFile(kanbanPath)
	if err != nil {
		return nil
	}
	lines := strings.Split(string(data), "\n")
	var newLines []string
	var taskLines []string
	capturing := false
	searchStr := "Задача №" + taskID

	for _, line := range lines {
		cleanLine := strings.TrimRight(line, "\r")

		if strings.Contains(cleanLine, searchStr) {
			capturing = true
			updatedLine := reCheckboxWithReason.ReplaceAllString(cleanLine, replacement)
			taskLines = append(taskLines, updatedLine)
			if reasonStr != "" {
				taskLines = append(taskLines, reasonStr)
			}
			continue
		}

		if capturing {
			// Skip existing reason line so we don't duplicate it
			if strings.HasPrefix(strings.TrimSpace(cleanLine), "✅ Выполнено:") || strings.HasPrefix(strings.TrimSpace(cleanLine), "❌ Провалено:") {
				continue
			}

			if strings.HasPrefix(cleanLine, "\t") || strings.HasPrefix(cleanLine, "  ") {
				taskLines = append(taskLines, cleanLine)
				continue
			} else {
				capturing = false
			}
		}

		if !capturing {
			newLines = append(newLines, cleanLine)
		}
	}

	if len(taskLines) == 0 {
		return nil
	}

	var finalLines []string
	targetHeader := ""
	switch newStatus {
	case "Done":
		targetHeader = "## ✅ Готово"
	case "Failed":
		targetHeader = "## ❌ Провалено"
	case "In Progress":
		targetHeader = "## ⏳ В процессе" // Or whatever name
	default:
		targetHeader = "## 🎯 К выполнению"
	}

	inserted := false
	hasHeader := false

	for _, line := range newLines {
		finalLines = append(finalLines, line)
		if strings.HasPrefix(strings.TrimSpace(line), targetHeader) {
			hasHeader = true
			finalLines = append(finalLines, taskLines...)
			inserted = true
		}
	}

	if !hasHeader {
		finalLines = append(finalLines, "", targetHeader)
		finalLines = append(finalLines, taskLines...)
	} else if !inserted {
		finalLines = append(finalLines, taskLines...)
	}

	return vfs.AtomicWrite(kanbanPath, []byte(strings.Join(finalLines, "\n")))
}

func deleteTaskLine(filePath, contentStr string) error {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil
	}
	lines := strings.Split(string(data), "\n")
	var newLines []string
	changed := false
	for _, line := range lines {
		if strings.Contains(line, contentStr) && (strings.Contains(line, "- [ ]") || strings.Contains(line, "- [x]") || strings.Contains(line, "- [/]")) {
			changed = true
			continue // Skip this line
		}
		newLines = append(newLines, line)
	}
	if changed {
		return vfs.AtomicWrite(filePath, []byte(strings.Join(newLines, "\n")))
	}
	return nil
}

func deleteTaskLineByID(filePath, taskID string) error {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil
	}
	lines := strings.Split(string(data), "\n")
	var newLines []string
	capturing := false
	searchStr := "Задача №" + taskID
	changed := false

	for _, line := range lines {
		if strings.Contains(line, searchStr) {
			capturing = true
			changed = true
			continue
		}
		if capturing {
			if strings.HasPrefix(line, "\t") || strings.HasPrefix(line, "  ") {
				continue
			} else {
				capturing = false
			}
		}
		if !capturing {
			newLines = append(newLines, line)
		}
	}

	if changed {
		return vfs.AtomicWrite(filePath, []byte(strings.Join(newLines, "\n")))
	}
	return nil
}
