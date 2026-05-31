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

const geminiAPIURL = "https://" + "generativelanguage.googleapis.com/v1beta/models/gemini-2.5-flash:generateContent?key="

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

func (c *Client) AnalyzeNote(content string, existingFolders []string, existingTags []string) (*AnalysisResult, error) {
	now := time.Now().Format(time.RFC3339) // Динамический контекст времени для ИИ
	
	prompt := fmt.Sprintf(`СИСТЕМНАЯ ИНСТРУКЦИЯ:
Ты — интеллектуальный арбитр системы управления знаниями (PKM).
Проанализируй заметку пользователя и верни результат СТРОГО И ИСКЛЮЧИТЕЛЬНО в формате JSON.
Не пиши приветствий, пояснений и прочего текста. Только валидный JSON-объект.

ФОРМАТ ОТВЕТА:
{
  "title": "краткий и емкий заголовок (до 5-6 слов)",
  "folder": "выбери одну подходящую папку из списка: [%s]. Если ничего не подходит, верни 'Inbox'",
  "tags": ["массив", "тегов", "включая подходящие из: %s"],
  "is_task": true/false (true, если в тексте есть дедлайн, напоминание или призыв к действию),
  "reminder_time": "строка в ISO8601 или пусто"
}

Текущее системное время: %s. Если пользователь просит напомнить о чем-то (например, 'напомни завтра вечером'), вычисли и верни точную дату и время в поле 'reminder_time' в формате ISO8601 (например, 2026-05-31T18:00:00Z). Если напоминания нет, верни пустую строку "".

ЗАМЕТКА ПОЛЬЗОВАТЕЛЯ:
%s`, 
		strings.Join(existingFolders, ", "), 
		strings.Join(existingTags, ", "), 
		now,
		content,
	)

	reqBody := geminiRequest{
		Contents: []geminiContent{
			{Parts: []geminiPart{{Text: prompt}}},
		},
	}

	payload, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	maxRetries := len(c.keys)
	for attempt := 0; attempt < maxRetries; attempt++ {
		key := c.getNextKey()
		maskedKey := key[:8] + "..." + key[len(key)-4:]
		
		c.logger.Debug("Sending request to Gemini", slog.String("key", maskedKey), slog.Int("attempt", attempt+1))

		req, err := http.NewRequest(http.MethodPost, geminiAPIURL+key, bytes.NewBuffer(payload))
		if err != nil {
			return nil, fmt.Errorf("failed to create http request: %w", err)
		}
		req.Header.Set("Content-Type", "application/json")

		resp, err := c.httpClient.Do(req)
		if err != nil {
			c.logger.Error("Network error during Gemini request", slog.Any("error", err))
			time.Sleep(1 * time.Second)
			continue
		}

		bodyBytes, _ := io.ReadAll(resp.Body)
		resp.Body.Close()

		if resp.StatusCode == http.StatusTooManyRequests {
			c.logger.Warn("Rate limit hit (429), rotating key and retrying...", slog.String("failed_key", maskedKey))
			time.Sleep(2 * time.Second)
			continue
		}

		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("gemini api error (status %d): %s", resp.StatusCode, string(bodyBytes))
		}

		var geminiResp geminiResponse
		if err := json.Unmarshal(bodyBytes, &geminiResp); err != nil {
			return nil, fmt.Errorf("failed to decode gemini response: %w", err)
		}

		if len(geminiResp.Candidates) == 0 || len(geminiResp.Candidates[0].Content.Parts) == 0 {
			return nil, fmt.Errorf("empty response from gemini")
		}

		rawText := geminiResp.Candidates[0].Content.Parts[0].Text

		rawText = strings.TrimSpace(rawText)
		if strings.HasPrefix(rawText, "```json") {
			rawText = strings.TrimPrefix(rawText, "```json")
			rawText = strings.TrimSuffix(rawText, "```")
		} else if strings.HasPrefix(rawText, "```") {
			rawText = strings.TrimPrefix(rawText, "```")
			rawText = strings.TrimSuffix(rawText, "```")
		}
		rawText = strings.TrimSpace(rawText)

		var result AnalysisResult
		if err := json.Unmarshal([]byte(rawText), &result); err != nil {
			return nil, fmt.Errorf("failed to parse JSON from llm: %w. Raw text: %s", err, rawText)
		}

		return &result, nil
	}

	return nil, fmt.Errorf("all %d api keys exhausted or rate limited", maxRetries)
}