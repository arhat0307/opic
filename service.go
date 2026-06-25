package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
)

type AppService struct {
	mu        sync.RWMutex
	apiKey    string
	geminiKey string
	settings  AppSettings
	sessions  map[string]*ExamSession
	speech    map[string]SpeechResponse
	client    *http.Client
}

func NewAppService() *AppService {
	key := strings.TrimSpace(os.Getenv("OPENAI_API_KEY"))
	geminiKey := strings.TrimSpace(os.Getenv("GEMINI_API_KEY"))
	provider := "openai"
	if key == "" && geminiKey != "" {
		provider = "gemini"
	}
	return &AppService{
		apiKey:    key,
		geminiKey: geminiKey,
		settings: AppSettings{
			Provider:                  provider,
			HasAPIKey:                 key != "" || geminiKey != "",
			HasOpenAIKey:              key != "",
			HasGeminiKey:              geminiKey != "",
			DemoMode:                  key == "" && geminiKey == "",
			EvaluationModel:           "gpt-5-mini",
			TranscribeModel:           "gpt-4o-transcribe",
			RealtimeModel:             "gpt-realtime",
			SpeechModel:               "gpt-4o-mini-tts",
			SpeechVoice:               "marin",
			GeminiEvaluationModels:    []string{"gemini-3.5-flash", "gemini-2.5-flash", "gemini-2.5-flash-lite"},
			GeminiTranscriptionModels: []string{"gemini-3.5-flash", "gemini-2.5-flash", "gemini-2.5-flash-lite"},
			GeminiSpeechModels:        []string{"gemini-3.1-flash-tts-preview", "gemini-2.5-flash-preview-tts"},
			GeminiSpeechVoice:         "Kore",
		},
		sessions: make(map[string]*ExamSession),
		speech:   make(map[string]SpeechResponse),
		client:   &http.Client{Timeout: 90 * time.Second},
	}
}

func (s *AppService) GetSettings() AppSettings {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.settings
}

func (s *AppService) Configure(request ConfigureRequest) (AppSettings, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if strings.TrimSpace(request.APIKey) != "" {
		s.apiKey = strings.TrimSpace(request.APIKey)
	}
	if strings.TrimSpace(request.GeminiAPIKey) != "" {
		s.geminiKey = strings.TrimSpace(request.GeminiAPIKey)
	}
	if request.Provider == "openai" || request.Provider == "gemini" {
		s.settings.Provider = request.Provider
	}
	if request.EvaluationModel != "" {
		s.settings.EvaluationModel = request.EvaluationModel
	}
	if request.TranscribeModel != "" {
		s.settings.TranscribeModel = request.TranscribeModel
	}
	if request.RealtimeModel != "" {
		s.settings.RealtimeModel = request.RealtimeModel
	}
	if request.SpeechModel != "" {
		s.settings.SpeechModel = request.SpeechModel
	}
	if request.SpeechVoice != "" {
		s.settings.SpeechVoice = request.SpeechVoice
	}
	if models := cleanModels(request.GeminiEvaluationModels); len(models) > 0 {
		s.settings.GeminiEvaluationModels = models
	}
	if models := cleanModels(request.GeminiTranscriptionModels); len(models) > 0 {
		s.settings.GeminiTranscriptionModels = models
	}
	if models := cleanModels(request.GeminiSpeechModels); len(models) > 0 {
		s.settings.GeminiSpeechModels = models
	}
	if request.GeminiSpeechVoice != "" {
		s.settings.GeminiSpeechVoice = request.GeminiSpeechVoice
	}
	s.settings.DemoMode = request.DemoMode
	s.settings.HasOpenAIKey = s.apiKey != ""
	s.settings.HasGeminiKey = s.geminiKey != ""
	s.settings.HasAPIKey = (s.settings.Provider == "openai" && s.settings.HasOpenAIKey) ||
		(s.settings.Provider == "gemini" && s.settings.HasGeminiKey)
	return s.settings, nil
}

