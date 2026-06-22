package storage

import (
	"time"
)

// TelegramMessage хранит связь между сообщением в ТГ и файлом
type TelegramMessage struct {
	MessageID      int64 `gorm:"primaryKey"`
	ChatID         int64
	TelegramUserID int64
	FilePath       string
}

// VaultIndex хранит кэш информации о физических файлах
type VaultIndex struct {
	ID           uint   `gorm:"primaryKey"`
	FilePath     string `gorm:"uniqueIndex"`
	LastModified time.Time
	SizeBytes    int64
}

// TaskLedger хранит задачи для управления через Inline-кнопки
type TaskLedger struct {
	TaskUUID     string    `gorm:"primaryKey"`
	ParentID     string    `gorm:"index"` // НОВОЕ: Ссылка на родительскую заметку
	MessageID    int64
	KanbanStatus string    `gorm:"default:'pending'"`
	Content      string    `gorm:"type:text"`
	FilePath     string    `gorm:"type:text"` // НОВОЕ: Физический путь к .md файлу
	Deadline     time.Time
	CreatedAt    time.Time `gorm:"autoCreateTime"` // НОВОЕ: Время создания
}

// FSMSession хранит состояния конечного автомата для диалогов
type FSMSession struct {
	SessionID   string `gorm:"primaryKey"`
	UserID      int64  `gorm:"uniqueIndex"`
	State       string
	ContextData string
	ExpiresAt   time.Time
}

// Reminder хранит отложенные напоминания
type Reminder struct {
	ID              uint `gorm:"primaryKey"`
	TaskUUID        string
	TriggerTime     time.Time
	MessagePayload  string
	Status          string // pending, fired, escalated
	Acknowledged    bool   `gorm:"default:false"`
	EscalationLevel int    `gorm:"default:0"`
}