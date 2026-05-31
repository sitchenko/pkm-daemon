package ai

type AnalysisResult struct {
	Action           string         `json:"action"`
	TargetFolder     string         `json:"target_folder"`
	ClusterName      string         `json:"cluster_name"`
	TargetFileToMove string         `json:"target_file_to_move"`
	FileName         string         `json:"file_name"`
	Title            string         `json:"title"`
	Tags             []string       `json:"tags"`
	IsTask           bool           `json:"is_task"`
	Priority         string         `json:"priority"`
	Content          string         `json:"content"`
	Tasks            []string       `json:"tasks"`
	Reminders        []ReminderInfo `json:"reminders"` // Изменено на массив
}

type ReminderInfo struct {
	Time string `json:"time"`
	Text string `json:"text"`
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