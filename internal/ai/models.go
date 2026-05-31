package ai

// AnalysisResult — это строго типизированный ответ, который мы ожидаем от LLM.
type AnalysisResult struct {
	Title        string   `json:"title"`
	Folder       string   `json:"folder"`
	Tags         []string `json:"tags"`
	IsTask       bool     `json:"is_task"`
	ReminderTime string   `json:"reminder_time"` // Новое поле для даты/времени напоминания
}

type geminiRequest struct {
	Contents []geminiContent `json:"contents"`
}

type geminiContent struct {
	Parts []geminiPart `json:"parts"`
}

type geminiPart struct {
	Text string `json:"text"`
}

// Структуры для парсинга REST-ответа от Gemini
type geminiResponse struct {
	Candidates []struct {
		Content struct {
			Parts []struct {
				Text string `json:"text"`
			} `json:"parts"`
		} `json:"content"`
	} `json:"candidates"`
}