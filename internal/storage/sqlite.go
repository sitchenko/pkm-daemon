package storage

import (
	"fmt"
	"log/slog"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// NewStorage инициализирует локальную SQLite базу данных с нужными оптимизациями.
func NewStorage(dbPath string, log *slog.Logger) (*Storage, error) {
	log.Info("Initializing SQLite database...", slog.String("path", dbPath))

	// Инициализация GORM с отключением стандартного шумного логгера (мы используем slog)
	db, err := gorm.Open(sqlite.Open(dbPath), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to connect to sqlite database: %w", err)
	}

	// Жесткая установка настроек для обеспечения параллельного доступа (Write-Ahead Logging)
	db.Exec("PRAGMA journal_mode=WAL;")
	db.Exec("PRAGMA synchronous=NORMAL;")

	// Автоматические миграции структуры
	err = db.AutoMigrate(
		&VaultIndex{},
		&TelegramMessage{},
		&TaskLedger{},
		&FSMSession{},
		&Reminder{},
	)
	if err != nil {
		return nil, fmt.Errorf("failed to run database migrations: %w", err)
	}

	log.Info("SQLite database initialized and migrated successfully")

	return &Storage{
		db:  db,
		log: log,
	}, nil
}