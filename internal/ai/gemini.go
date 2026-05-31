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

ПРАВИЛА КЛАСТЕРИЗАЦИИ:
1. Изучи список существующих файлов: [%s].
2. Если мысль пересекается с ОДНИМ файлом -> "action": "reorganize", "cluster_name": "объединяющее_имя".
3. Если подходит в СУЩЕСТВУЮЩИЙ кластер -> "action": "create", "target_folder": "имя_главной/имя_кластера".
4. Если пересечений нет -> "action": "create", "target_folder": "имя_категории".

ПРАВИЛА ОФОРМЛЕНИЯ ЗАМЕТКИ:
5. "file_name": английский или транслит, без даты, со змеиным_регистром (напр. 'diploma_work').
6. "title": Человекочитаемый заголовок (напр. 'Diploma work').
7. "tags": 1-3 тега в формате 'область/категория' (напр. 'образование/университет'). БЕЗ знака #.
8. "priority": Оцени срочность (High, Medium, Low). По умолчанию Medium.
9. "content": Основной описательный текст.
10. "is_task" и "tasks": Если в тексте есть план или просьба что-то сделать, ставь "is_task": true и выпиши конкретные шаги в массив строк "tasks" (БЕЗ маркдаун чек-боксов, просто текст). Иначе "is_task": false и [].

ФОРМАТ ОТВЕТА (JSON):
{
  "action": "create",
  "target_folder": "01_Projects",
  "cluster_name": "",
  "file_name": "diploma_work",
  "title": "Diploma work",
  "tags": ["образование/университет"],
  "is_task": true,
  "priority": "High",
  "content": "Дипломная работа: необходимо убедиться в академичности...",
  "tasks": ["Проверить подписи", "Добавить источники"]
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
		rawText = strings.TrimPrefix(rawText, "```json")
		rawText = strings.TrimPrefix(rawText, "```")
		rawText = strings.TrimSuffix(rawText, "```")
		rawText = strings.TrimSpace(rawText)

		var result AnalysisResult
		if err := json.Unmarshal([]byte(rawText), &result); err != nil {
			return nil, fmt.Errorf("failed to parse JSON: %w", err)
		}

		return &result, nil
	}

	return nil, fmt.Errorf("ai failed after %d retries", maxRetries)
}