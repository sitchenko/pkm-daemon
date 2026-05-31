package fsm

import (
	"fmt"
	"time"

	"pkm-daemon/internal/storage"
)

const (
	StateWaitReason = "WAIT_REASON"
)

// Manager управляет состояниями диалога
type Manager struct {
	db *storage.Storage
}

func NewManager(db *storage.Storage) *Manager {
	return &Manager{db: db}
}

// Set сохраняет или обновляет состояние пользователя
func (m *Manager) Set(userID int64, state string, contextData string) error {
	session := &storage.FSMSession{
		SessionID:   fmt.Sprintf("usr_%d", userID),
		UserID:      userID,
		State:       state,
		ContextData: contextData,
		ExpiresAt:   time.Now().Add(24 * time.Hour), // Состояние активно 24 часа
	}
	return m.db.SaveSession(session)
}

// Get возвращает текущее состояние (если есть)
func (m *Manager) Get(userID int64) (*storage.FSMSession, error) {
	return m.db.GetSession(userID)
}

// Clear удаляет активное состояние
func (m *Manager) Clear(userID int64) error {
	return m.db.DeleteSession(userID)
}