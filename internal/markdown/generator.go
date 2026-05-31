package markdown

import (
	"bytes"
	"fmt"
	"strings"
	"time"

	"pkm-daemon/internal/ai"
)

func GenerateNote(res ai.AnalysisResult) ([]byte, error) {
	var buf bytes.Buffer

	buf.WriteString("---\n")
	if len(res.Tags) > 0 {
		buf.WriteString("tags: [" + strings.Join(res.Tags, ", ") + "]\n")
	}
	buf.WriteString(fmt.Sprintf("date: %s\n", time.Now().Format("2006-01-02")))
	buf.WriteString("---\n\n")

	buf.WriteString(fmt.Sprintf("# %s\n\n", res.Title))

	// Умная дата и отсутствие лишнего заголовка
	if len(res.Reminders) > 0 {
		for _, rem := range res.Reminders {
			smartDate := formatSmartDate(rem.Time)
			buf.WriteString(fmt.Sprintf("> ⏰ **Напоминание установлено на:** %s\n> *%s*\n", smartDate, rem.Text))
		}
		buf.WriteString("\n")
	}

	buf.WriteString(res.Content + "\n\n")

	if res.IsTask && len(res.Tasks) > 0 {
		buf.WriteString("### 📝 Задачи\n")
		for _, task := range res.Tasks {
			buf.WriteString(fmt.Sprintf("- [ ] %s\n", task))
		}
		buf.WriteString("\n")
	}

	return buf.Bytes(), nil
}

// formatSmartDate превращает ISO 8601 в "завтра, 17:00", "02.06, пятница, 17:00"
func formatSmartDate(isoStr string) string {
	t, err := time.Parse(time.RFC3339, isoStr)
	if err != nil {
		return isoStr
	}

	now := time.Now()
	y1, m1, d1 := t.Date()
	y2, m2, d2 := now.Date()

	tDate := time.Date(y1, m1, d1, 0, 0, 0, 0, t.Location())
	nowDate := time.Date(y2, m2, d2, 0, 0, 0, 0, now.Location())

	daysDiff := int(tDate.Sub(nowDate).Hours() / 24)
	timeStr := t.Format("15:04")

	weekdays := map[time.Weekday]string{
		time.Monday: "понедельник", time.Tuesday: "вторник", time.Wednesday: "среда",
		time.Thursday: "четверг", time.Friday: "пятница", time.Saturday: "суббота", time.Sunday: "воскресенье",
	}

	switch daysDiff {
	case 0:
		return fmt.Sprintf("сегодня, %s", timeStr)
	case 1:
		return fmt.Sprintf("завтра, %s", timeStr)
	case 2:
		return fmt.Sprintf("послезавтра, %s", timeStr)
	default:
		return fmt.Sprintf("%02d.%02d, %s, %s", d1, m1, weekdays[t.Weekday()], timeStr)
	}
}