package ai

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"
)

const geminiAPIURL = "https://generativelanguage.googleapis.com/v1beta/models/gemini-2.5-flash:generateContent?key="

type Client struct {
	keys       []string
	mu         sync.Mutex
	keyIndex   int
	httpClient *http.Client
	logger     *slog.Logger
}

func NewClient(keys []string, logger *slog.Logger) *Client {
	return &Client{
		keys:       keys,
		keyIndex:   0,
		httpClient: &http.Client{Timeout: 30 * time.Second},
		logger:     logger,
	}
}

func (c *Client) getNextKey() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	key := c.keys[c.keyIndex]
	c.keyIndex = (c.keyIndex + 1) % len(c.keys)
	return key
}

func (c *Client) AnalyzeNote(content string, filesList []string) (*AnalysisResult, error) {
	filesStr := strings.Join(filesList, ", ")

	prompt := fmt.Sprintf(`СИСТЕМНАЯ ИНСТРУКЦИЯ:
Ты — системный архитектор базы знаний Obsidian. Проанализируй текст и верни СТРОГО JSON.

АЛГОРИТМ МАРШРУТИЗАЦИИ:
1. Изучи пути: [%s]. Если есть подходящая папка -> "action": "create", "target_folder": используй ПУТЬ из списка.
2. Иначе придумай новую категорию. ИМЕНОВАНИЕ ПАПОК: формат "ЭМОДЗИ Название" (напр. '🎓 Учеба').

ПРАВИЛА:
3. "file_name": английский, snake_case.
4. "title": Человекочитаемый заголовок.
5. "is_task": Если есть задачи, true. "tasks": массив строк.
6. "reminders": Если в тексте есть дедлайны/время, верни МАССИВ объектов (каждое отдельное время = отдельный объект).

ФОРМАТ ОТВЕТА (JSON):
{
  "action": "create",
  "target_folder": "01_Projects/🎓 Диплом",
  "file_name": "diploma_check",
  "title": "Diploma work",
  "tags": ["образование"],
  "is_task": true,
  "priority": "High",
  "reminders": [
    {"time": "2026-06-02T17:00:00Z", "text": "Взять ноутбук на тренировку"},
    {"time": "2026-06-02T18:30:00Z", "text": "Занести ноутбук бабушке"}
  ],
  "content": "Описание...",
  "tasks": ["Купить страховку", "Собрать снаряжение"]
}

ТЕКСТ ПОЛЬЗОВАТЕЛЯ:
%s`, filesStr, content)

	reqBody := geminiRequest{
		Contents: []geminiContent{{Parts: []geminiPart{{Text: prompt}}}},
	}
	payload, _ := json.Marshal(reqBody)

	maxRetries := len(c.keys)
	for attempt := 0; attempt < maxRetries; attempt++ {
		key := c.getNextKey()
		req, _ := http.NewRequest(http.MethodPost, geminiAPIURL+key, bytes.NewBuffer(payload))
		req.Header.Set("Content-Type", "application/json")

		resp, err := c.httpClient.Do(req)
		if err != nil {
			time.Sleep(1 * time.Second)
			continue
		}

		bodyBytes, _ := io.ReadAll(resp.Body)
		resp.Body.Close()

		if resp.StatusCode == http.StatusTooManyRequests {
			time.Sleep(2 * time.Second)
			continue
		}
		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("gemini error: %d %s", resp.StatusCode, string(bodyBytes))
		}

		var geminiResp geminiResponse
		if err := json.Unmarshal(bodyBytes, &geminiResp); err != nil { return nil, err }
		if len(geminiResp.Candidates) == 0 || len(geminiResp.Candidates[0].Content.Parts) == 0 {
			return nil, fmt.Errorf("empty response")
		}

		rawText := strings.TrimSpace(geminiResp.Candidates[0].Content.Parts[0].Text)
		startIdx := strings.Index(rawText, "{")
		endIdx := strings.LastIndex(rawText, "}")
		if startIdx != -1 && endIdx != -1 && endIdx > startIdx {
			rawText = rawText[startIdx : endIdx+1]
		} else {
			return nil, fmt.Errorf("no json structure found: %s", rawText)
		}

		var result AnalysisResult
		if err := json.Unmarshal([]byte(rawText), &result); err != nil { return nil, err }
		return &result, nil
	}
	return nil, fmt.Errorf("ai failed after %d retries", maxRetries)
}