<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from "vue";
import { api } from "./api";
import type { AnswerRecord, AppSettings, Evaluation, ExamReport, Question } from "./types";

type Screen = "setup" | "exam" | "report";

const difficulties = ["IM1", "IM2", "IM3", "IH", "AL"];
const topics = ["집", "가족", "직장", "학교", "영화", "음악", "운동", "여행", "카페", "요리", "게임", "쇼핑", "공원"];
const topicIcons: Record<string, string> = {
  집: "⌂", 가족: "♧", 직장: "▣", 학교: "◆", 영화: "▶", 음악: "♪", 운동: "●",
  여행: "✈", 카페: "☕", 요리: "♨", 게임: "✦", 쇼핑: "▤", 공원: "♿"
};
const screen = ref<Screen>("setup");
const selectedDifficulty = ref("IM2");
const selectedTopics = ref<string[]>(["영화", "여행", "카페"]);
const settingsOpen = ref(false);
const settings = ref<AppSettings>({
  provider: "openai",
  hasApiKey: false,
  hasOpenAIKey: false,
  hasGeminiKey: false,
  demoMode: true,
  evaluationModel: "gpt-5-mini",
  transcribeModel: "gpt-4o-transcribe",
  realtimeModel: "gpt-realtime",
  speechModel: "gpt-4o-mini-tts",
  speechVoice: "marin",
  geminiEvaluationModels: ["gemini-3.5-flash", "gemini-2.5-flash", "gemini-2.5-flash-lite"],
  geminiTranscriptionModels: ["gemini-3.5-flash", "gemini-2.5-flash", "gemini-2.5-flash-lite"],
  geminiSpeechModels: ["gemini-3.1-flash-tts-preview", "gemini-2.5-flash-preview-tts"],
  geminiSpeechVoice: "Kore"
});
const openAIAPIKey = ref("");
const geminiAPIKey = ref("");
const connectionMessage = ref("");
const busy = ref(false);
const error = ref("");

const sessionId = ref("");
const currentQuestion = ref<Question | null>(null);
const totalCount = ref(15);
const completedCount = ref(0);
const transcript = ref("");
const latestEvaluation = ref<Evaluation | null>(null);
const answeredRecords = ref<AnswerRecord[]>([]);
const pendingNextQuestion = ref<Question | null>(null);
const report = ref<ExamReport | null>(null);
const recording = ref(false);
const hasRecording = ref(false);
const autoSubmitting = ref(false);
const speaking = ref(false);
const elapsed = ref(0);
let timer: number | undefined;
let recorder: MediaRecorder | null = null;
let stream: MediaStream | null = null;
let chunks: Blob[] = [];
let recordedBlob: Blob | null = null;
let stopRecordingPromise: Promise<Blob | null> | null = null;
let resolveStopRecording: ((blob: Blob | null) => void) | null = null;
let englishVoice: SpeechSynthesisVoice | null = null;
let questionAudio: HTMLAudioElement | null = null;
let questionAudioURL = "";

const progress = computed(() => Math.round((completedCount.value / totalCount.value) * 100));
const topicLabel = computed(() => selectedTopics.value.join(" · "));
const currentAverage = computed(() => {
  if (!answeredRecords.value.length) return 0;
  const total = answeredRecords.value.reduce((sum, answer) => sum + answer.evaluation.score, 0);
  return Math.round(total / answeredRecords.value.length);
});
const canRestartRecording = computed(() =>
  (recording.value || hasRecording.value) && elapsed.value <= 20 && !busy.value
);
const remainingSeconds = computed(() => Math.max(0, 120 - elapsed.value));
const viewedAnswer = computed(() =>
  answeredRecords.value.find((answer) => answer.question.index === currentQuestion.value?.index) ?? null
);
const isReviewingAnswer = computed(() => viewedAnswer.value !== null);
const canGoPrevious = computed(() =>
  Boolean(currentQuestion.value && currentQuestion.value.index > 1 &&
    answeredRecords.value.some((answer) => answer.question.index === currentQuestion.value!.index - 1))
);
const canGoNext = computed(() => {
  if (!currentQuestion.value) return false;
  const nextIndex = currentQuestion.value.index + 1;
  return answeredRecords.value.some((answer) => answer.question.index === nextIndex) ||
    pendingNextQuestion.value?.index === nextIndex;
});

