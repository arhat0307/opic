export interface AppSettings {
  hasApiKey: boolean;
  demoMode: boolean;
  evaluationModel: string;
  transcribeModel: string;
  realtimeModel: string;
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
    priorities: string[];
  };
  answers: AnswerRecord[];
  generatedAt: string;
}

