package obsidian

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAtomicWrite(t *testing.T) {
	tempDir := t.TempDir()
	filePath := filepath.Join(tempDir, "test.md")
	content := []byte("hello world")

	if err := AtomicWrite(filePath, content); err != nil {
		t.Fatalf("AtomicWrite failed: %v", err)
	}

	data, err := os.ReadFile(filePath)
	if err != nil {
		t.Fatalf("Failed to read file: %v", err)
	}

	if string(data) != "hello world" {
		t.Errorf("Expected 'hello world', got '%s'", string(data))
	}
}

func TestUpdateTaskStatus(t *testing.T) {
	tempDir := t.TempDir()
	filePath := filepath.Join(tempDir, "tasks.md")
	os.WriteFile(filePath, []byte("Some text\n- [ ] My Task\n- [x] Done Task\n"), 0644)

	// Mark as done
	err := UpdateTaskStatus(filePath, "My Task", true)
	if err != nil {
		t.Fatalf("UpdateTaskStatus failed: %v", err)
	}

	data, _ := os.ReadFile(filePath)
	if !strings.Contains(string(data), "- [x] My Task") {
		t.Errorf("Task not marked as done: %s", string(data))
	}

	// Mark as not done
	err = UpdateTaskStatus(filePath, "Done Task", false)
	if err != nil {
		t.Fatalf("UpdateTaskStatus failed: %v", err)
	}

	data, _ = os.ReadFile(filePath)
	if !strings.Contains(string(data), "- [ ] Done Task") {
		t.Errorf("Task not unmarked: %s", string(data))
	}
}

func TestFailTaskStatus(t *testing.T) {
	tempDir := t.TempDir()
	filePath := filepath.Join(tempDir, "tasks.md")
	os.WriteFile(filePath, []byte("Some text\n- [ ] My Task\n"), 0644)

	err := FailTaskStatus(filePath, "My Task", "Not enough time")
	if err != nil {
		t.Fatalf("FailTaskStatus failed: %v", err)
	}

	data, _ := os.ReadFile(filePath)
	if !strings.Contains(string(data), "~~- [ ] My Task~~") {
		t.Errorf("Task not failed correctly: %s", string(data))
	}
	if !strings.Contains(string(data), "Провалено: Not enough time") {
		t.Errorf("Reason not added correctly: %s", string(data))
	}
}
