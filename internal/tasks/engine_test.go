package tasks

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"pkm-daemon/internal/storage"
)

func TestLevenshtein(t *testing.T) {
	tests := []struct {
		a, b string
		want int
	}{
		{"", "", 0},
		{"a", "", 1},
		{"", "a", 1},
		{"abc", "abc", 0},
		{"kitten", "sitting", 3},
		{"flaw", "lawn", 2},
	}

	for _, tt := range tests {
		t.Run(tt.a+"_"+tt.b, func(t *testing.T) {
			if got := levenshtein(tt.a, tt.b); got != tt.want {
				t.Errorf("levenshtein(%q, %q) = %v, want %v", tt.a, tt.b, got, tt.want)
			}
		})
	}
}

func TestCalculateSimilarity(t *testing.T) {
	tests := []struct {
		a, b string
		want float64
	}{
		{"abc", "abc", 100.0},
		{"", "", 100.0},
		{"abc", "", 0.0},
		{"kitten", "sitting", 57.14285714285714}, // 1 - 3/7 = 4/7
	}

	for _, tt := range tests {
		t.Run(tt.a+"_"+tt.b, func(t *testing.T) {
			got := calculateSimilarity(tt.a, tt.b)
			if got < tt.want-0.001 || got > tt.want+0.001 { // float comparison
				t.Errorf("calculateSimilarity(%q, %q) = %v, want %v", tt.a, tt.b, got, tt.want)
			}
		})
	}
}

func setupTestDB(t *testing.T) (*storage.Storage, *slog.Logger) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	db, err := storage.NewStorage(":memory:", logger)
	if err != nil {
		t.Fatalf("Failed to setup in-memory test db: %v", err)
	}
	return db, logger
}

func TestRegisterTask(t *testing.T) {
	vaultDir := t.TempDir()
	db, logger := setupTestDB(t)

	content := "Buy some milk"
	noteName := "Shopping List"
	noteFullPath := filepath.Join(vaultDir, "Shopping List.md")
	baseID := "20231015120000"
	idSuffix := "-1"

	err := RegisterTask(content, noteName, noteFullPath, vaultDir, baseID, idSuffix, db, logger)
	if err != nil {
		t.Fatalf("RegisterTask failed: %v", err)
	}

	tasksDir := filepath.Join(vaultDir, TasksFolder)
	tmPath := filepath.Join(tasksDir, TaskManagerFile)
	kanbanPath := filepath.Join(tasksDir, KanbanFile)

	// Verify Task Manager file
	tmData, err := os.ReadFile(tmPath)
	if err != nil {
		t.Fatalf("Failed to read Task_Manager.md: %v", err)
	}
	if !strings.Contains(string(tmData), "Buy some milk") {
		t.Errorf("Task_Manager.md does not contain task content")
	}

	// Verify Kanban file
	kanbanData, err := os.ReadFile(kanbanPath)
	if err != nil {
		t.Fatalf("Failed to read Kanban file: %v", err)
	}
	if !strings.Contains(string(kanbanData), "Buy some milk") {
		t.Errorf("Kanban file does not contain task content")
	}

	// Verify Database entry
	taskID := baseID + idSuffix
	savedTask, err := db.GetTaskByID(taskID)
	if err != nil {
		t.Fatalf("Failed to get task from DB: %v", err)
	}
	if savedTask.Content != content {
		t.Errorf("Expected content %s, got %s", content, savedTask.Content)
	}
	if savedTask.ParentID != baseID {
		t.Errorf("Expected parentID %s, got %s", baseID, savedTask.ParentID)
	}
}

func TestRegisterTask_Duplicate(t *testing.T) {
	vaultDir := t.TempDir()
	db, logger := setupTestDB(t)

	content := "Unique task content"
	err := RegisterTask(content, "Note", "path", vaultDir, "1", "-1", db, logger)
	if err != nil {
		t.Fatalf("RegisterTask failed: %v", err)
	}

	// Try registering almost the same task again
	duplicateContent := "unique task content "
	err = RegisterTask(duplicateContent, "Note2", "path2", vaultDir, "2", "-2", db, logger)
	if err != nil {
		t.Fatalf("RegisterTask failed on duplicate: %v", err)
	}

	// Since it's a duplicate, it shouldn't be added to DB under the new ID
	_, err = db.GetTaskByID("2-2")
	if err == nil {
		t.Errorf("Duplicate task was added to DB unexpectedly")
	}
}

func TestRegisterTask_Priority(t *testing.T) {
	vaultDir := t.TempDir()
	db, logger := setupTestDB(t)

	tests := []struct {
		name     string
		content  string
		expected int
	}{
		{"Normal", "Just a task", 0},
		{"Medium Keyword", "This is medium urgency", 1},
		{"High Keyword", "This is срочно!", 2},
		{"Date Today", "Task due " + time.Now().Format("02.01.2006"), 2},
		{"Date Future", "Task due " + time.Now().AddDate(0, 0, 3).Format("2006-01-02"), 1},
	}

	for i, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			baseID := fmt.Sprintf("id-%d", i)
			err := RegisterTask(tt.content, "Note", "path", vaultDir, baseID, "-1", db, logger)
			if err != nil {
				t.Fatalf("RegisterTask failed: %v", err)
			}

			taskID := baseID + "-1"
			savedTask, err := db.GetTaskByID(taskID)
			if err != nil {
				t.Fatalf("Failed to get task from DB: %v", err)
			}
			if savedTask.Priority != tt.expected {
				t.Errorf("Expected priority %d, got %d", tt.expected, savedTask.Priority)
			}
		})
	}
}
