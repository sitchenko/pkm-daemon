package markdown

import (
	"bytes"
	"strings"
	"text/template"
	"time"

	"pkm-daemon/internal/ai"
)

// noteTemplate формирует идеальный Markdown
const noteTemplate = `---
date: {{.Date}}
time: {{.Time}}
{{- if .IsTask}}
status: К выполнению
priority: {{.Priority}}
{{- end}}
tags:
{{- range .Tags}}
  - {{.}}
{{- end}}
---

# {{.Title}}

{{- if .HasReminder}}

> ⏰ **Напоминание установлено на:** {{.ReminderTime}}
{{- end}}

{{.Content}}

{{- if .IsTask}}

## Задачи
🔗 *Задачи синхронизированы в [[Task_Manager]]*
{{- range .Tasks}}
- [ ] {{.}}
{{- end}}
{{- end}}`

type templateData struct {
	Date         string
	Time         string
	Title        string
	Tags         []string
	IsTask       bool
	Priority     string
	HasReminder  bool
	ReminderTime string
	Content      string
	Tasks        []string
}

func GenerateNote(result ai.AnalysisResult) ([]byte, error) {
	now := time.Now()

	data := templateData{
		Date:         now.Format("2006-01-02"),
		Time:         now.Format("15:04"),
		Title:        result.Title,
		Tags:         result.Tags,
		IsTask:       result.IsTask,
		Priority:     result.Priority,
		HasReminder:  result.HasReminder,
		ReminderTime: result.ReminderTime,
		Content:      result.Content,
		Tasks:        result.Tasks,
	}

	tmpl, err := template.New("note").Parse(noteTemplate)
	if err != nil {
		return nil, err
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return nil, err
	}

	resultStr := strings.TrimLeft(buf.String(), "\n\r ")
	if !strings.HasSuffix(resultStr, "\n") {
		resultStr += "\n"
	}

	return []byte(resultStr), nil
}