onMounted(async () => {
  await loadEnglishVoice();
  try {
    settings.value = await api.getSettings();
  } catch {
    // The UI can still be previewed in a normal browser without the Wails runtime.
  }
});

onBeforeUnmount(() => {
  if (recording.value && recorder?.state !== "inactive") recorder?.stop();
  stopTracks();
  if (timer) window.clearInterval(timer);
});

function toggleTopic(topic: string) {
  const index = selectedTopics.value.indexOf(topic);
  if (index >= 0) selectedTopics.value.splice(index, 1);
  else if (selectedTopics.value.length < 5) selectedTopics.value.push(topic);
}

async function saveSettings() {
  busy.value = true;
  error.value = "";
  try {
    settings.value = await api.configure({
      provider: settings.value.provider,
      apiKey: openAIAPIKey.value,
      geminiApiKey: geminiAPIKey.value,
      demoMode: settings.value.demoMode,
      evaluationModel: settings.value.evaluationModel,
      transcribeModel: settings.value.transcribeModel,
      realtimeModel: settings.value.realtimeModel,
      speechModel: settings.value.speechModel,
      speechVoice: settings.value.speechVoice,
      geminiEvaluationModels: settings.value.geminiEvaluationModels,
      geminiTranscriptionModels: settings.value.geminiTranscriptionModels,
      geminiSpeechModels: settings.value.geminiSpeechModels,
      geminiSpeechVoice: settings.value.geminiSpeechVoice
    });
    openAIAPIKey.value = "";
    geminiAPIKey.value = "";
    settingsOpen.value = false;
  } catch (e) {
    error.value = readableError(e);
  } finally {
    busy.value = false;
  }
}

async function testConnection() {
  connectionMessage.value = "연결 확인 중…";
  try {
    if (openAIAPIKey.value || geminiAPIKey.value) {
      settings.value = await api.configure({
        provider: settings.value.provider,
        apiKey: openAIAPIKey.value,
        geminiApiKey: geminiAPIKey.value,
        demoMode: false,
        evaluationModel: settings.value.evaluationModel,
        transcribeModel: settings.value.transcribeModel,
        realtimeModel: settings.value.realtimeModel,
        speechModel: settings.value.speechModel,
        speechVoice: settings.value.speechVoice,
        geminiEvaluationModels: settings.value.geminiEvaluationModels,
        geminiTranscriptionModels: settings.value.geminiTranscriptionModels,
        geminiSpeechModels: settings.value.geminiSpeechModels,
        geminiSpeechVoice: settings.value.geminiSpeechVoice
      });
    }
    await api.testConnection();
    connectionMessage.value = "OpenAI API 연결 성공";
  } catch (e) {
    connectionMessage.value = readableError(e);
  }
}

async function startExam() {
  if (!selectedTopics.value.length) {
    error.value = "주제를 하나 이상 선택하세요.";
    return;
  }
  busy.value = true;
  error.value = "";
  try {
    const result = await api.startSession({
      difficulty: selectedDifficulty.value,
      topics: selectedTopics.value,
      language: "en"
    });
    sessionId.value = result.sessionId;
    currentQuestion.value = result.question;
    totalCount.value = result.totalCount;
    completedCount.value = 0;
    answeredRecords.value = [];
    pendingNextQuestion.value = null;
    transcript.value = "";
    latestEvaluation.value = null;
    screen.value = "exam";
    window.setTimeout(speakQuestion, 450);
  } catch (e) {
    error.value = readableError(e);
  } finally {
    busy.value = false;
  }
}

async function loadEnglishVoice(): Promise<SpeechSynthesisVoice | null> {
  if (!("speechSynthesis" in window)) return null;

  let voices = speechSynthesis.getVoices();
  if (!voices.length) {
    voices = await new Promise<SpeechSynthesisVoice[]>((resolve) => {
      const timeout = window.setTimeout(() => resolve(speechSynthesis.getVoices()), 1200);
      speechSynthesis.addEventListener("voiceschanged", () => {
        window.clearTimeout(timeout);
        resolve(speechSynthesis.getVoices());
      }, { once: true });
    });
  }

  const englishVoices = voices.filter((voice) => /^en[-_]/i.test(voice.lang));
  const preference = [
    /Microsoft.*Online.*Natural.*English.*United States/i,
    /Microsoft (Ava|Aria|Jenny|Emma|Andrew|Brian).*Natural/i,
    /Microsoft (Ava|Aria|Jenny|Emma|Andrew|Brian)/i,
    /Google US English/i,
    /Samantha/i,
    /English.*United States/i
  ];

  englishVoice =
    preference
      .map((pattern) => englishVoices.find((voice) => pattern.test(`${voice.name} ${voice.lang}`)))
      .find(Boolean) ??
    englishVoices.find((voice) => /^en-US$/i.test(voice.lang) && voice.localService) ??
    englishVoices.find((voice) => /^en-US$/i.test(voice.lang)) ??
    englishVoices[0] ??
    null;

  return englishVoice;
}

