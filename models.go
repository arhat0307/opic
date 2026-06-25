package main

import "time"

const questionCount = 15

type AppSettings struct {
	HasAPIKey       bool   `json:"hasApiKey"`
	DemoMode        bool   `json:"demoMode"`
	EvaluationModel string `json:"evaluationModel"`
	TranscribeModel string `json:"transcribeModel"`
	RealtimeModel   string `json:"realtimeModel"`
}

type ConfigureRequest struct {
	APIKey          string `json:"apiKey"`
	DemoMode        bool   `json:"demoMode"`
	EvaluationModel string `json:"evaluationModel"`
	TranscribeModel string `json:"transcribeModel"`
	RealtimeModel   string `json:"realtimeModel"`
}

type ExamConfig struct {
	Difficulty string   `json:"difficulty"`
	Topics     []string `json:"topics"`
	Language   string   `json:"language"`
}

type Question struct {
	Index    int    `json:"index"`
	Category string `json:"category"`
	Topic    string `json:"topic"`
	Text     string `json:"text"`
	Intent   string `json:"intent"`
}

type AnswerEvaluation struct {
	Score        int      `json:"score"`
	Keywords     []string `json:"keywords"`
	Strengths    []string `json:"strengths"`
	Improvements []string `json:"improvements"`
	Feedback     string   `json:"feedback"`
}

type AnswerRecord struct {
	Question     Question         `json:"question"`
	Transcript   string           `json:"transcript"`
	DurationSec  int              `json:"durationSec"`
	Evaluation   AnswerEvaluation `json:"evaluation"`
	AnsweredAt   time.Time        `json:"answeredAt"`
	AudioPresent bool             `json:"audioPresent"`
}

type ExamSession struct {
	ID        string         `json:"id"`
	Config    ExamConfig     `json:"config"`
	Questions []Question     `json:"questions"`
	Answers   []AnswerRecord `json:"answers"`
	StartedAt time.Time      `json:"startedAt"`
	Completed bool           `json:"completed"`
}

type StartSessionResponse struct {
	SessionID  string   `json:"sessionId"`
	Question   Question `json:"question"`
	TotalCount int      `json:"totalCount"`
}

type SubmitAnswerRequest struct {
	SessionID   string `json:"sessionId"`
	QuestionIdx int    `json:"questionIdx"`
	AudioBase64 string `json:"audioBase64"`
	AudioMime   string `json:"audioMime"`
	Transcript  string `json:"transcript"`
	DurationSec int    `json:"durationSec"`
}

type SubmitAnswerResponse struct {
	Transcript string           `json:"transcript"`
	Evaluation AnswerEvaluation `json:"evaluation"`
	Next       *Question        `json:"next,omitempty"`
	Completed  bool             `json:"completed"`
	Progress   int              `json:"progress"`
}

type OverallReport struct {
	Score         int      `json:"score"`
	EstimatedBand string   `json:"estimatedBand"`
	Summary       string   `json:"summary"`
	Strengths     []string `json:"strengths"`
	Priorities    []string `json:"priorities"`
}

type ExamReport struct {
	SessionID   string         `json:"sessionId"`
	Config      ExamConfig     `json:"config"`
	Overall     OverallReport  `json:"overall"`
	Answers     []AnswerRecord `json:"answers"`
	GeneratedAt time.Time      `json:"generatedAt"`
}

