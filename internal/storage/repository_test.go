package storage

import (
	"log/slog"
	"os"
	"testing"
	"time"
)

func setupTestDB(t *testing.T) *Storage {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	db, err := NewStorage(":memory:", logger)
	if err != nil {
		t.Fatalf("Failed to setup in-memory test db: %v", err)
	}
	return db
}

func TestStorage_MessagesAndVaultIndex(t *testing.T) {
	db := setupTestDB(t)

	msgID := int64(42)
	filePath := "test_note.md"

	// 1. UpsertVaultIndex
	index := &VaultIndex{
		FilePath:     filePath,
		LastModified: time.Now(),
		SizeBytes:    1024,
	}
	if err := db.UpsertVaultIndex(index); err != nil {
		t.Fatalf("Failed to UpsertVaultIndex: %v", err)
	}

	// 2. SaveMessage
	msg := &TelegramMessage{
		MessageID:      msgID,
		ChatID:         123,
		TelegramUserID: 456,
		FilePath:       filePath,
	}
	if err := db.SaveMessage(msg); err != nil {
		t.Fatalf("Failed to SaveMessage: %v", err)
	}

	// 3. FindFileByMessageID
	foundIndex, err := db.FindFileByMessageID(msgID)
	if err != nil {
		t.Fatalf("Failed to FindFileByMessageID: %v", err)
	}
	if foundIndex.FilePath != filePath {
		t.Errorf("Expected file path %s, got %s", filePath, foundIndex.FilePath)
	}
	if foundIndex.SizeBytes != 1024 {
		t.Errorf("Expected SizeBytes 1024, got %d", foundIndex.SizeBytes)
	}
}

func TestStorage_Tasks(t *testing.T) {
	db := setupTestDB(t)

	taskUUID := "uuid-1234"
	task := &TaskLedger{
		TaskUUID:     taskUUID,
		KanbanStatus: "pending",
		Content:      "Buy milk",
		MessageID:    111,
	}

	// 1. SaveTask
	if err := db.SaveTask(task); err != nil {
		t.Fatalf("Failed to SaveTask: %v", err)
	}

	// 2. GetTaskByID
	foundTask, err := db.GetTaskByID(taskUUID)
	if err != nil {
		t.Fatalf("Failed to GetTaskByID: %v", err)
	}
	if foundTask.Content != "Buy milk" {
		t.Errorf("Expected content 'Buy milk', got '%s'", foundTask.Content)
	}

	// 3. UpdateTaskStatus
	if err := db.UpdateTaskStatus(taskUUID, "Done"); err != nil {
		t.Fatalf("Failed to UpdateTaskStatus: %v", err)
	}
	foundTask, _ = db.GetTaskByID(taskUUID)
	if foundTask.KanbanStatus != "Done" {
		t.Errorf("Expected status 'Done', got '%s'", foundTask.KanbanStatus)
	}

	// 4. UpdateTaskMessageID
	if err := db.UpdateTaskMessageID(taskUUID, 222); err != nil {
		t.Fatalf("Failed to UpdateTaskMessageID: %v", err)
	}
	foundTask, _ = db.GetTaskByID(taskUUID)
	if foundTask.MessageID != 222 {
		t.Errorf("Expected MessageID 222, got %d", foundTask.MessageID)
	}

	// 5. GetActiveTasks (should exclude 'Done')
	activeTasks, err := db.GetActiveTasks()
	if err != nil {
		t.Fatalf("Failed to GetActiveTasks: %v", err)
	}
	if len(activeTasks) != 0 {
		t.Errorf("Expected 0 active tasks (since it is Done), got %d", len(activeTasks))
	}

	// Add a pending task
	db.SaveTask(&TaskLedger{TaskUUID: "uuid-5678", KanbanStatus: "In Progress"})
	activeTasks, _ = db.GetActiveTasks()
	if len(activeTasks) != 1 {
		t.Errorf("Expected 1 active task, got %d", len(activeTasks))
	}

	// 6. GetAllTasksForBoard (should exclude 'Archive')
	db.UpdateTaskStatus("uuid-5678", "Archive")
	boardTasks, err := db.GetAllTasksForBoard()
	if err != nil {
		t.Fatalf("Failed to GetAllTasksForBoard: %v", err)
	}
	// UUID-1234 is Done (not Archive), so it should be returned
	if len(boardTasks) != 1 || boardTasks[0].TaskUUID != taskUUID {
		t.Errorf("Expected 1 task for board (Done), got %d", len(boardTasks))
	}

	// 7. DeleteTask
	if err := db.DeleteTask(taskUUID); err != nil {
		t.Fatalf("Failed to DeleteTask: %v", err)
	}
	_, err = db.GetTaskByID(taskUUID)
	if err == nil {
		t.Errorf("Expected error when fetching deleted task, got nil")
	}
}

func TestStorage_Reminders(t *testing.T) {
	db := setupTestDB(t)

	now := time.Now()
	rem := &Reminder{
		TaskUUID:       "task-1",
		TriggerTime:    now.Add(-1 * time.Minute), // In the past
		MessagePayload: "Wake up!",
		Status:         "pending",
	}

	// 1. CreateReminder
	if err := db.CreateReminder(rem); err != nil {
		t.Fatalf("Failed to CreateReminder: %v", err)
	}

	// 2. GetPendingReminders
	reminders, err := db.GetPendingReminders(now)
	if err != nil {
		t.Fatalf("Failed to GetPendingReminders: %v", err)
	}
	if len(reminders) != 1 {
		t.Fatalf("Expected 1 pending reminder, got %d", len(reminders))
	}

	remID := reminders[0].ID

	// 3. MarkReminderFired
	if err := db.MarkReminderFired(remID); err != nil {
		t.Fatalf("Failed to MarkReminderFired: %v", err)
	}
	reminders, _ = db.GetPendingReminders(now)
	if len(reminders) != 0 {
		t.Errorf("Expected 0 pending reminders after marking fired, got %d", len(reminders))
	}

	// 4. AcknowledgeReminder
	if err := db.AcknowledgeReminder(remID); err != nil {
		t.Fatalf("Failed to AcknowledgeReminder: %v", err)
	}
}

func TestStorage_DeleteNoteData(t *testing.T) {
	db := setupTestDB(t)

	msgID := int64(999)
	filePath := "to_be_deleted.md"

	db.SaveMessage(&TelegramMessage{MessageID: msgID, FilePath: filePath})
	db.SaveTask(&TaskLedger{TaskUUID: "task1", MessageID: msgID})
	db.SaveTask(&TaskLedger{TaskUUID: "task2", MessageID: msgID})

	deletedPath, err := db.DeleteNoteData(msgID)
	if err != nil {
		t.Fatalf("Failed to DeleteNoteData: %v", err)
	}
	if deletedPath != filePath {
		t.Errorf("Expected deleted path %s, got %s", filePath, deletedPath)
	}

	// Verify msg is deleted
	_, err = db.GetTelegramMessageByMessageID(msgID)
	if err == nil {
		t.Errorf("Expected message to be deleted, but it was found")
	}

	// Verify tasks are deleted
	tasks, _ := db.GetTasksByMessageID(msgID)
	if len(tasks) != 0 {
		t.Errorf("Expected tasks to be deleted, but found %d", len(tasks))
	}
}
