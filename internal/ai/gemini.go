package ai

import (
	"bytes"
	"encoding/base64"
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

func (c *Client) AnalyzeNote(content string, filesList []string, mediaBytes []byte, mimeType string, onRetry func(int, int, error)) (*AnalysisResult, error) {
	filesStr := strings.Join(filesList, ", ")

	currentTime := time.Now().Format(time.RFC3339)
	prompt := fmt.Sprintf(`СИСТЕМНАЯ ИНСТРУКЦИЯ:
Ты — системный архитектор базы знаний Obsidian. Твоя задача — классифицировать заметку по методологии PARA. Верни СТРОГО JSON.

ИНСТРУКЦИЯ ПО МЕДИА И ССЫЛКАМ:
1. Если тебе передано аудио, музыка или видеосообщение (кружок), выполни его транскрибацию. На основе транскрибированного текста сформируй суть заметки.
2. Обязательно сохрани ВСЕ Markdown-ссылки на локальные файлы (например, ![[Photo_...]] или ![[VideoNote_...]]) из текста пользователя и включи их в поле "content".
3. Если пользователь прислал ВЕБ-ССЫЛКУ (URL, например http...), ОБЯЗАТЕЛЬНО сохрани её в итоговом тексте "content", не потеряй её.

АЛГОРИТМ МАРШРУТИЗАЦИИ:
1. Изучи список существующих путей: [%s].
2. ЕСЛИ суть заметки подходит под одну из СУЩЕСТВУЮЩИХ папок -> "action": "create", "target_folder": используй ПУТЬ из списка.
3. ЕСЛИ подходящей папки НЕТ, ты можешь придумать новую категорию, НО соблюдай строгое правило:
   - ТАБУ: НИКОГДА не создавай новые корневые папки (например, "06_Новая", "Заметки" или просто папки с эмодзи в корне).
   - Разрешено создавать только ПОДПАПКИ внутри существующих главных разделов из переданного списка (например, внутри "02_Ресурсы" создай "02_Ресурсы/🐶 Животные"). ИМЕНОВАНИЕ НОВЫХ ПОДПАПОК: формат "ЭМОДЗИ Название".

ПРАВИЛА (КРИТИЧЕСКИ ВАЖНО):
4. "file_name": ОБЯЗАТЕЛЬНО переведи на АНГЛИЙСКИЙ язык, в нижнем регистре (snake_case). Пример: beagle_care. Дату в это поле НЕ пиши.
5. "title": Человекочитаемый заголовок.
6. "is_task": Если есть задачи, true. "tasks": массив строк. Если задача срочная, используй теги #срочно или #важно. Укажи дату дедлайна в тексте задачи в формате DD.MM.YYYY, если она есть.
7. "reminders": Если в тексте есть дедлайны/время, верни МАССИВ объектов ({"time": "2026-06-02T17:00:00Z", "text": "Текст"}).

ТЕКУЩЕЕ ВРЕМЯ (ОБЯЗАТЕЛЬНО используй для расчёта дедлайнов и времени напоминаний):
%s

ФОРМАТ ОТВЕТА (JSON):
{
  "action": "create",
  "target_folder": "02_Ресурсы/🐶 Собаки",
  "file_name": "beagle_care",
  "title": "Уход за биглем",
  "tags": ["животные"],
  "is_task": false,
  "priority": "Low",
  "reminders": [],
  "content": "![[VideoNote_20260601_150405_123.mp4]]\n\nИнтересная ссылка: https://...\n\nТранскрипция/Суть...",
  "tasks": []
}

ТЕКСТ ПОЛЬЗОВАТЕЛЯ:
%s`, filesStr, currentTime, content)

	parts := []geminiPart{}

	if len(mediaBytes) > 0 && mimeType != "" {
		parts = append(parts, geminiPart{
			InlineData: &geminiInlineData{
				MimeType: mimeType,
				Data:     base64.StdEncoding.EncodeToString(mediaBytes),
			},
		})
	}
	parts = append(parts, geminiPart{Text: prompt})

	reqBody := geminiRequest{
		Contents: []geminiContent{{Parts: parts}},
	}
	payload, _ := json.Marshal(reqBody)

	maxRetries := 50 // Safe max retries, 50 * 10s = roughly 8.3 minutes
	for attempt := 0; attempt < maxRetries; attempt++ {
		key := c.getNextKey()
		req, _ := http.NewRequest(http.MethodPost, geminiAPIURL+key, bytes.NewBuffer(payload))
		req.Header.Set("Content-Type", "application/json")

		resp, err := c.httpClient.Do(req)
		if err != nil {
			if onRetry != nil {
				onRetry(attempt+1, maxRetries, err)
			}
			time.Sleep(10 * time.Second)
			continue
		}

		bodyBytes, _ := io.ReadAll(resp.Body)
		resp.Body.Close()

		if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500 {
			if onRetry != nil {
				onRetry(attempt+1, maxRetries, fmt.Errorf("gemini error: %d %s", resp.StatusCode, string(bodyBytes)))
			}
			time.Sleep(10 * time.Second)
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
		startIdx := strings.Index(rawText, "{")
		endIdx := strings.LastIndex(rawText, "}")
		if startIdx != -1 && endIdx != -1 && endIdx > startIdx {
			rawText = rawText[startIdx : endIdx+1]
		} else {
			return nil, fmt.Errorf("no json structure found: %s", rawText)
		}

		var result AnalysisResult
		if err := json.Unmarshal([]byte(rawText), &result); err != nil {
			return nil, err
		}
		return &result, nil
	}
	return nil, fmt.Errorf("ai failed after %d retries", maxRetries)
}