func (s *AppService) TestConnection() error {
	s.mu.RLock()
	key := s.apiKey
	geminiKey := s.geminiKey
	provider := s.settings.Provider
	s.mu.RUnlock()
	if provider == "gemini" {
		if geminiKey == "" {
			return errors.New("Gemini API 키가 설정되지 않았습니다")
		}
		req, _ := http.NewRequest(http.MethodGet, "https://generativelanguage.googleapis.com/v1beta/models", nil)
		req.Header.Set("x-goog-api-key", geminiKey)
		res, err := s.client.Do(req)
		if err != nil {
			return fmt.Errorf("Gemini 연결 실패: %w", err)
		}
		defer res.Body.Close()
		if res.StatusCode >= 300 {
			body, _ := io.ReadAll(io.LimitReader(res.Body, 2048))
			return fmt.Errorf("Gemini 연결 실패 (%d): %s", res.StatusCode, string(body))
		}
		return nil
	}
	if key == "" {
		return errors.New("OpenAI API 키가 설정되지 않았습니다")
	}
	req, _ := http.NewRequest(http.MethodGet, "https://api.openai.com/v1/models", nil)
	req.Header.Set("Authorization", "Bearer "+key)
	res, err := s.client.Do(req)
	if err != nil {
		return fmt.Errorf("OpenAI 연결 실패: %w", err)
	}
	defer res.Body.Close()
	if res.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(res.Body, 2048))
		return fmt.Errorf("OpenAI 연결 실패 (%d): %s", res.StatusCode, string(body))
	}
	return nil
}

func (s *AppService) GenerateSpeech(text string) (SpeechResponse, error) {
	text = strings.TrimSpace(text)
	if text == "" {
		return SpeechResponse{}, errors.New("읽을 질문이 없습니다")
	}

	s.mu.RLock()
	key := s.apiKey
	geminiKey := s.geminiKey
	settings := s.settings
	cacheKey := settings.Provider + "|" + settings.SpeechModel + "|" + settings.SpeechVoice + "|" +
		strings.Join(settings.GeminiSpeechModels, ",") + "|" + settings.GeminiSpeechVoice + "|" + text
	cached, ok := s.speech[cacheKey]
	s.mu.RUnlock()
	if ok {
		return cached, nil
	}
	if settings.DemoMode {
		return SpeechResponse{}, errors.New("자연스러운 AI 음성을 사용하려면 API 연결과 데모 모드 해제가 필요합니다")
	}
	if settings.Provider == "gemini" {
		if geminiKey == "" {
			return SpeechResponse{}, errors.New("Gemini API 키가 설정되지 않았습니다")
		}
		result, err := s.generateGeminiSpeech(text, geminiKey, settings)
		if err != nil {
			return SpeechResponse{}, err
		}
		s.mu.Lock()
		s.speech[cacheKey] = result
		s.mu.Unlock()
		return result, nil
	}
	if key == "" {
		return SpeechResponse{}, errors.New("OpenAI API 키가 설정되지 않았습니다")
	}

	body, _ := json.Marshal(map[string]any{
		"model":           settings.SpeechModel,
		"voice":           settings.SpeechVoice,
		"input":           text,
		"instructions":    "Speak as a professional American English speaking-test interviewer. Use a natural, warm, neutral conversational tone. Pronounce every word clearly without sounding robotic. Use subtle American English intonation, natural reductions, and brief pauses at commas and between ideas. Do not sound dramatic, cheerful, or like an advertisement. Ask the question once at a measured interview pace.",
		"response_format": "mp3",
	})
	req, _ := http.NewRequest(http.MethodPost, "https://api.openai.com/v1/audio/speech", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+key)
	req.Header.Set("Content-Type", "application/json")
	res, err := s.client.Do(req)
	if err != nil {
		return SpeechResponse{}, fmt.Errorf("AI 음성 생성 요청 실패: %w", err)
	}
	defer res.Body.Close()
	audio, _ := io.ReadAll(res.Body)
	if res.StatusCode >= 300 {
		return SpeechResponse{}, fmt.Errorf("AI 음성 생성 실패 (%d): %s", res.StatusCode, string(audio))
	}

	result := SpeechResponse{
		AudioBase64: base64.StdEncoding.EncodeToString(audio),
		MimeType:    "audio/mpeg",
	}
	s.mu.Lock()
	s.speech[cacheKey] = result
	s.mu.Unlock()
	return result, nil
}

func (s *AppService) StartSession(config ExamConfig) (StartSessionResponse, error) {
	if len(config.Topics) == 0 {
		return StartSessionResponse{}, errors.New("주제를 하나 이상 선택하세요")
	}
	questions := buildQuestions(config)
	session := &ExamSession{
		ID:        newID(),
		Config:    config,
		Questions: questions,
		Answers:   []AnswerRecord{},
		StartedAt: time.Now(),
	}
	s.mu.Lock()
	s.sessions[session.ID] = session
	s.mu.Unlock()
	return StartSessionResponse{SessionID: session.ID, Question: questions[0], TotalCount: len(questions)}, nil
}