async function speakQuestion() {
  if (!currentQuestion.value) return;
  stopQuestionAudio();
  speechSynthesis.cancel();

  if (settings.value.hasApiKey && !settings.value.demoMode) {
    speaking.value = true;
    try {
      const result = await api.generateSpeech(currentQuestion.value.text);
      const bytes = Uint8Array.from(atob(result.audioBase64), (char) => char.charCodeAt(0));
      const blob = new Blob([bytes], { type: result.mimeType });
      questionAudioURL = URL.createObjectURL(blob);
      questionAudio = new Audio(questionAudioURL);
      questionAudio.onended = () => {
        stopQuestionAudio();
        void startRecording();
      };
      questionAudio.onerror = stopQuestionAudio;
      await questionAudio.play();
      return;
    } catch (e) {
      error.value = `${readableError(e)} 시스템 영어 음성으로 재생합니다.`;
      stopQuestionAudio();
    }
  }

  if (!("speechSynthesis" in window)) return;
  const voice = englishVoice ?? await loadEnglishVoice();
  const naturalEnglish = currentQuestion.value.text
    .replace(/\s+/g, " ")
    .replace(/\s+([,.?!])/g, "$1")
    .trim();
  const utterance = new SpeechSynthesisUtterance(naturalEnglish);
  utterance.lang = "en-US";
  if (voice) utterance.voice = voice;
  utterance.rate = 0.88;
  utterance.pitch = 0.98;
  utterance.volume = 1;
  utterance.onend = () => {
    speaking.value = false;
    void startRecording();
  };
  utterance.onerror = () => { speaking.value = false; };
  speaking.value = true;
  speechSynthesis.speak(utterance);
}

function stopQuestionAudio() {
  if (questionAudio) {
    questionAudio.pause();
    questionAudio.onended = null;
    questionAudio.onerror = null;
    questionAudio = null;
  }
  if (questionAudioURL) {
    URL.revokeObjectURL(questionAudioURL);
    questionAudioURL = "";
  }
  speaking.value = false;
}

async function startRecording() {
  if (recording.value || hasRecording.value || busy.value || isReviewingAnswer.value || !currentQuestion.value) return;
  error.value = "";
  try {
    stream = await navigator.mediaDevices.getUserMedia({ audio: true });
    const preferred = MediaRecorder.isTypeSupported("audio/webm;codecs=opus")
      ? "audio/webm;codecs=opus"
      : "audio/webm";
    recorder = new MediaRecorder(stream, { mimeType: preferred });
    chunks = [];
    recordedBlob = null;
    hasRecording.value = false;
    stopRecordingPromise = new Promise<Blob | null>((resolve) => {
      resolveStopRecording = resolve;
    });
    recorder.ondataavailable = (event) => {
      if (event.data.size) chunks.push(event.data);
    };
    recorder.onstop = () => {
      recordedBlob = chunks.length
        ? new Blob(chunks, { type: recorder?.mimeType || preferred })
        : null;
      hasRecording.value = Boolean(recordedBlob?.size);
      recording.value = false;
      if (timer) window.clearInterval(timer);
      timer = undefined;
      stopTracks();
      resolveStopRecording?.(recordedBlob);
      resolveStopRecording = null;
    };
    recorder.start(250);
    elapsed.value = 0;
    recording.value = true;
    timer = window.setInterval(() => {
      elapsed.value += 1;
      if (elapsed.value >= 120) {
        window.clearInterval(timer);
        timer = undefined;
        void forceSubmitAtLimit();
      }
    }, 1000);
  } catch {
    error.value = "마이크 권한을 확인하세요. 텍스트 답변으로도 진행할 수 있습니다.";
  }
}

