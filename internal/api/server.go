package api

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"

	"gopkg.in/telebot.v3"

	"pkm-daemon/internal/storage"
	"pkm-daemon/internal/sync"
)

type Server struct {
	db        *storage.Storage
	bot       *telebot.Bot
	logger    *slog.Logger
	vaultPath string
}

func NewServer(db *storage.Storage, bot *telebot.Bot, vaultPath string, logger *slog.Logger) *Server {
	return &Server{
		db:        db,
		bot:       bot,
		logger:    logger,
		vaultPath: vaultPath,
	}
}

func (s *Server) Start(port string) error {
	mux := http.NewServeMux()

	// Serve static files
	webDir := filepath.Join("web")
	mux.Handle("/", http.FileServer(http.Dir(webDir)))

	// API Endpoints
	mux.HandleFunc("/api/tasks", s.handleGetTasks)
	mux.HandleFunc("/api/update_task", s.handleUpdateTask)
	mux.HandleFunc("/api/delete_task", s.handleDeleteTask)
	mux.HandleFunc("/api/note", s.handleGetNote)

	// Add CORS headers for local development testing
	handler := func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}
		mux.ServeHTTP(w, r)
	}

	s.logger.Info("Starting HTTP API Server", slog.String("port", port))
	return http.ListenAndServe(":"+port, http.HandlerFunc(handler))
}

func (s *Server) handleGetTasks(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Additionally, fetch 'Done' and 'Failed' tasks to show the full board? 
	// The Web App might want to see recently completed ones. 
	// For now, let's fetch all tasks to build the board correctly.
	// We'll modify db query to get all tasks, or at least active + recent.
	// Since GetActiveTasks() excludes Done and Failed, let's just get ALL for the Kanban.
	allTasks, err := s.getAllTasks()
	if err != nil {
		s.logger.Error("Failed to fetch tasks", slog.Any("error", err))
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(allTasks)
}

// Temporary helper until we add it to storage properly
func (s *Server) getAllTasks() ([]storage.TaskLedger, error) {
	// For Kanban we need everything, but let's limit to avoid massive payloads over time.
	// Ideally we filter by KanbanStatus or limit. Let's return all for now.
	return s.db.GetAllTasksForBoard() // We'll implement this in storage/repository.go
}

type UpdateTaskReq struct {
	TaskID string `json:"task_id"`
	Status string `json:"status"` // "Pending", "In Progress", "Done", "Failed"
	Reason string `json:"reason"`
}

func (s *Server) handleUpdateTask(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req UpdateTaskReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Bad request", http.StatusBadRequest)
		return
	}

	err := sync.ChangeTaskStatusAtomic(req.TaskID, req.Status, req.Reason, s.db, s.vaultPath)
	if err != nil {
		s.logger.Error("Failed to update task", slog.Any("error", err))
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
	fmt.Fprintf(w, `{"status":"ok"}`)
}

type DeleteTaskReq struct {
	TaskID string `json:"task_id"`
}

func (s *Server) handleDeleteTask(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req DeleteTaskReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Bad request", http.StatusBadRequest)
		return
	}

	err := sync.DeleteTaskAtomic(req.TaskID, s.db, s.vaultPath)
	if err != nil {
		s.logger.Error("Failed to delete task", slog.Any("error", err))
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
	fmt.Fprintf(w, `{"status":"ok"}`)
}

type GetNoteResp struct {
	Content string `json:"content"`
	Error   string `json:"error,omitempty"`
}

func (s *Server) handleGetNote(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	taskID := r.URL.Query().Get("task_id")
	if taskID == "" {
		http.Error(w, "task_id is required", http.StatusBadRequest)
		return
	}

	filePath, err := s.db.GetFilePathByTaskUUID(taskID)
	if err != nil || filePath == "" {
		s.logger.Error("Failed to get filepath for task", slog.String("task_id", taskID), slog.Any("error", err))
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(GetNoteResp{Error: "Note not found"})
		return
	}

	// Try to read the file
	data, err := os.ReadFile(filePath)
	if err != nil {
		// Try with vault path just in case
		data, err = os.ReadFile(filepath.Join(s.vaultPath, filePath))
		if err != nil {
			s.logger.Error("Failed to read note file", slog.String("path", filePath), slog.Any("error", err))
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(GetNoteResp{Error: "Failed to read note file"})
			return
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(GetNoteResp{Content: string(data)})
}
