package sync

import (
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"

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

func TestInsertOrReplaceReason(t *testing.T) {
	lines := []string{"- [x] Task", "  Other info"}
	newLines := insertOrReplaceReason(lines, 0, "\t✅ Выполнено: reason")
	if len(newLines) != 3 {
		t.Fatalf("Expected 3 lines, got %d", len(newLines))
	}
	if !strings.Contains(newLines[1], "Выполнено:") {
		t.Errorf("Expected reason on line 1, got %s", newLines[1])
	}

	// Test replace
	lines = []string{"- [x] Task", "\t✅ Выполнено: old reason", "  Other info"}
	newLines = insertOrReplaceReason(lines, 0, "\t✅ Выполнено: new reason")
	if len(newLines) != 3 {
		t.Fatalf("Expected 3 lines, got %d", len(newLines))
	}
	if !strings.Contains(newLines[1], "new reason") {
		t.Errorf("Expected 'new reason', got %s", newLines[1])
	}
}

func TestChangeTaskStatusAtomic(t *testing.T) {
	db := setupTestDB(t)
	vaultPath := t.TempDir()

	tasksDir := filepath.Join(vaultPath, "01_Задачи")
	os.MkdirAll(tasksDir, 0755)

	tmPath := filepath.Join(tasksDir, "Task_Manager.md")
	kanbanPath := filepath.Join(tasksDir, "🎯 Канбан.md")
	notePath := filepath.Join(vaultPath, "note.md")

	os.WriteFile(tmPath, []byte("- [ ] **Задача №123**: do something\n"), 0644)
	os.WriteFile(kanbanPath, []byte("## 🎯 К выполнению\n- [ ] **Задача №123**: do something\n"), 0644)
	os.WriteFile(notePath, []byte("Some text\n- [ ] do something\n"), 0644)

	task := &storage.TaskLedger{
		TaskUUID:     "123",
		KanbanStatus: "pending",
		Content:      "do something",
		FilePath:     notePath,
	}
	db.SaveTask(task)

	err := ChangeTaskStatusAtomic("123", "Done", "because I did it", db, vaultPath)
	if err != nil {
		t.Fatalf("ChangeTaskStatusAtomic failed: %v", err)
	}

	updatedTask, _ := db.GetTaskByID("123")
	if updatedTask.KanbanStatus != "Done" {
		t.Errorf("DB status not updated")
	}

	tmData, _ := os.ReadFile(tmPath)
	if !strings.Contains(string(tmData), "- [x]") || !strings.Contains(string(tmData), "because I did it") {
		t.Errorf("TaskManager not updated properly: %s", string(tmData))
	}

	kanbanData, _ := os.ReadFile(kanbanPath)
	if !strings.Contains(string(kanbanData), "## ✅ Готово") || !strings.Contains(string(kanbanData), "- [x]") {
		t.Errorf("Kanban not updated properly: %s", string(kanbanData))
	}

	noteData, _ := os.ReadFile(notePath)
	if !strings.Contains(string(noteData), "- [x]") || !strings.Contains(string(noteData), "because I did it") {
		t.Errorf("Note file not updated properly: %s", string(noteData))
	}
}

func TestDeleteTaskAtomic(t *testing.T) {
	db := setupTestDB(t)
	vaultPath := t.TempDir()

	tasksDir := filepath.Join(vaultPath, "01_Задачи")
	os.MkdirAll(tasksDir, 0755)

	tmPath := filepath.Join(tasksDir, "Task_Manager.md")
	kanbanPath := filepath.Join(tasksDir, "🎯 Канбан.md")
	notePath := filepath.Join(vaultPath, "note.md")

	os.WriteFile(tmPath, []byte("- [ ] **Задача №123**: do something\n  Some extra info\n- [ ] Next task\n"), 0644)
	os.WriteFile(kanbanPath, []byte("## 🎯 К выполнению\n- [ ] **Задача №123**: do something\n\tLink\n- [ ] Next task\n"), 0644)
	os.WriteFile(notePath, []byte("Some text\n- [ ] do something\n- [ ] Another task\n"), 0644)

	task := &storage.TaskLedger{
		TaskUUID:     "123",
		Content:      "do something",
		FilePath:     notePath,
	}
	db.SaveTask(task)

	err := DeleteTaskAtomic("123", db, vaultPath)
	if err != nil {
		t.Fatalf("DeleteTaskAtomic failed: %v", err)
	}

	_, err = db.GetTaskByID("123")
	if err == nil {
		t.Errorf("Task still exists in DB")
	}

	tmData, _ := os.ReadFile(tmPath)
	if strings.Contains(string(tmData), "Задача №123") {
		t.Errorf("Task not removed from TaskManager: %s", string(tmData))
	}

	kanbanData, _ := os.ReadFile(kanbanPath)
	if strings.Contains(string(kanbanData), "Задача №123") {
		t.Errorf("Task not removed from Kanban: %s", string(kanbanData))
	}

	noteData, _ := os.ReadFile(notePath)
	if strings.Contains(string(noteData), "do something") {
		t.Errorf("Task not removed from Note: %s", string(noteData))
	}
	if !strings.Contains(string(noteData), "Another task") {
		t.Errorf("Deleted wrong task from Note")
	}
}