func (s *AppService) SubmitAnswer(request SubmitAnswerRequest) (SubmitAnswerResponse, error) {
	s.mu.RLock()
	session, ok := s.sessions[request.SessionID]
	settings := s.settings
	s.mu.RUnlock()
	if !ok {
		return SubmitAnswerResponse{}, errors.New("시험 세션을 찾을 수 없습니다")
	}
	if request.QuestionIdx < 0 || request.QuestionIdx >= len(session.Questions) {
		return SubmitAnswerResponse{}, errors.New("잘못된 문항 번호입니다")
	}
	if len(session.Answers) != request.QuestionIdx {
		return SubmitAnswerResponse{}, errors.New("문항은 순서대로 제출해야 합니다")
	}

	transcript := strings.TrimSpace(request.Transcript)
	if transcript == "" && request.AudioBase64 != "" {
		if settings.DemoMode {
			transcript = demoTranscript(session.Questions[request.QuestionIdx])
		} else {
			var err error
			transcript, err = s.transcribe(request.AudioBase64, request.AudioMime, settings)
			if err != nil {
				return SubmitAnswerResponse{}, err
			}
		}
	}
	if transcript == "" {
		return SubmitAnswerResponse{}, errors.New("녹음 또는 답변 텍스트가 필요합니다")
	}

	question := session.Questions[request.QuestionIdx]
	var evaluation AnswerEvaluation
	var err error
	if settings.DemoMode {
		evaluation = evaluateDemo(question, transcript, request.DurationSec)
	} else {
		if settings.Provider == "gemini" {
			evaluation, err = s.evaluateWithGemini(session, question, transcript, request.DurationSec, settings)
		} else {
			evaluation, err = s.evaluateWithOpenAI(session, question, transcript, request.DurationSec)
		}
		if err != nil {
			return SubmitAnswerResponse{}, err
		}
	}

	record := AnswerRecord{
		Question: question, Transcript: transcript, DurationSec: request.DurationSec,
		Evaluation: evaluation, AnsweredAt: time.Now(), AudioPresent: request.AudioBase64 != "",
	}
	s.mu.Lock()
	session.Answers = append(session.Answers, record)
	completed := len(session.Answers) == len(session.Questions)
	session.Completed = completed
	s.mu.Unlock()

	response := SubmitAnswerResponse{
		Transcript: transcript, Evaluation: evaluation, Completed: completed,
		Progress: len(session.Answers),
	}
	if !completed {
		next := session.Questions[len(session.Answers)]
		response.Next = &next
	}
	return response, nil
}

func (s *AppService) GetReport(sessionID string) (ExamReport, error) {
	s.mu.RLock()
	session, ok := s.sessions[sessionID]
	s.mu.RUnlock()
	if !ok {
		return ExamReport{}, errors.New("시험 세션을 찾을 수 없습니다")
	}
	if !session.Completed {
		return ExamReport{}, errors.New("아직 모든 문항이 완료되지 않았습니다")
	}
	return buildReport(session), nil
}

func (s *AppService) FinalizeSession(sessionID string) (ExamReport, error) {
	s.mu.Lock()
	session, ok := s.sessions[sessionID]
	if !ok {
		s.mu.Unlock()
		return ExamReport{}, errors.New("시험 세션을 찾을 수 없습니다")
	}
	if len(session.Answers) == 0 {
		s.mu.Unlock()
		return ExamReport{}, errors.New("총평을 생성하려면 한 문항 이상 답변해야 합니다")
	}
	session.Completed = true
	s.mu.Unlock()
	return buildReport(session), nil
}

func (s *AppService) transcribe(encoded, mimeType string, settings AppSettings) (string, error) {
	if settings.Provider == "gemini" {
		return s.transcribeWithGemini(encoded, mimeType, settings)
	}
	raw, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return "", errors.New("녹음 데이터를 읽을 수 없습니다")
	}
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	ext := ".webm"
	if strings.Contains(mimeType, "mp4") {
		ext = ".m4a"
	}
	part, _ := writer.CreateFormFile("file", "answer"+ext)
	_, _ = part.Write(raw)
	_ = writer.WriteField("model", s.settings.TranscribeModel)
	_ = writer.WriteField("language", "en")
	_ = writer.Close()

	req, _ := http.NewRequest(http.MethodPost, "https://api.openai.com/v1/audio/transcriptions", &body)
	req.Header.Set("Authorization", "Bearer "+s.apiKey)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	res, err := s.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("음성 전사 요청 실패: %w", err)
	}
	defer res.Body.Close()
	payload, _ := io.ReadAll(res.Body)
	if res.StatusCode >= 300 {
		return "", fmt.Errorf("음성 전사 실패 (%d): %s", res.StatusCode, string(payload))
	}
	var result struct {
		Text string `json:"text"`
	}
	if err := json.Unmarshal(payload, &result); err != nil {
		return "", errors.New("음성 전사 응답을 해석할 수 없습니다")
	}
	return strings.TrimSpace(result.Text), nil
}

