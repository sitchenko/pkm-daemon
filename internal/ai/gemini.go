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
Ты — системный архитектор базы знаний Obsidian. Ты работаешь СТРОГО по методу PARA (Тьяго Форте).
Проанализируй текст и верни СТРОГО JSON.

СТРУКТУРА ХРАНИЛИЩА (PARA):
- Projects: Задачи с дедлайном.
- Areas: Зоны ответственности (здоровье, финансы).
- Resources: Полезная информация.

АЛГОРИТМ МАРШРУТИЗАЦИИ:
1. Изучи список существующих путей: [%s].
2. Если в корневой папке УЖЕ ЕСТЬ подпапка с похожим смыслом -> "action": "create", "target_folder": ТОЧНЫЙ ПУТЬ из списка. ЗАПРЕЩЕНО создавать 'Дипломная/Дипломная'.
3. Если подходящей подпапки НЕТ -> придумай новую широкую категорию и запиши в "target_folder". 
4. Кластеризация ("reorganize"): Если заметка логически дополняет существующий ОДИНОЧНЫЙ файл, ставь "action": "reorganize", укажи путь в "target_file_to_move", а в "cluster_name" — имя новой объединяющей папки.

ПРАВИЛА ОФОРМЛЕНИЯ ЗАМЕТКИ (КРИТИЧЕСКИ ВАЖНО):
5. "file_name": английский, без даты, snake_case (напр. 'pills_schedule').
6. "title": Человекочитаемый заголовок (напр. 'Прием лекарств').
7. "tags": 1-3 тега (напр. 'здоровье/медикаменты'). БЕЗ знака #.
8. "is_task" и "tasks": Если есть список действий, ставь "is_task": true. КАЖДОЕ независимое действие ВЫДЕЛЯЙ В ОТДЕЛЬНЫЙ ЭЛЕМЕНТ массива "tasks" (напр. если надо выпить 4 разные таблетки, в массиве "tasks" должно быть 4 независимых строки).
9. "has_reminder" и "reminder_time": Если пользователь просит напомнить (или указывает время), ставь "has_reminder": true и извлеки время в "reminder_time".

ФОРМАТ ОТВЕТА (JSON):
{
  "action": "create",
  "target_folder": "02_Areas/Здоровье",
  "cluster_name": "",
  "target_file_to_move": "",
  "file_name": "take_pills",
  "title": "Прием лекарств",
  "tags": ["здоровье/медикаменты"],
  "is_task": true,
  "priority": "High",
  "has_reminder": true,
  "reminder_time": "сегодня в 20:00",
  "content": "Необходимо принять все назначенные препараты.",
  "tasks": ["Выпить капсулу", "Выпить коричневую таблетку", "Принять капли", "Выпить зеленую таблетку"]
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