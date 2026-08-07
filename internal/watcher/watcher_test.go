package watcher

import (
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"

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

func TestWatcher_HandleFileChange_Kanban(t *testing.T) {
	db := setupTestDB(t)
	vaultPath := t.TempDir()
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	tasksDir := filepath.Join(vaultPath, "01_Задачи")
	os.MkdirAll(tasksDir, 0755)
	
	kanbanPath := filepath.Join(tasksDir, "🎯 Канбан.md")

	w, err := NewWatcher(vaultPath, db, logger)
	if err != nil {
		t.Fatalf("Failed to create watcher: %v", err)
	}
	defer w.Close()

	// Seed task
	task := &storage.TaskLedger{
		TaskUUID:     "123",
		KanbanStatus: "pending",
		Content:      "Buy milk",
	}
	db.SaveTask(task)

	// User moves task to In Progress in Kanban
	kanbanContent := "## ⏳ В процессе\n- [ ] Задача №123: Buy milk\n"
	os.WriteFile(kanbanPath, []byte(kanbanContent), 0644)

	// Trigger handleFileChange manually (normally triggered by fsnotify)
	w.handleFileChange(kanbanPath)

	// Status should now be 'In Progress'
	updatedTask, _ := db.GetTaskByID("123")
	if updatedTask.KanbanStatus != "In Progress" {
		t.Errorf("Expected status 'In Progress', got '%s'", updatedTask.KanbanStatus)
	}

	// Move to Done
	kanbanContent = "## ✅ Готово\n- [x] Задача №123: Buy milk\n"
	os.WriteFile(kanbanPath, []byte(kanbanContent), 0644)
	w.handleFileChange(kanbanPath)

	updatedTask, _ = db.GetTaskByID("123")
	if updatedTask.KanbanStatus != "Done" {
		t.Errorf("Expected status 'Done', got '%s'", updatedTask.KanbanStatus)
	}
}

func TestWatcher_HandleFileChange_Note(t *testing.T) {
	db := setupTestDB(t)
	vaultPath := t.TempDir()
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	notePath := filepath.Join(vaultPath, "test_note.md")

	w, err := NewWatcher(vaultPath, db, logger)
	if err != nil {
		t.Fatalf("Failed to create watcher: %v", err)
	}
	defer w.Close()

	// Seed task
	task := &storage.TaskLedger{
		TaskUUID:     "456",
		KanbanStatus: "pending",
		Content:      "Clean room",
		FilePath:     notePath,
	}
	db.SaveTask(task)

	// User marks task as Done in the note
	noteContent := "Some text\n- [x] Clean room\n"
	os.WriteFile(notePath, []byte(noteContent), 0644)

	// Allow some time for file system to settle (though handleFileChange reads immediately here)
	time.Sleep(50 * time.Millisecond)
	
	w.handleFileChange(notePath)

	updatedTask, _ := db.GetTaskByID("456")
	if updatedTask.KanbanStatus != "Done" {
		t.Errorf("Expected status 'Done', got '%s'", updatedTask.KanbanStatus)
	}
}
