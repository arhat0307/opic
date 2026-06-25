export interface AppSettings {
  provider: "openai" | "gemini";
  hasApiKey: boolean;
  hasOpenAIKey: boolean;
  hasGeminiKey: boolean;
  demoMode: boolean;
  evaluationModel: string;
  transcribeModel: string;
  realtimeModel: string;
  speechModel: string;
  speechVoice: string;
  geminiEvaluationModels: string[];
  geminiTranscriptionModels: string[];
  geminiSpeechModels: string[];
  geminiSpeechVoice: string;
}

export interface Question {
  index: number;
  category: string;
  topic: string;
  text: string;
  intent: string;
}

export interface Evaluation {
  score: number;
  keywords: string[];
  strengths: string[];
  improvements: string[];
  feedback: string;
}

export interface AnswerRecord {
  question: Question;
  transcript: string;
  durationSec: number;
  evaluation: Evaluation;
}

export interface ExamReport {
  sessionId: string;
  config: { difficulty: string; topics: string[]; language: string };
  overall: {
    score: number;
    estimatedBand: string;
    summary: string;
    strengths: string[];
    weaknesses: string[];
    priorities: string[];
    targetGrade: string;
    targetStatus: string;
    targetProbability: number;
    gradePredictions: {
      grade: string;
      probability: number;
      status: string;
      description: string;
    }[];
  };
  answers: AnswerRecord[];
  answeredCount: number;
  totalCount: number;
  partial: boolean;
  generatedAt: string;
}
