package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
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
	mu       sync.RWMutex
	apiKey   string
	settings AppSettings
	sessions map[string]*ExamSession
	client   *http.Client
}

func NewAppService() *AppService {
	key := strings.TrimSpace(os.Getenv("OPENAI_API_KEY"))
	return &AppService{
		apiKey: key,
		settings: AppSettings{
			HasAPIKey:       key != "",
			DemoMode:        key == "",
			EvaluationModel: "gpt-5-mini",
			TranscribeModel: "gpt-4o-transcribe",
			RealtimeModel:   "gpt-realtime",
		},
		sessions: make(map[string]*ExamSession),
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
	if request.EvaluationModel != "" {
		s.settings.EvaluationModel = request.EvaluationModel
	}
	if request.TranscribeModel != "" {
		s.settings.TranscribeModel = request.TranscribeModel
	}
	if request.RealtimeModel != "" {
		s.settings.RealtimeModel = request.RealtimeModel
	}
	s.settings.DemoMode = request.DemoMode
	s.settings.HasAPIKey = s.apiKey != ""
	return s.settings, nil
}

func (s *AppService) TestConnection() error {
	s.mu.RLock()
	key := s.apiKey
	s.mu.RUnlock()
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
			transcript, err = s.transcribe(request.AudioBase64, request.AudioMime)
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
		evaluation, err = s.evaluateWithOpenAI(session, question, transcript, request.DurationSec)
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

func (s *AppService) transcribe(encoded, mimeType string) (string, error) {
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
Do not inflate the score. Return concise Korean coaching feedback. Keywords must be useful English words or phrases from the answer.
The next question is already controlled by the application, so do not generate one.

INPUT:
` + string(inputJSON)

	schema := map[string]any{
		"type": "object", "additionalProperties": false,
		"properties": map[string]any{
			"score": map[string]any{"type": "integer", "minimum": 0, "maximum": 100},
			"keywords": map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "maxItems": 8},
			"strengths": map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "maxItems": 4},
			"improvements": map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "maxItems": 4},
			"feedback": map[string]any{"type": "string"},
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
	strengths := []string{"질문의 핵심 주제를 벗어나지 않았습니다.", "개인 경험을 중심으로 답변을 구성했습니다."}
	improvements := []string{}
	if len(words) < 60 {
		improvements = append(improvements, "구체적인 장소·인물·감정 묘사를 추가해 답변을 60단어 이상으로 확장하세요.")
	}
	if len(used) < 2 {
		improvements = append(improvements, "because, for example, however 같은 연결 표현으로 문장 관계를 분명히 하세요.")
	}
	if duration < 40 {
		improvements = append(improvements, "한 문항에 40~70초 정도 말할 수 있도록 이유와 사례를 하나씩 더하세요.")
	}
	if len(improvements) == 0 {
		improvements = append(improvements, "같은 의미의 단어 반복을 줄이고 더 자연스러운 구어 표현을 사용하세요.")
	}
	return AnswerEvaluation{
		Score: score, Keywords: keywords, Strengths: strengths, Improvements: improvements,
		Feedback: "핵심 답변은 전달되었습니다. 배경 설명 → 구체적 사건 → 느낌과 결과 순서로 확장하면 더 높은 등급에 가까워집니다.",
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
	type pair struct{ word string; count int }
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
	band := "NL–NM"
	switch {
	case score >= 88:
		band = "AL 가능권"
	case score >= 78:
		band = "IH 가능권"
	case score >= 65:
		band = "IM2–IM3 가능권"
	case score >= 52:
		band = "IM1 가능권"
	}
	return ExamReport{
		SessionID: session.ID, Config: session.Config, Answers: session.Answers, GeneratedAt: time.Now(),
		Overall: OverallReport{
			Score: score, EstimatedBand: band,
			Summary: "전 문항의 과업 수행, 구체성, 구성, 어휘·문법, 답변 길이를 종합한 연습용 추정치입니다.",
			Strengths: []string{"개인 경험 중심의 응답", "질문별 핵심 과업 수행", "시험 전 범위 완주"},
			Priorities: []string{"답변마다 구체적 사례 1개 추가", "연결어로 이야기 구조 강화", "반복 어휘를 자연스러운 동의어로 교체"},
		},
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