func (s *AppService) evaluateWithOpenAI(session *ExamSession, question Question, transcript string, duration int) (AnswerEvaluation, error) {
	history := make([]map[string]any, 0, len(session.Answers))
	for _, answer := range session.Answers {
		history = append(history, map[string]any{
			"question": answer.Question.Text, "answer": answer.Transcript, "score": answer.Evaluation.Score,
		})
	}
	input := map[string]any{
		"difficulty": session.Config.Difficulty, "topics": session.Config.Topics,
		"question": question, "answer": transcript, "duration_seconds": duration, "previous_turns": history,
	}
	inputJSON, _ := json.Marshal(input)
	prompt := `You are a rigorous OPIc speaking coach. Evaluate the learner's English answer from 0 to 100.
Judge task completion, detail, organization, vocabulary, grammar, fluency inferred from transcript and duration, and natural spoken style.
Do not inflate the score. Return all coaching feedback, strengths, and improvements in concise natural English. Keywords must be useful English words or phrases from the answer.
The next question is already controlled by the application, so do not generate one.

INPUT:
` + string(inputJSON)

	schema := map[string]any{
		"type": "object", "additionalProperties": false,
		"properties": map[string]any{
			"score":        map[string]any{"type": "integer", "minimum": 0, "maximum": 100},
			"keywords":     map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "maxItems": 8},
			"strengths":    map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "maxItems": 4},
			"improvements": map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "maxItems": 4},
			"feedback":     map[string]any{"type": "string"},
		},
		"required": []string{"score", "keywords", "strengths", "improvements", "feedback"},
	}
	requestBody := map[string]any{
		"model": s.settings.EvaluationModel,
		"input": prompt,
		"text": map[string]any{"format": map[string]any{
			"type": "json_schema", "name": "opic_answer_evaluation", "strict": true, "schema": schema,
		}},
	}
	encoded, _ := json.Marshal(requestBody)
	req, _ := http.NewRequestWithContext(context.Background(), http.MethodPost, "https://api.openai.com/v1/responses", bytes.NewReader(encoded))
	req.Header.Set("Authorization", "Bearer "+s.apiKey)
	req.Header.Set("Content-Type", "application/json")
	res, err := s.client.Do(req)
	if err != nil {
		return AnswerEvaluation{}, fmt.Errorf("답변 평가 요청 실패: %w", err)
	}
	defer res.Body.Close()
	payload, _ := io.ReadAll(res.Body)
	if res.StatusCode >= 300 {
		return AnswerEvaluation{}, fmt.Errorf("답변 평가 실패 (%d): %s", res.StatusCode, string(payload))
	}
	text, err := extractResponseText(payload)
	if err != nil {
		return AnswerEvaluation{}, err
	}
	var evaluation AnswerEvaluation
	if err := json.Unmarshal([]byte(text), &evaluation); err != nil {
		return AnswerEvaluation{}, errors.New("평가 결과를 해석할 수 없습니다")
	}
	return evaluation, nil
}

