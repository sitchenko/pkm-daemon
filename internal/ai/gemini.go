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
Ты — системный архитектор базы знаний Obsidian. Ты работаешь СТРОГО по методу PARA.
Проанализируй текст и верни СТРОГО JSON.

СТРУКТУРА ХРАНИЛИЩА (PARA):
- Projects: Задачи с дедлайном.
- Areas: Зоны ответственности.
- Resources: Полезная информация.

АЛГОРИТМ МАРШРУТИЗАЦИИ:
1. Изучи список существующих путей: [%s].
2. ЕСЛИ подходящая папка уже есть -> "action": "create", "target_folder": используй ПУТЬ из списка.
3. ЕСЛИ подходящей папки НЕТ -> придумай новую категорию и запиши в "target_folder". 
4. КЛАСТЕРИЗАЦИЯ: Если заметка дополняет файл, ставь "action": "reorganize", укажи путь в "target_file_to_move", а в "cluster_name" — имя новой папки.
5. ИМЕНОВАНИЕ ПАПОК: Используй формат "ЭМОДЗИ Название" (например, '🎓 Дипломная работа'). Эмодзи должен соответствовать смыслу папки.

ПРАВИЛА ОФОРМЛЕНИЯ ЗАМЕТКИ:
6. "file_name": английский или транслит, без даты, snake_case.
7. "title": Человекочитаемый заголовок.
8. "tags": 1-3 тега. БЕЗ знака #.
9. "priority": High, Medium, Low.
10. "is_task": Если есть задачи, true. "tasks": массив строк.
11. "has_reminder" и "reminder_time": Если есть время, true и время.

ФОРМАТ ОТВЕТА (JSON):
{
  "action": "create",
  "target_folder": "01_Projects/🎓 Дипломная работа",
  "cluster_name": "",
  "target_file_to_move": "",
  "file_name": "diploma_check",
  "title": "Diploma work",
  "tags": ["образование/университет"],
  "is_task": true,
  "priority": "High",
  "has_reminder": true,
  "reminder_time": "2026-06-01T09:00:00Z",
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
		if err := json.Unmarshal(bodyBytes, &geminiResp); err != nil {
			return nil, err
		}

		if len(geminiResp.Candidates) == 0 || len(geminiResp.Candidates[0].Content.Parts) == 0 {
			return nil, fmt.Errorf("empty response")
		}

		rawText := strings.TrimSpace(geminiResp.Candidates[0].Content.Parts[0].Text)
		
		// НАДЕЖНОЕ ВЫРЕЗАНИЕ JSON (Игнорируем любой текст от ИИ до и после JSON)
		startIdx := strings.Index(rawText, "{")
		endIdx := strings.LastIndex(rawText, "}")
		if startIdx != -1 && endIdx != -1 && endIdx > startIdx {
			rawText = rawText[startIdx : endIdx+1]
		} else {
			return nil, fmt.Errorf("no json structure found in response: %s", rawText)
		}

		var result AnalysisResult
		if err := json.Unmarshal([]byte(rawText), &result); err != nil {
			return nil, fmt.Errorf("failed to parse JSON: %w", err)
		}

		return &result, nil
	}

	return nil, fmt.Errorf("ai failed after %d retries", maxRetries)
}