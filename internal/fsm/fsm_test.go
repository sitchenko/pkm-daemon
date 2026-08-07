package fsm

import (
	"log/slog"
	"os"
	"testing"
	"time"

	"pkm-daemon/internal/storage"
)

func setupTestDB(t *testing.T) *storage.Storage {
	// Initialize slog to discard logs in tests
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	// Use in-memory SQLite database for testing
	db, err := storage.NewStorage(":memory:", logger)
	if err != nil {
		t.Fatalf("Failed to setup in-memory test db: %v", err)
	}
	return db
}

func TestFSMManager_SetAndGet(t *testing.T) {
	db := setupTestDB(t)
	manager := NewManager(db)

	userID := int64(12345)
	state := "TEST_STATE"
	contextData := "some_data"

	// Test Set
	err := manager.Set(userID, state, contextData)
	if err != nil {
		t.Fatalf("Failed to Set state: %v", err)
	}

	// Test Get
	session, err := manager.Get(userID)
	if err != nil {
		t.Fatalf("Failed to Get state: %v", err)
	}

	if session == nil {
		t.Fatalf("Expected session to not be nil")
	}
	if session.UserID != userID {
		t.Errorf("Expected UserID %d, got %d", userID, session.UserID)
	}
	if session.State != state {
		t.Errorf("Expected State %s, got %s", state, session.State)
	}
	if session.ContextData != contextData {
		t.Errorf("Expected ContextData %s, got %s", contextData, session.ContextData)
	}

	// Verify ExpiresAt is set to future
	if session.ExpiresAt.Before(time.Now()) {
		t.Errorf("Expected ExpiresAt to be in the future")
	}
}

func TestFSMManager_Clear(t *testing.T) {
	db := setupTestDB(t)
	manager := NewManager(db)

	userID := int64(67890)

	// Set state
	err := manager.Set(userID, "STATE_TO_CLEAR", "")
	if err != nil {
		t.Fatalf("Failed to Set state: %v", err)
	}

	// Clear state
	err = manager.Clear(userID)
	if err != nil {
		t.Fatalf("Failed to Clear state: %v", err)
	}

	// Try to Get state, should not be found
	_, err = manager.Get(userID)
	if err == nil {
		t.Errorf("Expected an error (e.g. record not found) after clear, but got nil")
	}
}
