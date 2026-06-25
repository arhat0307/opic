<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from "vue";
import { api } from "./api";
import type { AppSettings, Evaluation, ExamReport, Question } from "./types";

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
  hasApiKey: false,
  demoMode: true,
  evaluationModel: "gpt-5-mini",
  transcribeModel: "gpt-4o-transcribe",
  realtimeModel: "gpt-realtime"
});
const apiKey = ref("");
const connectionMessage = ref("");
const busy = ref(false);
const error = ref("");

const sessionId = ref("");
const currentQuestion = ref<Question | null>(null);
const totalCount = ref(15);
const completedCount = ref(0);
const transcript = ref("");
const latestEvaluation = ref<Evaluation | null>(null);
const report = ref<ExamReport | null>(null);
const recording = ref(false);
const elapsed = ref(0);
let timer: number | undefined;
let recorder: MediaRecorder | null = null;
let stream: MediaStream | null = null;
let chunks: Blob[] = [];
let englishVoice: SpeechSynthesisVoice | null = null;

const progress = computed(() => Math.round((completedCount.value / totalCount.value) * 100));
const topicLabel = computed(() => selectedTopics.value.join(" · "));

onMounted(async () => {
  await loadEnglishVoice();
  try {
    settings.value = await api.getSettings();
  } catch {
    // The UI can still be previewed in a normal browser without the Wails runtime.
  }
});

