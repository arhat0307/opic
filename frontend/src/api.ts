import { AppService } from "../bindings/opiccoach";
import type { AppSettings, ExamReport, Evaluation, Question } from "./types";

export const api = {
  getSettings: () => AppService.GetSettings() as Promise<AppSettings>,
  configure: (settings: Record<string, unknown>) =>
    AppService.Configure(settings as never) as Promise<AppSettings>,
  testConnection: () => AppService.TestConnection() as Promise<void>,
  startSession: (config: Record<string, unknown>) =>
    AppService.StartSession(config as never) as Promise<{
      sessionId: string;
      question: Question;
      totalCount: number;
    }>,
  submitAnswer: (answer: Record<string, unknown>) =>
    AppService.SubmitAnswer(answer as never) as Promise<{
      transcript: string;
      evaluation: Evaluation;
      next?: Question;
      completed: boolean;
      progress: number;
    }>,
  getReport: (sessionId: string) => AppService.GetReport(sessionId) as Promise<ExamReport>
};
