package storage

import (
	"log/slog"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type Repository interface {
	SaveMessage(msg *TelegramMessage) error
	FindFileByMessageID(messageID int64) (*VaultIndex, error)
	UpsertVaultIndex(index *VaultIndex) error
	
	SaveTask(task *TaskLedger) error
	UpdateTaskStatus(taskUUID string, newStatus string) error
	GetActiveTasks() ([]TaskLedger, error)
	GetTaskByID(taskUUID string) (*TaskLedger, error)
	GetFilePathByTaskUUID(taskUUID string) (string, error)
	GetTasksByParentID(parentID string) ([]TaskLedger, error)
	GetTelegramMessageByMessageID(messageID int64) (*TelegramMessage, error)
	UpdateTaskMessageID(taskUUID string, msgID int64) error
	GetTasksByMessageID(msgID int64) ([]TaskLedger, error) // НОВЫЙ МЕТОД

	SaveSession(session *FSMSession) error
	GetSession(userID int64) (*FSMSession, error)
	DeleteSession(userID int64) error

	CreateReminder(reminder *Reminder) error
	GetPendingReminders(currentTime time.Time) ([]Reminder, error)
	MarkReminderFired(reminderID uint) error
}

type Storage struct {
	db  *gorm.DB
	log *slog.Logger
}

func (s *Storage) AutoMigrate() error {
	s.log.Info("Running database migrations...")
	return s.db.AutoMigrate(&TelegramMessage{}, &VaultIndex{}, &FSMSession{}, &Reminder{}, &TaskLedger{})
}

func (s *Storage) UpsertVaultIndex(index *VaultIndex) error {
	return s.db.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "file_path"}}, DoUpdates: clause.AssignmentColumns([]string{"last_modified", "size_bytes"})}).Create(index).Error
}
func (s *Storage) SaveMessage(msg *TelegramMessage) error { return s.db.Save(msg).Error }
func (s *Storage) FindFileByMessageID(messageID int64) (*VaultIndex, error) {
	var msg TelegramMessage
	if err := s.db.First(&msg, "message_id = ?", messageID).Error; err != nil { return nil, err }
	var index VaultIndex
	if err := s.db.First(&index, "file_path = ?", msg.FilePath).Error; err != nil { return nil, err }
	return &index, nil
}
func (s *Storage) SaveTask(task *TaskLedger) error { return s.db.Save(task).Error }
func (s *Storage) UpdateTaskStatus(u string, st string) error { return s.db.Model(&TaskLedger{}).Where("task_uuid=?", u).Update("kanban_status", st).Error }
func (s *Storage) UpdateTaskMessageID(taskUUID string, msgID int64) error { return s.db.Model(&TaskLedger{}).Where("task_uuid=?", taskUUID).Update("message_id", msgID).Error }
func (s *Storage) GetActiveTasks() ([]TaskLedger, error) {
	var t []TaskLedger
	err := s.db.Where("kanban_status NOT IN ?", []string{"Archive", "Done", "Failed"}).Find(&t).Error
	return t, err
}
func (s *Storage) GetTaskByID(u string) (*TaskLedger, error) {
	var t TaskLedger
	err := s.db.First(&t, "task_uuid = ?", u).Error
	return &t, err
}
func (s *Storage) GetFilePathByTaskUUID(u string) (string, error) { 
	var t TaskLedger
	if err := s.db.First(&t, "task_uuid = ?", u).Error; err != nil { return "", err }
	if t.FilePath != "" { return t.FilePath, nil }
	var m TelegramMessage
	if err := s.db.First(&m, "message_id = ?", t.MessageID).Error; err != nil { return "", err }
	return m.FilePath, nil 
}
func (s *Storage) GetTasksByParentID(parentID string) ([]TaskLedger, error) {
	var tasks []TaskLedger
	err := s.db.Where("parent_id = ?", parentID).Find(&tasks).Error
	return tasks, err
}
func (s *Storage) GetTelegramMessageByMessageID(messageID int64) (*TelegramMessage, error) {
	var msg TelegramMessage
	err := s.db.First(&msg, "message_id = ?", messageID).Error
	return &msg, err
}
func (s *Storage) GetTasksByMessageID(msgID int64) ([]TaskLedger, error) {
	var tasks []TaskLedger
	err := s.db.Where("message_id = ?", msgID).Find(&tasks).Error
	return tasks, err
}
func (s *Storage) SaveSession(ss *FSMSession) error { return s.db.Save(ss).Error }
func (s *Storage) GetSession(uid int64) (*FSMSession, error) { 
	var ss FSMSession
	err := s.db.Where("user_id=?", uid).First(&ss).Error
	return &ss, err 
}
func (s *Storage) DeleteSession(uid int64) error { return s.db.Where("user_id=?", uid).Delete(&FSMSession{}).Error }
func (s *Storage) CreateReminder(reminder *Reminder) error { return s.db.Create(reminder).Error }
func (s *Storage) GetPendingReminders(currentTime time.Time) ([]Reminder, error) {
	var reminders []Reminder
	err := s.db.Where("trigger_time <= ? AND status = ?", currentTime, "pending").Find(&reminders).Error
	return reminders, err
}
func (s *Storage) MarkReminderFired(reminderID uint) error { return s.db.Model(&Reminder{}).Where("id = ?", reminderID).Update("status", "fired").Error }