onBeforeUnmount(() => {
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
      apiKey: apiKey.value,
      demoMode: settings.value.demoMode,
      evaluationModel: settings.value.evaluationModel,
      transcribeModel: settings.value.transcribeModel,
      realtimeModel: settings.value.realtimeModel
    });
    apiKey.value = "";
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
    if (apiKey.value) {
      settings.value = await api.configure({
        apiKey: apiKey.value,
        demoMode: false,
        evaluationModel: settings.value.evaluationModel,
        transcribeModel: settings.value.transcribeModel,
        realtimeModel: settings.value.realtimeModel
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
  if (!currentQuestion.value || !("speechSynthesis" in window)) return;
  speechSynthesis.cancel();

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
  speechSynthesis.speak(utterance);
}

async function toggleRecording() {
  if (recording.value) {
    recorder?.stop();
    return;
  }
  error.value = "";
  try {
    stream = await navigator.mediaDevices.getUserMedia({ audio: true });
    const preferred = MediaRecorder.isTypeSupported("audio/webm;codecs=opus")
      ? "audio/webm;codecs=opus"
      : "audio/webm";
    recorder = new MediaRecorder(stream, { mimeType: preferred });
    chunks = [];
    recorder.ondataavailable = (event) => {
      if (event.data.size) chunks.push(event.data);
    };
    recorder.onstop = () => {
      recording.value = false;
      if (timer) window.clearInterval(timer);
      stopTracks();
    };
    recorder.start(250);
    elapsed.value = 0;
    recording.value = true;
    timer = window.setInterval(() => elapsed.value++, 1000);
  } catch {
    error.value = "마이크 권한을 확인하세요. 텍스트 답변으로도 진행할 수 있습니다.";
  }
}

function stopTracks() {
  stream?.getTracks().forEach((track) => track.stop());
  stream = null;
}

async function submitAnswer() {
  if (!currentQuestion.value || busy.value) return;
  if (recording.value) recorder?.stop();
  busy.value = true;
  error.value = "";
  try {
    const blob = chunks.length ? new Blob(chunks, { type: recorder?.mimeType || "audio/webm" }) : null;
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
    chunks = [];
    if (response.completed) {
      report.value = await api.getReport(sessionId.value);
      screen.value = "report";
      return;
    }
    window.setTimeout(() => {
      currentQuestion.value = response.next || null;
      transcript.value = "";
      latestEvaluation.value = null;
      elapsed.value = 0;
      window.setTimeout(speakQuestion, 350);
    }, 900);
  } catch (e) {
    error.value = readableError(e);
  } finally {
    busy.value = false;
  }
}

function resetExam() {
  speechSynthesis?.cancel();
  screen.value = "setup";
  report.value = null;
  currentQuestion.value = null;
  latestEvaluation.value = null;
  transcript.value = "";
  completedCount.value = 0;
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
          <i></i>{{ settings.demoMode ? "DEMO MODE" : "OPENAI CONNECTED" }}
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
        <div class="exam-tip"><small>COACH'S NOTE</small><p>완벽한 문법보다 구체적인 경험과 자연스러운 이야기 흐름에 집중하세요.</p></div>
      </aside>

      <section class="conversation">
        <div class="question-meta">
          <span>QUESTION {{ currentQuestion?.index?.toString().padStart(2, "0") }}</span>
          <b>{{ currentQuestion?.category }}</b>
          <span>{{ currentQuestion?.topic }}</span>
        </div>
        <div class="question-card">
          <button class="speaker" @click="speakQuestion">◖))</button>
          <div><small>INTERVIEWER</small><h1>{{ currentQuestion?.text }}</h1><p>{{ currentQuestion?.intent }}</p></div>
        </div>

        <div class="answer-area">
          <div class="record-row">
            <button class="record-button" :class="{ active: recording }" @click="toggleRecording"><i></i>{{ recording ? "녹음 중지" : "답변 녹음" }}</button>
            <span class="timer">{{ Math.floor(elapsed / 60).toString().padStart(2, "0") }}:{{ (elapsed % 60).toString().padStart(2, "0") }}</span>
            <span class="audio-bars" :class="{ active: recording }"><i v-for="n in 18" :key="n"></i></span>
          </div>
          <label for="transcript">답변 스크립트 <small>녹음만 제출하면 AI가 자동 전사합니다. 직접 입력하거나 수정할 수도 있습니다.</small></label>
          <textarea id="transcript" v-model="transcript" placeholder="Start speaking, or type your answer here…"></textarea>
          <div class="submit-row">
            <span>{{ transcript.trim().split(/\s+/).filter(Boolean).length }} words</span>
            <button class="primary" :disabled="busy || (!transcript.trim() && !chunks.length)" @click="submitAnswer">{{ busy ? "AI 분석 중…" : "답변 제출" }} <span>→</span></button>
          </div>
        </div>

        <div v-if="latestEvaluation" class="instant-feedback">
          <div class="score">{{ latestEvaluation.score }}</div>
          <div><small>INSTANT FEEDBACK</small><p>{{ latestEvaluation.feedback }}</p></div>
        </div>
        <p v-if="error" class="error">{{ error }}</p>
      </section>
    </main>

    <main v-else-if="report && screen === 'report'" class="report-layout">
      <section class="report-cover">
        <p class="eyebrow">SESSION COMPLETE</p>
        <h1>Speaking<br />Performance<br /><em>Report</em></h1>
        <p>{{ new Date(report.generatedAt).toLocaleDateString("ko-KR") }} · {{ report.config.difficulty }} · {{ report.config.topics.join(", ") }}</p>
        <div class="overall-score">
          <div><strong>{{ report.overall.score }}</strong><span>/ 100</span></div>
          <aside><small>ESTIMATED BAND</small><b>{{ report.overall.estimatedBand }}</b></aside>
        </div>
        <p class="report-summary">{{ report.overall.summary }}</p>
        <div class="report-actions"><button class="secondary" @click="printReport">보고서 인쇄</button><button class="primary" @click="resetExam">새 연습 시작 →</button></div>
      </section>

      <section class="report-content">
        <div class="overview-grid">
          <article><small>STRENGTHS</small><ul><li v-for="item in report.overall.strengths" :key="item">{{ item }}</li></ul></article>
          <article class="accent"><small>NEXT PRIORITIES</small><ul><li v-for="item in report.overall.priorities" :key="item">{{ item }}</li></ul></article>
        </div>
        <div class="answer-list">
          <article v-for="answer in report.answers" :key="answer.question.index" class="answer-report">
            <header><span>Q{{ answer.question.index.toString().padStart(2, "0") }}</span><div><small>{{ answer.question.category }} · {{ answer.question.topic }}</small><h3>{{ answer.question.text }}</h3></div><strong>{{ answer.evaluation.score }}</strong></header>
            <blockquote>{{ answer.transcript }}</blockquote>
            <div class="keyword-row"><span v-for="keyword in answer.evaluation.keywords" :key="keyword">{{ keyword }}</span></div>
            <div class="feedback-grid">
              <div><small>잘한 점</small><p>{{ answer.evaluation.strengths.join(" ") }}</p></div>
              <div><small>개선점</small><p>{{ answer.evaluation.improvements.join(" ") }}</p></div>
            </div>
          </article>
        </div>
      </section>
    </main>

    <div v-if="settingsOpen" class="modal-backdrop" @click.self="settingsOpen = false">
      <section class="settings-modal">
        <button class="modal-close" @click="settingsOpen = false">×</button>
        <p class="eyebrow">AI CONNECTION</p>
        <h2>OpenAI 연결 설정</h2>
        <p class="modal-copy">ChatGPT 웹 로그인은 서드파티 데스크톱 앱의 API 인증으로 사용할 수 없습니다. OpenAI Platform API 키를 사용하세요. 키는 현재 앱 프로세스 메모리에만 보관됩니다.</p>
        <label>API key<input v-model="apiKey" type="password" placeholder="sk-…" autocomplete="off" /></label>
        <label class="switch-row"><span><b>데모 모드</b><small>API 비용 없이 전체 흐름을 시험합니다.</small></span><input v-model="settings.demoMode" type="checkbox" /></label>
        <div class="model-grid">
          <label>평가 모델<input v-model="settings.evaluationModel" /></label>
          <label>전사 모델<input v-model="settings.transcribeModel" /></label>
          <label>Realtime 모델<input v-model="settings.realtimeModel" /></label>
        </div>
        <p v-if="connectionMessage" class="connection-message">{{ connectionMessage }}</p>
        <p v-if="error" class="error">{{ error }}</p>
        <footer><button class="secondary" @click="testConnection">연결 테스트</button><button class="primary" :disabled="busy" @click="saveSettings">설정 저장</button></footer>
      </section>
    </div>
  </div>
</template>
