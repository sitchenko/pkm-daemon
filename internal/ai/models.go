package ai

// AnalysisResult — строго типизированный ответ от Gemini
type AnalysisResult struct {
	Action       string   `json:"action"`
	TargetFolder string   `json:"target_folder"`
	ClusterName  string   `json:"cluster_name"`
	FileName     string   `json:"file_name"`
	Title        string   `json:"title"`    // Заголовок внутри заметки (H1)
	Tags         []string `json:"tags"`     // Массив тегов
	IsTask       bool     `json:"is_task"`  // Флаг наличия задач
	Priority     string   `json:"priority"` // Приоритет: High, Medium, Low
	Content      string   `json:"content"`  // Описание заметки
	Tasks        []string `json:"tasks"`    // Список шагов (если is_task = true)
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

type geminiResponse struct {
	Candidates []struct {
		Content struct {
			Parts []struct {
				Text string `json:"text"`
			} `json:"parts"`
		} `json:"content"`
	} `json:"candidates"`
}