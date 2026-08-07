package markdown

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"pkm-daemon/internal/ai"
)

func TestFormatSmartDate(t *testing.T) {
	now := time.Now()

	// Today
	today := now.Format(time.RFC3339)
	if !strings.HasPrefix(formatSmartDate(today), "сегодня,") {
		t.Errorf("Expected 'сегодня', got %s", formatSmartDate(today))
	}

	// Tomorrow
	tomorrow := now.AddDate(0, 0, 1).Format(time.RFC3339)
	if !strings.HasPrefix(formatSmartDate(tomorrow), "завтра,") {
		t.Errorf("Expected 'завтра', got %s", formatSmartDate(tomorrow))
	}

	// Day after tomorrow
	dayAfterTomorrow := now.AddDate(0, 0, 2).Format(time.RFC3339)
	if !strings.HasPrefix(formatSmartDate(dayAfterTomorrow), "послезавтра,") {
		t.Errorf("Expected 'послезавтра', got %s", formatSmartDate(dayAfterTomorrow))
	}

	// Other day
	otherDay := now.AddDate(0, 0, 5)
	otherDayStr := otherDay.Format(time.RFC3339)
	formatted := formatSmartDate(otherDayStr)
	expectedPrefix := fmt.Sprintf("%02d.%02d,", otherDay.Day(), otherDay.Month())
	if !strings.HasPrefix(formatted, expectedPrefix) {
		t.Errorf("Expected prefix %s, got %s", expectedPrefix, formatted)
	}

	// Invalid date
	if formatSmartDate("invalid") != "invalid" {
		t.Errorf("Expected 'invalid', got %s", formatSmartDate("invalid"))
	}
}

func TestGenerateNote(t *testing.T) {
	res := ai.AnalysisResult{
		Title:   "Test Title",
		Tags:    []string{"tag1", "tag2"},
		Content: "Test content line.",
		IsTask:  true,
		Tasks:   []string{"Task 1", "Task 2"},
		Reminders: []ai.ReminderInfo{
			{
				Time: time.Now().Format(time.RFC3339),
				Text: "Reminder text",
			},
		},
	}

	data, err := GenerateNote(res)
	if err != nil {
		t.Fatalf("GenerateNote returned error: %v", err)
	}

	noteStr := string(data)

	// Check tags
	if !strings.Contains(noteStr, "tags: [tag1, tag2]") {
		t.Errorf("Expected tags in output, got:\n%s", noteStr)
	}

	// Check date
	todayDate := time.Now().Format("2006-01-02")
	if !strings.Contains(noteStr, fmt.Sprintf("date: %s", todayDate)) {
		t.Errorf("Expected date in output, got:\n%s", noteStr)
	}

	// Check title
	if !strings.Contains(noteStr, "# Test Title") {
		t.Errorf("Expected title in output, got:\n%s", noteStr)
	}

	// Check reminder
	if !strings.Contains(noteStr, "Напоминание установлено на:") {
		t.Errorf("Expected reminder in output, got:\n%s", noteStr)
	}
	if !strings.Contains(noteStr, "Reminder text") {
		t.Errorf("Expected reminder text in output, got:\n%s", noteStr)
	}

	// Check content
	if !strings.Contains(noteStr, "Test content line.") {
		t.Errorf("Expected content in output, got:\n%s", noteStr)
	}

	// Check tasks
	if !strings.Contains(noteStr, "### 📝 Задачи") {
		t.Errorf("Expected tasks section in output, got:\n%s", noteStr)
	}
	if !strings.Contains(noteStr, "- [ ] Task 1") {
		t.Errorf("Expected Task 1 in output, got:\n%s", noteStr)
	}
}
