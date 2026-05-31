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
- Projects (Проекты): Задачи с конечной целью и дедлайном.
- Areas (Области): Зоны ответственности без сроков окончания (здоровье, финансы).
- Resources (Ресурсы): Полезная информация, конспекты, статьи по интересам.

АЛГОРИТМ МАРШРУТИЗАЦИИ (КРИТИЧЕСКИ ВАЖНО):
1. Изучи переданный список существующих путей (Files: [%s]).
2. Выбери подходящую Корневую папку (Projects, Areas или Resources).
3. Поиск подпапки (Кластера):
   - ЕСЛИ в выбранной корневой папке УЖЕ ЕСТЬ подпапка с похожим смыслом (например, '01_Projects/🎓 Дипломная работа') -> ставь "action": "create". В поле "target_folder" скопируй ТОЧНЫЙ путь к этой подпапке из списка. ЗАПРЕЩЕНО создавать папку внутри существующей папки! (Глупо делать 'Дипломная/Дипломная').
   - ЕСЛИ подходящей подпапки НЕТ -> придумай новую широкую категорию (например, '03_Resources/Программирование') и запиши её в "target_folder". 
4. Кластеризация одиночек ("reorganize"): Если заметка логически дополняет существующий ОДИНОЧНЫЙ файл (лежащий прямо в корне PARA), ставь "action": "reorganize". В "target_file_to_move" укажи путь к этому старому файлу, в "target_folder" — корневую папку PARA, а в "cluster_name" — имя новой объединяющей подпапки, куда переедут оба файла.

ПРАВИЛА ОФОРМЛЕНИЯ ЗАМЕТКИ:
5. "file_name": английский или транслит, без даты, snake_case (напр. 'diploma_check').
6. "title": Человекочитаемый заголовок для H1.
7. "tags": 1-3 тега в формате 'область/категория'. БЕЗ знака #.
8. "priority": Оцени срочность (High, Medium, Low).
9. "is_task" и "tasks": Если в тексте есть план/призыв к действию, ставь "is_task": true и выпиши шаги в массив строк "tasks" (без маркдаун чек-боксов). Иначе "is_task": false и [].
10. "content": Основной описательный текст.

ФОРМАТ ОТВЕТА (JSON):
{
  "action": "create",
  "target_folder": "01_Projects/🎓 Дипломная работа",
  "cluster_name": "",
  "target_file_to_move": "",
  "file_name": "diploma_check",
  "title": "Проверка диплома",
  "tags": ["образование/университет"],
  "is_task": true,
  "priority": "High",
  "content": "Дипломная работа...",
  "tasks": ["Проверить антиплагиат", "Добавить источники"]
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