package main

import "time"

const questionCount = 15

type AppSettings struct {
	Provider                  string   `json:"provider"`
	HasAPIKey                 bool     `json:"hasApiKey"`
	HasOpenAIKey              bool     `json:"hasOpenAIKey"`
	HasGeminiKey              bool     `json:"hasGeminiKey"`
	DemoMode                  bool     `json:"demoMode"`
	EvaluationModel           string   `json:"evaluationModel"`
	TranscribeModel           string   `json:"transcribeModel"`
	RealtimeModel             string   `json:"realtimeModel"`
	SpeechModel               string   `json:"speechModel"`
	SpeechVoice               string   `json:"speechVoice"`
	GeminiEvaluationModels    []string `json:"geminiEvaluationModels"`
	GeminiTranscriptionModels []string `json:"geminiTranscriptionModels"`
	GeminiSpeechModels        []string `json:"geminiSpeechModels"`
	GeminiSpeechVoice         string   `json:"geminiSpeechVoice"`
}

type ConfigureRequest struct {
	Provider                  string   `json:"provider"`
	APIKey                    string   `json:"apiKey"`
	GeminiAPIKey              string   `json:"geminiApiKey"`
	DemoMode                  bool     `json:"demoMode"`
	EvaluationModel           string   `json:"evaluationModel"`
	TranscribeModel           string   `json:"transcribeModel"`
	RealtimeModel             string   `json:"realtimeModel"`
	SpeechModel               string   `json:"speechModel"`
	SpeechVoice               string   `json:"speechVoice"`
	GeminiEvaluationModels    []string `json:"geminiEvaluationModels"`
	GeminiTranscriptionModels []string `json:"geminiTranscriptionModels"`
	GeminiSpeechModels        []string `json:"geminiSpeechModels"`
	GeminiSpeechVoice         string   `json:"geminiSpeechVoice"`
}

type SpeechResponse struct {
	AudioBase64 string `json:"audioBase64"`
	MimeType    string `json:"mimeType"`
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
	Score             int               `json:"score"`
	EstimatedBand     string            `json:"estimatedBand"`
	Summary           string            `json:"summary"`
	Strengths         []string          `json:"strengths"`
	Weaknesses        []string          `json:"weaknesses"`
	Priorities        []string          `json:"priorities"`
	TargetGrade       string            `json:"targetGrade"`
	TargetStatus      string            `json:"targetStatus"`
	TargetProbability int               `json:"targetProbability"`
	GradePredictions  []GradePrediction `json:"gradePredictions"`
}

type GradePrediction struct {
	Grade       string `json:"grade"`
	Probability int    `json:"probability"`
	Status      string `json:"status"`
	Description string `json:"description"`
}

type ExamReport struct {
	SessionID     string         `json:"sessionId"`
	Config        ExamConfig     `json:"config"`
	Overall       OverallReport  `json:"overall"`
	Answers       []AnswerRecord `json:"answers"`
	AnsweredCount int            `json:"answeredCount"`
	TotalCount    int            `json:"totalCount"`
	Partial       bool           `json:"partial"`
	GeneratedAt   time.Time      `json:"generatedAt"`
}