async function stopRecording(): Promise<Blob | null> {
  if (!recording.value || !recorder || recorder.state === "inactive") {
    return recordedBlob;
  }
  const pending = stopRecordingPromise;
  recorder.stop();
  return pending ?? recordedBlob;
}

async function toggleRecording() {
  if (recording.value) {
    await stopRecording();
    return;
  }
  if (hasRecording.value) return;
  await startRecording();
}

async function restartRecording() {
  if (!canRestartRecording.value) return;
  await stopRecording();
  chunks = [];
  recordedBlob = null;
  hasRecording.value = false;
  elapsed.value = 0;
  await startRecording();
}

async function forceSubmitAtLimit() {
  if (autoSubmitting.value || busy.value || !recording.value) return;
  autoSubmitting.value = true;
  try {
    await stopRecording();
    await submitAnswer(true);
  } finally {
    autoSubmitting.value = false;
  }
}

function stopTracks() {
  stream?.getTracks().forEach((track) => track.stop());
  stream = null;
}

async function submitAnswer(forced = false) {
  if (!currentQuestion.value || busy.value || isReviewingAnswer.value) return;
  const blob = recording.value ? await stopRecording() : recordedBlob;
  if (!forced && !transcript.value.trim() && !blob) return;
  busy.value = true;
  error.value = "";
  try {
    const audioBase64 = blob ? await blobToBase64(blob) : "";
    const response = await api.submitAnswer({
      sessionId: sessionId.value,
      questionIdx: currentQuestion.value.index - 1,
      audioBase64,
      audioMime: blob?.type || "",
      transcript: transcript.value,
      durationSec: elapsed.value
    });
    transcript.value = response.transcript;
    latestEvaluation.value = response.evaluation;
    completedCount.value = response.progress;
    answeredRecords.value.push({
      question: currentQuestion.value,
      transcript: response.transcript,
      durationSec: elapsed.value,
      evaluation: response.evaluation
    });
    pendingNextQuestion.value = response.next || null;
    chunks = [];
    recordedBlob = null;
    hasRecording.value = false;
  } catch (e) {
    error.value = readableError(e);
  } finally {
    busy.value = false;
  }
}

function showQuestion(question: Question, answer?: AnswerRecord) {
  stopQuestionAudio();
  speechSynthesis?.cancel();
  currentQuestion.value = question;
  transcript.value = answer?.transcript ?? "";
  latestEvaluation.value = answer?.evaluation ?? null;
  elapsed.value = answer?.durationSec ?? 0;
  chunks = [];
  recordedBlob = null;
  hasRecording.value = false;
}

function goToPreviousQuestion() {
  if (!currentQuestion.value) return;
  const answer = answeredRecords.value.find(
    (item) => item.question.index === currentQuestion.value!.index - 1
  );
  if (answer) showQuestion(answer.question, answer);
}

function goToNextQuestion() {
  if (!currentQuestion.value) return;
  const nextIndex = currentQuestion.value.index + 1;
  const answered = answeredRecords.value.find((item) => item.question.index === nextIndex);
  if (answered) {
    showQuestion(answered.question, answered);
    return;
  }
  if (pendingNextQuestion.value?.index === nextIndex) {
    const next = pendingNextQuestion.value;
    showQuestion(next);
    window.setTimeout(speakQuestion, 350);
  }
}

function goToAnsweredQuestion(answer: AnswerRecord) {
  showQuestion(answer.question, answer);
}

async function finalizeExam() {
  if (!sessionId.value || completedCount.value === 0 || busy.value) return;
  busy.value = true;
  error.value = "";
  try {
    stopQuestionAudio();
    speechSynthesis?.cancel();
    report.value = await api.finalizeSession(sessionId.value);
    screen.value = "report";
  } catch (e) {
    error.value = readableError(e);
  } finally {
    busy.value = false;
  }
}

function resetExam() {
  stopQuestionAudio();
  speechSynthesis?.cancel();
  screen.value = "setup";
  report.value = null;
  currentQuestion.value = null;
  latestEvaluation.value = null;
  answeredRecords.value = [];
  pendingNextQuestion.value = null;
  transcript.value = "";
  completedCount.value = 0;
  recordedBlob = null;
  hasRecording.value = false;
}

function printReport() {
  window.print();
}

function blobToBase64(blob: Blob): Promise<string> {
  return new Promise((resolve, reject) => {
    const reader = new FileReader();
    reader.onloadend = () => resolve(String(reader.result).split(",")[1] || "");
    reader.onerror = reject;
    reader.readAsDataURL(blob);
  });
}

