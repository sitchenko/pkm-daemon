package telegram

import (
	"log/slog"
	"os"
	"testing"

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

func TestNewBot(t *testing.T) {
	db := setupTestDB(t)
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	aiClient := ai.NewClient([]string{"dummy"}, logger)

	cfg := Config{
		Token:        "dummy", // will fail to init without Offline, but we test the structure
		AdminID:      123,
		GuestID:      456,
		ObsidianPath: t.TempDir(),
	}

	// We can't actually start the bot without a real token unless we use telebot.Settings{Offline: true}
	// But NewBot sets Offline to false implicitly. 
	// To avoid actual network calls in NewBot, let's just make sure it returns an error or we catch it.
	bot, err := NewBot(cfg, aiClient, db, logger)
	if err == nil {
		t.Errorf("Expected error due to invalid token with network, but got nil")
	}
	
	if bot != nil {
		t.Errorf("Expected nil bot due to init failure, got bot")
	}
}

// Since telebot heavily relies on network, we'll keep the bot test minimal. 
// A full test would require refactoring NewBot to accept telebot.Settings or an interface.
