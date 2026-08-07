package api

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"gopkg.in/telebot.v3"

	"pkm-daemon/internal/storage"
)

func setupTestServer(t *testing.T) (*Server, *storage.Storage) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	db, err := storage.NewStorage(":memory:", logger)
	if err != nil {
		t.Fatalf("Failed to setup in-memory test db: %v", err)
	}
	bot, _ := telebot.NewBot(telebot.Settings{Offline: true})
	server := NewServer(db, bot, t.TempDir(), logger)
	return server, db
}

func TestHandleGetTasks(t *testing.T) {
	server, db := setupTestServer(t)

	db.SaveTask(&storage.TaskLedger{TaskUUID: "t1", Content: "Task 1", KanbanStatus: "pending"})
	db.SaveTask(&storage.TaskLedger{TaskUUID: "t2", Content: "Task 2", KanbanStatus: "In Progress"})
	
	req, err := http.NewRequest(http.MethodGet, "/api/tasks", nil)
	if err != nil {
		t.Fatal(err)
	}

	rr := httptest.NewRecorder()
	handler := http.HandlerFunc(server.handleGetTasks)
	handler.ServeHTTP(rr, req)

	if status := rr.Code; status != http.StatusOK {
		t.Errorf("handler returned wrong status code: got %v want %v", status, http.StatusOK)
	}

	var tasks []storage.TaskLedger
	if err := json.NewDecoder(rr.Body).Decode(&tasks); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if len(tasks) != 2 {
		t.Errorf("Expected 2 tasks, got %d", len(tasks))
	}
}

func TestHandleUpdateTask(t *testing.T) {
	server, db := setupTestServer(t)

	db.SaveTask(&storage.TaskLedger{TaskUUID: "123", Content: "Task", KanbanStatus: "pending"})

	reqBody := strings.NewReader(`{"task_id":"123","status":"Done","reason":"done it"}`)
	req, err := http.NewRequest(http.MethodPost, "/api/update_task", reqBody)
	if err != nil {
		t.Fatal(err)
	}

	rr := httptest.NewRecorder()
	handler := http.HandlerFunc(server.handleUpdateTask)
	handler.ServeHTTP(rr, req)

	if status := rr.Code; status != http.StatusOK {
		t.Errorf("handler returned wrong status code: got %v want %v", status, http.StatusOK)
	}

	updatedTask, _ := db.GetTaskByID("123")
	if updatedTask.KanbanStatus != "Done" {
		t.Errorf("Task status not updated in DB. Got %s", updatedTask.KanbanStatus)
	}
}

func TestHandleDeleteTask(t *testing.T) {
	server, db := setupTestServer(t)

	db.SaveTask(&storage.TaskLedger{TaskUUID: "123", Content: "Task"})

	reqBody := strings.NewReader(`{"task_id":"123"}`)
	req, err := http.NewRequest(http.MethodPost, "/api/delete_task", reqBody)
	if err != nil {
		t.Fatal(err)
	}

	rr := httptest.NewRecorder()
	handler := http.HandlerFunc(server.handleDeleteTask)
	handler.ServeHTTP(rr, req)

	if status := rr.Code; status != http.StatusOK {
		t.Errorf("handler returned wrong status code: got %v want %v", status, http.StatusOK)
	}

	_, err = db.GetTaskByID("123")
	if err == nil {
		t.Errorf("Expected task to be deleted, but it still exists")
	}
}