function readableError(value: unknown) {
  if (value instanceof Error) return value.message;
  return String(value).replace(/^.*?: /, "");
}

function updateGeminiModels(
  field: "geminiEvaluationModels" | "geminiTranscriptionModels" | "geminiSpeechModels",
  event: Event
) {
  settings.value[field] = (event.target as HTMLInputElement).value
    .split(/[\n,]/)
    .map((model) => model.trim())
    .filter(Boolean);
}
</script>

<template>
  <div class="app-shell">
    <header class="topbar">
      <button class="brand" @click="resetExam">
        <span class="brand-mark">O</span>
        <span><strong>OPIc Flow</strong><small>AI SPEAKING LAB</small></span>
      </button>
      <div class="top-actions">
        <span class="connection-pill" :class="{ live: settings.hasApiKey && !settings.demoMode }">
          <i></i>{{ settings.demoMode ? "DEMO MODE" : `${settings.provider.toUpperCase()} CONNECTED` }}
        </span>
        <button class="icon-button" aria-label="설정" @click="settingsOpen = true">⚙</button>
      </div>
    </header>

    <main v-if="screen === 'setup'" class="setup-layout">
      <section class="hero">
        <p class="eyebrow">PERSONAL AI INTERVIEWER</p>
        <h1>말할수록,<br /><em>정확해지는</em> 영어.</h1>
        <p class="hero-copy">실제 OPIc 흐름으로 질문을 듣고 답하세요. AI가 매 문항을 분석하고 다음 연습 방향을 제시합니다.</p>
        <div class="hero-stats">
          <div><strong>15</strong><span>Questions</span></div>
          <div><strong>AI</strong><span>Live feedback</span></div>
          <div><strong>01</strong><span>Final report</span></div>
        </div>
      </section>

      <section class="setup-card">
        <div class="step-heading"><span>01</span><div><h2>목표 난이도</h2><p>현재 목표 등급을 선택하세요.</p></div></div>
        <div class="difficulty-grid">
          <button v-for="level in difficulties" :key="level" :class="{ selected: selectedDifficulty === level }" @click="selectedDifficulty = level">
            <strong>{{ level }}</strong><small>{{ level === "AL" ? "Advanced" : "Intermediate" }}</small>
          </button>
        </div>

        <div class="divider"></div>
        <div class="step-heading"><span>02</span><div><h2>설문 주제</h2><p>자신 있게 말할 수 있는 주제를 최대 5개 고르세요.</p></div><b>{{ selectedTopics.length }}/5</b></div>
        <div class="topic-grid">
          <button v-for="topic in topics" :key="topic" :class="{ selected: selectedTopics.includes(topic) }" @click="toggleTopic(topic)">
            <span>{{ topicIcons[topic] }}</span>{{ topic }}
          </button>
        </div>

        <div class="session-summary">
          <div><small>YOUR SESSION</small><strong>{{ selectedDifficulty }} · {{ topicLabel }}</strong></div>
          <button class="primary" :disabled="busy" @click="startExam">{{ busy ? "준비 중…" : "연습 시작" }} <span>→</span></button>
        </div>
        <p v-if="error" class="error">{{ error }}</p>
      </section>
    </main>

    <main v-else-if="screen === 'exam'" class="exam-layout">
      <aside class="exam-sidebar">
        <p class="eyebrow">MOCK INTERVIEW</p>
        <h2>{{ selectedDifficulty }} Session</h2>
        <div class="progress-ring" :style="{ '--progress': `${progress * 3.6}deg` }"><span><strong>{{ completedCount }}</strong>/ {{ totalCount }}</span></div>
        <div class="progress-meta"><span>PROGRESS</span><b>{{ progress }}%</b></div>
        <div class="progress-bar"><i :style="{ width: `${progress}%` }"></i></div>
        <div v-if="answeredRecords.length" class="current-average">
          <div><small>CURRENT AVERAGE</small><strong>{{ currentAverage }}</strong><span>/ 100</span></div>
          <p>{{ completedCount }}개 문항 기준 평균</p>
        </div>
        <div v-if="answeredRecords.length" class="question-history">
          <small>ANSWER HISTORY</small>
          <div>
            <button
              v-for="answer in answeredRecords"
              :key="answer.question.index"
              :class="{ active: currentQuestion?.index === answer.question.index }"
              @click="goToAnsweredQuestion(answer)"
            >
              <span class="history-number">Q{{ answer.question.index.toString().padStart(2, "0") }}</span>
              <span class="history-category">{{ answer.question.category }}</span>
              <strong>{{ answer.evaluation.score }}</strong>
            </button>
          </div>
        </div>
        <div class="exam-tip"><small>COACH'S NOTE</small><p>완벽한 문법보다 구체적인 경험과 자연스러운 이야기 흐름에 집중하세요.</p></div>
        <button class="finish-early" :disabled="busy || completedCount === 0" @click="finalizeExam">
          {{ completedCount === totalCount ? "최종 결과 보기" : "현재까지 최종 제출" }}
        </button>
      </aside>

      <section class="conversation">
        <div class="question-navigation">
          <button class="secondary nav-button" :disabled="!canGoPrevious" @click="goToPreviousQuestion">← 이전 문제</button>
          <span v-if="isReviewingAnswer">제출 완료 · 평가 다시 보기</span>
          <span v-else>답변 대기 중</span>
          <button class="secondary nav-button" :disabled="!canGoNext" @click="goToNextQuestion">다음 문제 →</button>
        </div>
        <div class="question-meta">
          <span>QUESTION {{ currentQuestion?.index?.toString().padStart(2, "0") }}</span>
          <b>{{ currentQuestion?.category }}</b>
          <span>{{ currentQuestion?.topic }}</span>
        </div>
        <div class="question-card">
          <button class="speaker" :class="{ active: speaking }" :disabled="speaking || recording" @click="speakQuestion">{{ speaking ? "•••" : "◖))" }}</button>
          <div><small>AI INTERVIEWER · AI-GENERATED VOICE</small><h1>{{ currentQuestion?.text }}</h1><p>{{ currentQuestion?.intent }}</p></div>
        </div>

        <div v-if="!isReviewingAnswer" class="answer-area">
          <div class="record-row">
            <button class="record-button" :class="{ active: recording }" :disabled="hasRecording && !recording" @click="toggleRecording">
              <i></i>{{ recording ? "녹음 중지" : hasRecording ? "녹음 완료" : "답변 녹음" }}
            </button>
            <button v-if="canRestartRecording" class="restart-button" @click="restartRecording">↻ 처음부터 다시</button>
            <span class="timer" :class="{ warning: remainingSeconds <= 20 }">
              {{ Math.floor(elapsed / 60).toString().padStart(2, "0") }}:{{ (elapsed % 60).toString().padStart(2, "0") }}
              <small>/ 02:00</small>
            </span>
            <span class="audio-bars" :class="{ active: recording }"><i v-for="n in 18" :key="n"></i></span>
          </div>
          <div class="recording-guide">
            <span v-if="speaking">질문 재생이 끝나면 녹음이 자동 시작됩니다.</span>
            <span v-else-if="recording && elapsed <= 20">자동 녹음 중 · {{ 20 - elapsed }}초 동안 다시 시작 가능</span>
            <span v-else-if="recording">자동 녹음 중 · {{ remainingSeconds }}초 후 자동 제출</span>
            <span v-else-if="hasRecording">녹음 완료 · 답변을 제출하거나 다시 녹음할 수 있습니다.</span>
            <span v-else>질문을 다시 들으면 종료 후 녹음이 자동 시작됩니다.</span>
          </div>
          <label for="transcript">답변 스크립트 <small>녹음만 제출하면 AI가 자동 전사합니다. 직접 입력하거나 수정할 수도 있습니다.</small></label>
          <textarea id="transcript" v-model="transcript" placeholder="Start speaking, or type your answer here…"></textarea>
          <div class="submit-row">
            <span>{{ transcript.trim().split(/\s+/).filter(Boolean).length }} words</span>
            <button class="primary" :disabled="busy || (!transcript.trim() && !hasRecording && !recording)" @click="submitAnswer(false)">
              {{ autoSubmitting ? "시간 종료 · 자동 제출 중…" : busy ? "AI 분석 중…" : "답변 제출" }} <span>→</span>
            </button>
          </div>
        </div>

        <div v-else class="submitted-answer">
          <small>YOUR ANSWER · {{ viewedAnswer?.durationSec }} SEC</small>
          <p>{{ viewedAnswer?.transcript }}</p>
        </div>

        <div v-if="latestEvaluation" class="evaluation-panel">
          <header>
            <div class="score">{{ latestEvaluation.score }}</div>
            <div><small>AI ANALYSIS COMPLETE</small><h2>답변 분석 결과</h2><p>{{ latestEvaluation.feedback }}</p></div>
          </header>
          <div class="keyword-row">
            <span v-for="keyword in latestEvaluation.keywords" :key="keyword">{{ keyword }}</span>
          </div>
          <div class="evaluation-grid">
            <article><small>잘한 점</small><ul><li v-for="item in latestEvaluation.strengths" :key="item">{{ item }}</li></ul></article>
            <article><small>개선할 점</small><ul><li v-for="item in latestEvaluation.improvements" :key="item">{{ item }}</li></ul></article>
          </div>
          <footer>
            <button class="secondary" :disabled="!canGoPrevious" @click="goToPreviousQuestion">← 이전 문제 보기</button>
            <button v-if="canGoNext" class="primary" @click="goToNextQuestion">다음 문제로 <span>→</span></button>
            <button v-else class="primary" :disabled="busy" @click="finalizeExam">
              {{ busy ? "총평 생성 중…" : "최종 결과 보기" }} <span>→</span>
            </button>
          </footer>
        </div>
        <p v-if="error" class="error">{{ error }}</p>
      </section>
    </main>

    <main v-else-if="report && screen === 'report'" class="report-layout">
      <section class="report-cover">
        <p class="eyebrow">SESSION COMPLETE</p>
        <h1>Speaking<br />Performance<br /><em>Report</em></h1>
        <p>{{ new Date(report.generatedAt).toLocaleDateString("ko-KR") }} · {{ report.config.difficulty }} · {{ report.config.topics.join(", ") }}</p>
        <p class="completion-label">{{ report.partial ? `조기 제출 · ${report.answeredCount}/${report.totalCount} 문항 완료` : `전체 ${report.totalCount} 문항 완료` }}</p>
        <div class="overall-score">
          <div><strong>{{ report.overall.score }}</strong><span>/ 100</span></div>
          <aside><small>ESTIMATED BAND</small><b>{{ report.overall.estimatedBand }}</b></aside>
        </div>
        <p class="report-summary">{{ report.overall.summary }}</p>
        <div class="target-result">
          <small>TARGET GRADE · {{ report.overall.targetGrade }}</small>
          <strong>{{ report.overall.targetStatus }}</strong>
          <span>{{ report.overall.targetProbability }}% estimated likelihood</span>
        </div>
        <div class="report-actions"><button class="secondary" @click="printReport">Print / Save as PDF</button><button class="primary" @click="resetExam">Start New Session →</button></div>
      </section>

      <section class="report-content">
        <div class="overview-grid">
          <article><small>STRENGTHS</small><ul><li v-for="item in report.overall.strengths" :key="item">{{ item }}</li></ul></article>
          <article><small>WEAKNESSES</small><ul><li v-for="item in report.overall.weaknesses" :key="item">{{ item }}</li></ul></article>
          <article class="accent"><small>PRIORITY IMPROVEMENTS</small><ul><li v-for="item in report.overall.priorities" :key="item">{{ item }}</li></ul></article>
        </div>
        <section class="grade-predictions">
          <header><div><small>OPIc GRADE FORECAST</small><h2>Estimated grade attainment</h2></div><p>Practice estimate, not an official OPIc score.</p></header>
          <div class="grade-table">
            <article v-for="grade in report.overall.gradePredictions" :key="grade.grade" :class="grade.status.toLowerCase()">
              <strong>{{ grade.grade }}</strong>
              <div><span>{{ grade.status }}</span><p>{{ grade.description }}</p></div>
              <aside><b>{{ grade.probability }}%</b><i><em :style="{ width: `${grade.probability}%` }"></em></i></aside>
            </article>
          </div>
        </section>
        <div class="section-heading">
          <small>QUESTION-BY-QUESTION REVIEW</small>
          <h2>Detailed response analysis</h2>
        </div>
        <div class="answer-list">
          <article v-for="answer in report.answers" :key="answer.question.index" class="answer-report">
            <header><span>Q{{ answer.question.index.toString().padStart(2, "0") }}</span><div><small>{{ answer.question.category }} · {{ answer.question.topic }}</small><h3>{{ answer.question.text }}</h3></div><strong>{{ answer.evaluation.score }}</strong></header>
            <blockquote>{{ answer.transcript }}</blockquote>
            <div class="keyword-row"><span v-for="keyword in answer.evaluation.keywords" :key="keyword">{{ keyword }}</span></div>
            <div class="feedback-grid">
              <div><small>STRENGTHS</small><p>{{ answer.evaluation.strengths.join(" ") }}</p></div>
              <div><small>AREAS TO IMPROVE</small><p>{{ answer.evaluation.improvements.join(" ") }}</p></div>
            </div>
          </article>
        </div>
      </section>
    </main>

    <div v-if="settingsOpen" class="modal-backdrop" @click.self="settingsOpen = false">
      <section class="settings-modal">
        <button class="modal-close" @click="settingsOpen = false">×</button>
        <p class="eyebrow">AI CONNECTION</p>
        <h2>AI 연결 설정</h2>
        <p class="modal-copy">OpenAI 또는 Google Gemini API를 선택할 수 있습니다. API 키는 현재 앱 프로세스 메모리에만 보관됩니다.</p>
        <div class="provider-tabs">
          <button :class="{ selected: settings.provider === 'openai' }" @click="settings.provider = 'openai'">OpenAI</button>
          <button :class="{ selected: settings.provider === 'gemini' }" @click="settings.provider = 'gemini'">Google Gemini</button>
        </div>
        <label v-if="settings.provider === 'openai'">OpenAI API key<input v-model="openAIAPIKey" type="password" placeholder="sk-…" autocomplete="off" /></label>
        <label v-else>Gemini API key<input v-model="geminiAPIKey" type="password" placeholder="AIza…" autocomplete="off" /></label>
        <label class="switch-row"><span><b>데모 모드</b><small>API 비용 없이 전체 흐름을 시험합니다.</small></span><input v-model="settings.demoMode" type="checkbox" /></label>
        <div v-if="settings.provider === 'openai'" class="model-grid">
          <label>평가 모델<input v-model="settings.evaluationModel" /></label>
          <label>전사 모델<input v-model="settings.transcribeModel" /></label>
          <label>Realtime 모델<input v-model="settings.realtimeModel" /></label>
          <label>TTS 모델<input v-model="settings.speechModel" /></label>
          <label>TTS 음성
            <select v-model="settings.speechVoice">
              <option value="marin">Marin — 자연스럽고 선명함</option>
              <option value="cedar">Cedar — 차분하고 안정적</option>
              <option value="coral">Coral — 밝고 친근함</option>
              <option value="sage">Sage — 중립적</option>
              <option value="onyx">Onyx — 낮고 묵직함</option>
            </select>
          </label>
        </div>
        <div v-else class="gemini-models">
          <p class="fallback-note">모델을 쉼표 또는 줄바꿈으로 입력하세요. 위에서부터 시도하고 실패하면 다음 모델로 자동 전환합니다.</p>
          <label>평가 모델 우선순위
            <textarea class="model-list" :value="settings.geminiEvaluationModels.join('\n')" @input="updateGeminiModels('geminiEvaluationModels', $event)"></textarea>
          </label>
          <label>음성 전사 모델 우선순위
            <textarea class="model-list" :value="settings.geminiTranscriptionModels.join('\n')" @input="updateGeminiModels('geminiTranscriptionModels', $event)"></textarea>
          </label>
          <label>TTS 모델 우선순위
            <textarea class="model-list" :value="settings.geminiSpeechModels.join('\n')" @input="updateGeminiModels('geminiSpeechModels', $event)"></textarea>
          </label>
          <label>Gemini TTS 음성
            <select v-model="settings.geminiSpeechVoice">
              <option value="Kore">Kore — 단정하고 힘 있음</option>
              <option value="Iapetus">Iapetus — 선명함</option>
              <option value="Schedar">Schedar — 균형 잡힘</option>
              <option value="Gacrux">Gacrux — 성숙함</option>
              <option value="Charon">Charon — 설명형</option>
              <option value="Aoede">Aoede — 부드러움</option>
            </select>
          </label>
        </div>
        <p v-if="connectionMessage" class="connection-message">{{ connectionMessage }}</p>
        <p v-if="error" class="error">{{ error }}</p>
        <footer><button class="secondary" @click="testConnection">연결 테스트</button><button class="primary" :disabled="busy" @click="saveSettings">설정 저장</button></footer>
      </section>
    </div>
  </div>
</template>