func extractResponseText(payload []byte) (string, error) {
	var response struct {
		Output []struct {
			Content []struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"content"`
		} `json:"output"`
	}
	if err := json.Unmarshal(payload, &response); err != nil {
		return "", errors.New("OpenAI 응답을 해석할 수 없습니다")
	}
	for _, output := range response.Output {
		for _, content := range output.Content {
			if content.Type == "output_text" && content.Text != "" {
				return content.Text, nil
			}
		}
	}
	return "", errors.New("OpenAI 응답에 평가 텍스트가 없습니다")
}

func (s *AppService) evaluateWithGemini(session *ExamSession, question Question, transcript string, duration int, settings AppSettings) (AnswerEvaluation, error) {
	history := make([]map[string]any, 0, len(session.Answers))
	for _, answer := range session.Answers {
		history = append(history, map[string]any{
			"question": answer.Question.Text, "answer": answer.Transcript, "score": answer.Evaluation.Score,
		})
	}
	input := map[string]any{
		"difficulty": session.Config.Difficulty, "topics": session.Config.Topics,
		"question": question, "answer": transcript, "duration_seconds": duration, "previous_turns": history,
	}
	inputJSON, _ := json.Marshal(input)
	prompt := `You are a rigorous OPIc speaking coach. Evaluate the learner's English answer from 0 to 100.
Judge task completion, detail, organization, vocabulary, grammar, fluency inferred from transcript and duration, and natural spoken style.
Do not inflate the score. Return all coaching feedback, strengths, and improvements in concise natural English. Keywords must be useful English words or phrases from the answer.
The next question is already controlled by the application, so do not generate one.

INPUT:
` + string(inputJSON)
	schema := evaluationSchema()
	payload, _, err := s.callGeminiValidatedWithFallback(settings.GeminiEvaluationModels, func(model string) map[string]any {
		return map[string]any{
			"model": model,
			"input": prompt,
			"response_format": map[string]any{
				"type": "text", "mime_type": "application/json", "schema": schema,
			},
		}
	}, func(payload []byte) error {
		text, err := extractGeminiText(payload)
		if err != nil {
			return err
		}
		var evaluation AnswerEvaluation
		if err := json.Unmarshal([]byte(text), &evaluation); err != nil {
			return fmt.Errorf("평가 JSON 오류: %w", err)
		}
		if evaluation.Score < 0 || evaluation.Score > 100 {
			return errors.New("평가 점수가 0~100 범위를 벗어남")
		}
		return nil
	})
	if err != nil {
		return AnswerEvaluation{}, fmt.Errorf("Gemini 답변 평가 실패: %w", err)
	}
	text, err := extractGeminiText(payload)
	if err != nil {
		return AnswerEvaluation{}, err
	}
	var evaluation AnswerEvaluation
	if err := json.Unmarshal([]byte(text), &evaluation); err != nil {
		return AnswerEvaluation{}, fmt.Errorf("Gemini 평가 결과를 해석할 수 없습니다: %w", err)
	}
	if evaluation.Score < 0 || evaluation.Score > 100 {
		return AnswerEvaluation{}, errors.New("Gemini 평가 점수가 0~100 범위를 벗어났습니다")
	}
	return evaluation, nil
}

func (s *AppService) transcribeWithGemini(encoded, mimeType string, settings AppSettings) (string, error) {
	if strings.TrimSpace(encoded) == "" {
		return "", errors.New("녹음 데이터가 없습니다")
	}
	if mimeType == "" {
		mimeType = "audio/webm"
	}
	payload, _, err := s.callGeminiValidatedWithFallback(settings.GeminiTranscriptionModels, func(model string) map[string]any {
		return map[string]any{
			"model": model,
			"input": []map[string]any{
				{"type": "text", "text": "Transcribe only the spoken English in this audio accurately. Return plain text only. Preserve natural punctuation and do not add commentary."},
				{"type": "audio", "data": encoded, "mime_type": mimeType},
			},
		}
	}, func(payload []byte) error {
		text, err := extractGeminiText(payload)
		if err != nil {
			return err
		}
		if strings.TrimSpace(text) == "" {
			return errors.New("빈 전사 결과")
		}
		return nil
	})
	if err != nil {
		return "", fmt.Errorf("Gemini 음성 전사 실패: %w", err)
	}
	text, err := extractGeminiText(payload)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(text), nil
}

func (s *AppService) generateGeminiSpeech(text, key string, settings AppSettings) (SpeechResponse, error) {
	prompt := `Speak as a professional American English speaking-test interviewer. Use a natural, warm, neutral conversational tone, clear pronunciation, subtle American intonation, and brief pauses between ideas. Do not sound dramatic or like an advertisement. Ask this question exactly once:

` + text
	payload, model, err := s.callGeminiWithKeyValidatedFallback(key, settings.GeminiSpeechModels, func(model string) map[string]any {
		return map[string]any{
			"model":           model,
			"input":           prompt,
			"response_format": map[string]any{"type": "audio"},
			"generation_config": map[string]any{
				"speech_config": []map[string]any{{"voice": settings.GeminiSpeechVoice}},
			},
		}
	}, func(payload []byte) error {
		audio, err := extractGeminiAudio(payload)
		if err != nil {
			return err
		}
		if len(audio) == 0 {
			return errors.New("빈 오디오 결과")
		}
		return nil
	})
	if err != nil {
		return SpeechResponse{}, fmt.Errorf("Gemini TTS 실패: %w", err)
	}
	audio, err := extractGeminiAudio(payload)
	if err != nil {
		return SpeechResponse{}, fmt.Errorf("%s 응답의 음성을 해석할 수 없습니다: %w", model, err)
	}
	wav := pcmToWAV(audio, 24000, 1, 16)
	return SpeechResponse{AudioBase64: base64.StdEncoding.EncodeToString(wav), MimeType: "audio/wav"}, nil
}

func (s *AppService) callGeminiWithFallback(models []string, bodyForModel func(string) map[string]any) ([]byte, string, error) {
	return s.callGeminiValidatedWithFallback(models, bodyForModel, nil)
}

func (s *AppService) callGeminiValidatedWithFallback(models []string, bodyForModel func(string) map[string]any, validate func([]byte) error) ([]byte, string, error) {
	s.mu.RLock()
	key := s.geminiKey
	s.mu.RUnlock()
	if key == "" {
		return nil, "", errors.New("Gemini API 키가 설정되지 않았습니다")
	}
	return s.callGeminiWithKeyValidatedFallback(key, models, bodyForModel, validate)
}

func (s *AppService) callGeminiWithKeyFallback(key string, models []string, bodyForModel func(string) map[string]any) ([]byte, string, error) {
	return s.callGeminiWithKeyValidatedFallback(key, models, bodyForModel, nil)
}

func (s *AppService) callGeminiWithKeyValidatedFallback(key string, models []string, bodyForModel func(string) map[string]any, validate func([]byte) error) ([]byte, string, error) {
	models = cleanModels(models)
	if len(models) == 0 {
		return nil, "", errors.New("시도할 Gemini 모델이 없습니다")
	}
	failures := make([]string, 0, len(models))
	for _, model := range models {
		encoded, _ := json.Marshal(bodyForModel(model))
		req, _ := http.NewRequestWithContext(context.Background(), http.MethodPost, "https://generativelanguage.googleapis.com/v1beta/interactions", bytes.NewReader(encoded))
		req.Header.Set("x-goog-api-key", key)
		req.Header.Set("Content-Type", "application/json")
		res, err := s.client.Do(req)
		if err != nil {
			failures = append(failures, model+": "+err.Error())
			continue
		}
		payload, readErr := io.ReadAll(res.Body)
		res.Body.Close()
		if readErr != nil {
			failures = append(failures, model+": "+readErr.Error())
			continue
		}
		if res.StatusCode >= 300 {
			failures = append(failures, fmt.Sprintf("%s: HTTP %d %s", model, res.StatusCode, compactError(payload)))
			continue
		}
		if len(payload) == 0 {
			failures = append(failures, model+": 빈 응답")
			continue
		}
		if validate != nil {
			if err := validate(payload); err != nil {
				failures = append(failures, model+": 응답 검증 실패: "+err.Error())
				continue
			}
		}
		return payload, model, nil
	}
	return nil, "", errors.New(strings.Join(failures, " | "))
}

func evaluationSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"score":        map[string]any{"type": "integer", "minimum": 0, "maximum": 100},
			"keywords":     map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
			"strengths":    map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
			"improvements": map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
			"feedback":     map[string]any{"type": "string"},
		},
		"required": []string{"score", "keywords", "strengths", "improvements", "feedback"},
	}
}

func extractGeminiText(payload []byte) (string, error) {
	var root any
	if err := json.Unmarshal(payload, &root); err != nil {
		return "", fmt.Errorf("Gemini 응답 JSON 오류: %w", err)
	}
	if text := findStringField(root, "output_text"); text != "" {
		return text, nil
	}
	if text := findTypedData(root, "text", "text"); text != "" {
		return text, nil
	}
	return "", errors.New("Gemini 응답에 텍스트가 없습니다")
}

func extractGeminiAudio(payload []byte) ([]byte, error) {
	var root any
	if err := json.Unmarshal(payload, &root); err != nil {
		return nil, err
	}
	encoded := findNestedData(root, "output_audio")
	if encoded == "" {
		encoded = findTypedData(root, "audio", "data")
	}
	if encoded == "" {
		return nil, errors.New("audio data 필드가 없습니다")
	}
	return base64.StdEncoding.DecodeString(encoded)
}

func findStringField(value any, key string) string {
	switch typed := value.(type) {
	case map[string]any:
		if text, ok := typed[key].(string); ok && text != "" {
			return text
		}
		for _, child := range typed {
			if result := findStringField(child, key); result != "" {
				return result
			}
		}
	case []any:
		for _, child := range typed {
			if result := findStringField(child, key); result != "" {
				return result
			}
		}
	}
	return ""
}

func findTypedData(value any, wantedType, field string) string {
	switch typed := value.(type) {
	case map[string]any:
		if kind, _ := typed["type"].(string); kind == wantedType {
			if result, _ := typed[field].(string); result != "" {
				return result
			}
		}
		for _, child := range typed {
			if result := findTypedData(child, wantedType, field); result != "" {
				return result
			}
		}
	case []any:
		for _, child := range typed {
			if result := findTypedData(child, wantedType, field); result != "" {
				return result
			}
		}
	}
	return ""
}

func findNestedData(value any, key string) string {
	switch typed := value.(type) {
	case map[string]any:
		if nested, ok := typed[key].(map[string]any); ok {
			if data, _ := nested["data"].(string); data != "" {
				return data
			}
		}
		for _, child := range typed {
			if result := findNestedData(child, key); result != "" {
				return result
			}
		}
	case []any:
		for _, child := range typed {
			if result := findNestedData(child, key); result != "" {
				return result
			}
		}
	}
	return ""
}

func pcmToWAV(pcm []byte, sampleRate, channels, bitsPerSample int) []byte {
	var output bytes.Buffer
	byteRate := sampleRate * channels * bitsPerSample / 8
	blockAlign := channels * bitsPerSample / 8
	output.WriteString("RIFF")
	_ = binary.Write(&output, binary.LittleEndian, uint32(36+len(pcm)))
	output.WriteString("WAVEfmt ")
	_ = binary.Write(&output, binary.LittleEndian, uint32(16))
	_ = binary.Write(&output, binary.LittleEndian, uint16(1))
	_ = binary.Write(&output, binary.LittleEndian, uint16(channels))
	_ = binary.Write(&output, binary.LittleEndian, uint32(sampleRate))
	_ = binary.Write(&output, binary.LittleEndian, uint32(byteRate))
	_ = binary.Write(&output, binary.LittleEndian, uint16(blockAlign))
	_ = binary.Write(&output, binary.LittleEndian, uint16(bitsPerSample))
	output.WriteString("data")
	_ = binary.Write(&output, binary.LittleEndian, uint32(len(pcm)))
	output.Write(pcm)
	return output.Bytes()
}

func cleanModels(models []string) []string {
	result := make([]string, 0, len(models))
	seen := map[string]bool{}
	for _, model := range models {
		model = strings.TrimSpace(model)
		if model != "" && !seen[model] {
			result = append(result, model)
			seen[model] = true
		}
	}
	return result
}

func compactError(payload []byte) string {
	text := strings.Join(strings.Fields(string(payload)), " ")
	if len(text) > 350 {
		return text[:350] + "…"
	}
	return text
}

func evaluateDemo(question Question, transcript string, duration int) AnswerEvaluation {
	words := strings.Fields(transcript)
	score := 38 + min(len(words)/2, 30) + min(duration/10, 10)
	lower := strings.ToLower(transcript)
	connectors := []string{"because", "however", "for example", "so", "first", "finally", "although", "when"}
	used := []string{}
	for _, item := range connectors {
		if strings.Contains(lower, item) {
			used = append(used, item)
			score += 3
		}
	}
	if strings.Contains(question.Category, "Role-play") && strings.Contains(transcript, "?") {
		score += 5
	}
	score = min(score, 92)
	keywords := extractKeywords(transcript)
	strengths := []string{"The response stayed relevant to the main task.", "The answer was organized around a personal experience."}
	improvements := []string{}
	if len(words) < 60 {
		improvements = append(improvements, "Add specific details about places, people, and feelings, and develop the answer beyond 60 words.")
	}
	if len(used) < 2 {
		improvements = append(improvements, "Use connectors such as because, for example, and however to make relationships between ideas clear.")
	}
	if duration < 40 {
		improvements = append(improvements, "Add one reason and one example so the response can naturally continue for about 40 to 70 seconds.")
	}
	if len(improvements) == 0 {
		improvements = append(improvements, "Reduce repeated vocabulary and use a wider range of natural spoken expressions.")
	}
	return AnswerEvaluation{
		Score: score, Keywords: keywords, Strengths: strengths, Improvements: improvements,
		Feedback: "The core message was clear. Develop the response through background, a specific event, and a final reaction or result to move toward a higher proficiency level.",
	}
}

func extractKeywords(text string) []string {
	re := regexp.MustCompile(`[A-Za-z][A-Za-z'-]{3,}`)
	words := re.FindAllString(strings.ToLower(text), -1)
	stop := map[string]bool{"this": true, "that": true, "with": true, "have": true, "were": true, "they": true, "about": true, "because": true, "really": true, "very": true}
	counts := map[string]int{}
	for _, word := range words {
		if !stop[word] {
			counts[word]++
		}
	}
	type pair struct {
		word  string
		count int
	}
	pairs := []pair{}
	for word, count := range counts {
		pairs = append(pairs, pair{word, count})
	}
	sort.Slice(pairs, func(i, j int) bool { return pairs[i].count > pairs[j].count })
	result := []string{}
	for _, item := range pairs {
		result = append(result, item.word)
		if len(result) == 6 {
			break
		}
	}
	return result
}

func buildReport(session *ExamSession) ExamReport {
	total := 0
	for _, answer := range session.Answers {
		total += answer.Evaluation.Score
	}
	score := 0
	if len(session.Answers) > 0 {
		score = total / len(session.Answers)
	}
	band := "NL"
	switch {
	case score >= 88:
		band = "AL"
	case score >= 78:
		band = "IH"
	case score >= 70:
		band = "IM3"
	case score >= 65:
		band = "IM2"
	case score >= 52:
		band = "IM1"
	case score >= 45:
		band = "IL"
	case score >= 37:
		band = "NH"
	case score >= 28:
		band = "NM"
	}
	partial := len(session.Answers) < len(session.Questions)
	summary := fmt.Sprintf("This estimate is based on %d completed responses and considers task completion, detail, organization, vocabulary, grammar, and response length.", len(session.Answers))
	strengths := []string{
		"Responses generally remain connected to the assigned speaking task.",
		"Personal experiences are used to support the main message.",
		"The completed responses show a consistent attempt to develop ideas.",
	}
	weaknesses := []string{
		"Some responses may not include enough concrete supporting detail.",
		"Organization can weaken when transitions between ideas are limited.",
		"Vocabulary and sentence patterns may become repetitive under time pressure.",
	}
	priorities := []string{
		"Use a clear structure: background, specific example, reaction, and result.",
		"Add at least one concrete detail and one reason to every response.",
		"Practice flexible connectors and replace repeated words with natural alternatives.",
	}
	if !partial {
		summary = "This estimate reflects all completed questions and considers task completion, detail, organization, vocabulary, grammar, and response length."
		strengths[2] = "The full mock interview was completed across multiple OPIc task types."
	}
	predictions := buildGradePredictions(score)
	target := normalizeTargetGrade(session.Config.Difficulty)
	targetProbability := gradeProbability(predictions, target)
	targetStatus := likelihoodStatus(targetProbability)
	return ExamReport{
		SessionID: session.ID, Config: session.Config, Answers: session.Answers,
		AnsweredCount: len(session.Answers), TotalCount: len(session.Questions), Partial: partial, GeneratedAt: time.Now(),
		Overall: OverallReport{
			Score: score, EstimatedBand: band, Summary: summary,
			Strengths: strengths, Weaknesses: weaknesses, Priorities: priorities,
			TargetGrade: target, TargetStatus: targetStatus, TargetProbability: targetProbability,
			GradePredictions: predictions,
		},
	}
}

func buildGradePredictions(score int) []GradePrediction {
	levels := []struct {
		grade       string
		threshold   int
		description string
	}{
		{"AL", 88, "Sustained, well-organized narration and discussion of complex topics."},
		{"IH", 78, "Detailed connected speech with effective handling of most situations."},
		{"IM3", 70, "Consistent paragraph-length responses with supporting details."},
		{"IM2", 65, "Generally connected responses with some detail and control."},
		{"IM1", 52, "Simple connected responses on familiar topics."},
		{"IL", 45, "Short sentence-level responses with limited development."},
		{"NH", 37, "Basic sentences and phrases for familiar situations."},
		{"NM", 28, "Memorized phrases and simple personal information."},
		{"NL", 0, "Isolated words and highly memorized expressions."},
	}
	result := make([]GradePrediction, 0, len(levels))
	for _, level := range levels {
		probability := 50 + (score-level.threshold)*5
		if level.grade == "NL" {
			probability = 100
		}
		if probability < 2 {
			probability = 2
		}
		if probability > 98 {
			probability = 98
		}
		result = append(result, GradePrediction{
			Grade: level.grade, Probability: probability,
			Status: likelihoodStatus(probability), Description: level.description,
		})
	}
	return result
}

func normalizeTargetGrade(target string) string {
	target = strings.ToUpper(strings.TrimSpace(target))
	valid := map[string]bool{"AL": true, "IH": true, "IM3": true, "IM2": true, "IM1": true, "IL": true, "NH": true, "NM": true, "NL": true}
	if valid[target] {
		return target
	}
	return "IM2"
}

func gradeProbability(predictions []GradePrediction, grade string) int {
	for _, prediction := range predictions {
		if prediction.Grade == grade {
			return prediction.Probability
		}
	}
	return 0
}

func likelihoodStatus(probability int) string {
	switch {
	case probability >= 70:
		return "Likely"
	case probability >= 35:
		return "Possible"
	default:
		return "Unlikely"
	}
}

func demoTranscript(question Question) string {
	return "I would like to talk about " + question.Topic + ". I have been interested in it for several years because it gives me a chance to relax and spend meaningful time with people close to me. For example, last weekend I made a detailed plan and tried something new. At first, there was a small problem, but I changed the plan and everything worked out well. I felt proud and satisfied, so it became a memorable experience for me."
}

func newID() string {
	b := make([]byte, 12)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
