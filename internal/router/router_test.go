package router

import (
	"log/slog"
	"os"
	"testing"
	"time"

	"pkm-daemon/internal/ai"
	"pkm-daemon/internal/storage"
)

func setupTestDB(t *testing.T) *storage.Storage {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	db, err := storage.NewStorage(":memory:", logger)
	if err != nil {
		t.Fatalf("Failed to setup in-memory test db: %v", err)
	}
	return db
}

func TestRouteAndSave(t *testing.T) {
	db := setupTestDB(t)
	vaultPath := t.TempDir()

	aiResult := &ai.AnalysisResult{
		TargetFolder: "02_Ресурсы",
		FileName:     "test_note",
		IsTask:       true,
		Tasks:        []string{"New Task"},
		Reminders: []ai.ReminderInfo{
			{Time: time.Now().Format(time.RFC3339), Text: "Reminder"},
		},
	}

	content := []byte("Some content here")

	baseID, fullPath, err := RouteAndSave(aiResult, content, vaultPath, db)
	if err != nil {
		t.Fatalf("RouteAndSave failed: %v", err)
	}

	if baseID == "" {
		t.Errorf("Expected non-empty baseID")
	}

	// Verify file was written
	data, err := os.ReadFile(fullPath)
	if err != nil {
		t.Fatalf("Failed to read created file: %v", err)
	}
	if string(data) != "Some content here" {
		t.Errorf("File content mismatch")
	}

	// Verify Task was registered
	tasks, err := db.GetActiveTasks()
	if err != nil {
		t.Fatalf("Failed to get tasks: %v", err)
	}
	if len(tasks) != 1 {
		t.Errorf("Expected 1 task, got %d", len(tasks))
	} else if tasks[0].Content != "New Task" {
		t.Errorf("Expected task content 'New Task', got '%s'", tasks[0].Content)
	}

	// Verify Reminder was registered
	now := time.Now()
	reminders, err := db.GetPendingReminders(now.Add(1 * time.Second))
	if err != nil {
		t.Fatalf("Failed to get reminders: %v", err)
	}
	if len(reminders) != 1 {
		t.Errorf("Expected 1 reminder, got %d", len(reminders))
	} else if reminders[0].MessagePayload != "Reminder" {
		t.Errorf("Expected reminder 'Reminder', got '%s'", reminders[0].MessagePayload)
	}
}
