package markdown

import (
	"bytes"
	"strings"
	"text/template"
	"time"

	"pkm-daemon/internal/ai"
)

// noteTemplate формирует идеальный Markdown согласно стандарту пользователя
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

{{.Content}}
{{if .IsTask}}
## Задачи
🔗 *Задачи синхронизированы в [[Task_Manager]]*
{{- range .Tasks}}
- [ ] {{.}}
{{- end}}
{{- end}}`

type templateData struct {
	Date     string
	Time     string
	Title    string
	Tags     []string
	IsTask   bool
	Priority string
	Content  string
	Tasks    []string
}

// GenerateNote компилирует данные из ИИ в готовый Markdown файл
func GenerateNote(result ai.AnalysisResult) ([]byte, error) {
	now := time.Now()

	data := templateData{
		Date:     now.Format("2006-01-02"),
		Time:     now.Format("15:04"),
		Title:    result.Title,
		Tags:     result.Tags,
		IsTask:   result.IsTask,
		Priority: result.Priority,
		Content:  result.Content,
		Tasks:    result.Tasks,
	}

	tmpl, err := template.New("note").Parse(noteTemplate)
	if err != nil {
		return nil, err
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return nil, err
	}

	// Очищаем лишние пустые строки в самом начале
	resultStr := strings.TrimLeft(buf.String(), "\n\r ")
	
	// Добавляем пустую строку в конец файла (Best Practice для Markdown)
	if !strings.HasSuffix(resultStr, "\n") {
		resultStr += "\n"
	}

	return []byte(resultStr), nil
}