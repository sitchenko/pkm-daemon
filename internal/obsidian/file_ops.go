package obsidian

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// AtomicWrite гарантирует безопасную перезапись файла
func AtomicWrite(targetPath string, data []byte) error {
	dir := filepath.Dir(targetPath)
	base := filepath.Base(targetPath)
	tempPath := filepath.Join(dir, base+".tmp")

	f, err := os.OpenFile(tempPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0644)
	if err != nil {
		return err
	}

	var renameDone bool
	var fileClosed bool

	closeFile := func() {
		if !fileClosed {
			_ = f.Close()
			fileClosed = true
		}
	}
	defer closeFile()

	defer func() {
		if !renameDone {
			_ = os.Remove(tempPath)
		}
	}()

	if _, err := f.Write(data); err != nil {
		return err
	}

	if err := f.Sync(); err != nil {
		return err
	}

	closeFile()

	if err := os.Rename(tempPath, targetPath); err != nil {
		return err
	}

	renameDone = true
	return nil
}

// UpdateTaskStatus отмечает задачу как выполненную или возвращает в To Do
func UpdateTaskStatus(filePath string, taskSubstring string, isDone bool) error {
	content, err := os.ReadFile(filePath)
	if err != nil {
		return err
	}

	lines := strings.Split(string(bytes.ReplaceAll(content, []byte("\r\n"), []byte("\n"))), "\n")
	updated := false

	for i, line := range lines {
		if strings.Contains(line, taskSubstring) {
			if isDone && strings.Contains(line, "- [ ]") {
				lines[i] = strings.Replace(line, "- [ ]", "- [x]", 1)
				updated = true
			} else if !isDone && (strings.Contains(line, "- [x]") || strings.Contains(line, "- [X]")) {
				// Обработка снятия галочки
				lines[i] = strings.Replace(line, "- [x]", "- [ ]", 1)
				lines[i] = strings.Replace(lines[i], "- [X]", "- [ ]", 1)
				updated = true
			}
		}
	}

	if !updated {
		return fmt.Errorf("task not found or already in state")
	}

	return AtomicWrite(filePath, []byte(strings.Join(lines, "\n")))
}

// FailTaskStatus зачеркивает задачу и дописывает причину с новой строки
func FailTaskStatus(filePath string, taskSubstring string, reason string) error {
	content, err := os.ReadFile(filePath)
	if err != nil {
		return err
	}

	lines := strings.Split(string(bytes.ReplaceAll(content, []byte("\r\n"), []byte("\n"))), "\n")
	updated := false

	for i, line := range lines {
		if strings.Contains(line, taskSubstring) && strings.Contains(line, "- [ ]") {
			// Зачеркиваем строку: ~~- [ ] Задача~~
			lines[i] = "~~" + line + "~~" + fmt.Sprintf("\nПровалено: %s", reason)
			updated = true
			break
		}
	}

	if !updated {
		return fmt.Errorf("active task not found to fail")
	}

	return AtomicWrite(filePath, []byte(strings.Join(lines, "\n")))
}