import { AppService } from "../bindings/opiccoach";
import type { AppSettings, ExamReport, Evaluation, Question } from "./types";

type WailsWindow = Window & {
  chrome?: { webview?: { postMessage?: unknown } };
  webkit?: { messageHandlers?: { external?: { postMessage?: unknown } } };
  wails?: { invoke?: unknown; invokeAsync?: unknown };
};

const isWailsRuntime = () => {
  if (typeof window === "undefined") return false;
  const runtimeWindow = window as WailsWindow;
  return Boolean(
    runtimeWindow.chrome?.webview?.postMessage ||
    runtimeWindow.webkit?.messageHandlers?.external?.postMessage ||
    runtimeWindow.wails?.invoke ||
    runtimeWindow.wails?.invokeAsync
  );
};

async function request<T>(path: string, init?: RequestInit): Promise<T> {
  const response = await fetch(path, {
    ...init,
    headers: {
      "Content-Type": "application/json",
      ...init?.headers
    }
  });
  if (!response.ok) {
    throw new Error(await response.text());
  }
  return response.json() as Promise<T>;
}

export const api = {
  getSettings: () => isWailsRuntime()
    ? AppService.GetSettings() as Promise<AppSettings>
    : request<AppSettings>("/api/settings"),
  configure: (settings: Record<string, unknown>) =>
    isWailsRuntime()
      ? AppService.Configure(settings as never) as Promise<AppSettings>
      : request<AppSettings>("/api/configure", { method: "POST", body: JSON.stringify(settings) }),
  testConnection: () => isWailsRuntime()
    ? AppService.TestConnection() as Promise<void>
    : request<void>("/api/test-connection", { method: "POST", body: "{}" }),
  generateSpeech: (text: string) =>
    isWailsRuntime()
      ? AppService.GenerateSpeech(text) as Promise<{ audioBase64: string; mimeType: string }>
      : request<{ audioBase64: string; mimeType: string }>("/api/speech", { method: "POST", body: JSON.stringify({ text }) }),
  startSession: (config: Record<string, unknown>) =>
    isWailsRuntime()
      ? AppService.StartSession(config as never) as Promise<{
        sessionId: string;
        question: Question;
        totalCount: number;
      }>
      : request<{
        sessionId: string;
        question: Question;
        totalCount: number;
      }>("/api/sessions", { method: "POST", body: JSON.stringify(config) }),
  submitAnswer: (answer: Record<string, unknown>) =>
    isWailsRuntime()
      ? AppService.SubmitAnswer(answer as never) as Promise<{
        transcript: string;
        evaluation: Evaluation;
        next?: Question;
        completed: boolean;
        progress: number;
      }>
      : request<{
        transcript: string;
        evaluation: Evaluation;
        next?: Question;
        completed: boolean;
        progress: number;
      }>("/api/answers", { method: "POST", body: JSON.stringify(answer) }),
  finalizeSession: (sessionId: string) =>
    isWailsRuntime()
      ? AppService.FinalizeSession(sessionId) as Promise<ExamReport>
      : request<ExamReport>(`/api/sessions/${sessionId}/finalize`, { method: "POST", body: "{}" }),
  getReport: (sessionId: string) => isWailsRuntime()
    ? AppService.GetReport(sessionId) as Promise<ExamReport>
    : request<ExamReport>(`/api/sessions/${sessionId}/report`)
};