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
	if newStatus == "Done" {
		if reason != "" {
			replacement = fmt.Sprintf("- [x] ✅ Выполнено: %s", reason)
		} else {
			replacement = "- [x]"
		}
	} else if newStatus == "Failed" {
		replacement = fmt.Sprintf("- [x] ❌ Провалено: %s", reason)
	} else if newStatus == "In Progress" {
		replacement = "- [/]"
	} else {
		replacement = "- [ ]"
	}

	// 1. Update main Note file
	if task.FilePath != "" {
		replaceTaskLine(task.FilePath, task.Content, replacement)
	}

	// 2. Update Task_Manager.md
	tasksFolderPath := filepath.Join(vaultPath, "01_Задачи")
	tmPath := filepath.Join(tasksFolderPath, "Task_Manager.md")
	replaceTaskLineInManager(tmPath, taskID, replacement)

	// 3. Update Kanban.md
	kanbanPath := filepath.Join(tasksFolderPath, "🎯 Канбан.md")
	moveTaskInKanban(kanbanPath, taskID, newStatus, replacement)

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

	// 1. In Note file
	if task.FilePath != "" {
		deleteTaskLine(task.FilePath, task.Content)
	}

	// 2. Task Manager
	tasksFolderPath := filepath.Join(vaultPath, "01_Задачи")
	tmPath := filepath.Join(tasksFolderPath, "Task_Manager.md")
	deleteTaskLineByID(tmPath, taskID)

	// 3. Kanban
	kanbanPath := filepath.Join(tasksFolderPath, "🎯 Канбан.md")
	deleteTaskLineByID(kanbanPath, taskID)

	return nil
}

// Helpers for generic replacements

func replaceTaskLine(filePath, contentStr, replacement string) error {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil
	}
	lines := strings.Split(string(data), "\n")
	changed := false
	for i, line := range lines {
		if strings.Contains(line, contentStr) && (strings.Contains(line, "- [ ]") || strings.Contains(line, "- [x]") || strings.Contains(line, "- [/]")) {
			// Find the checkbox part and replace it.
			if replacement == "- [x]" || replacement == "- [ ]" || replacement == "- [/]" {
				lines[i] = reCheckbox.ReplaceAllString(line, replacement)
			} else {
				// Has reason attached. Replace the whole task text or just append?
				// Better to append the reason if it's failed/done
				lines[i] = reCheckbox.ReplaceAllString(line, replacement)
			}
			changed = true
			break
		}
	}
	if changed {
		return vfs.AtomicWrite(filePath, []byte(strings.Join(lines, "\n")))
	}
	return nil
}

func replaceTaskLineInManager(filePath, taskID, replacement string) error {
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
			changed = true
			break
		}
	}
	if changed {
		return vfs.AtomicWrite(filePath, []byte(strings.Join(lines, "\n")))
	}
	return nil
}

func moveTaskInKanban(kanbanPath, taskID, newStatus, replacement string) error {
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
			continue
		}

		if capturing {
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
