# OPIc Flow 구현 계획

## 1. 제품 목표

사용자가 난이도와 설문 주제를 선택하면 실제 OPIc 흐름에 가까운 15문항을 음성으로 듣고 영어로 답한다. 각 답변은 즉시 전사·평가되며, 시험 종료 후 전체 평가를 첫 장으로 하고 문항별 점수·키워드·개선점을 포함하는 보고서를 제공한다.

점수와 추정 등급은 공식 OPIc 성적이 아닌 학습용 지표로 명확히 표시한다.

## 2. 시험 구성

1. 자기소개 1문항
2. 선택 주제 묘사·일상·과거 경험 6문항
3. 롤플레이 3문항
4. 돌발 주제 2문항
5. 비교·의견·미래 예측 3문항

총 15문항이며 난이도에 따라 질문의 추상성, 요구 근거 수, 후속 조건을 조정한다.

## 3. 상태 흐름

`Setup → Device check → Question → Listening → Recording → Transcribing → Evaluating → Next question → Final report`

세션 상태는 Go 백엔드가 권위 있게 관리한다. 프런트엔드는 현재 문항보다 앞선 문항을 임의 제출할 수 없다.

## 4. AI 파이프라인

### 현재 MVP

- 질문 음성: Web Speech Synthesis
- 사용자 음성: MediaRecorder(WebM/Opus)
- 전사: `POST /v1/audio/transcriptions`
- 평가: `POST /v1/responses`, strict JSON Schema
- 다음 질문: 사전 구성된 OPIc 블루프린트

사전 구성 질문을 사용하면 AI가 시험 범위를 벗어나거나 문항 수를 잘못 생성하는 문제를 막을 수 있다. 답변 내용은 다음 문항을 고를 때 분기 조건으로 사용하도록 확장한다.

### Realtime 2단계

- Wails Go 백엔드가 표준 API 키로 `/v1/realtime/calls`에 SDP를 전달한다.
- Vue WebView는 `RTCPeerConnection`으로 마이크 트랙과 `oai-events` 데이터 채널을 구성한다.
- `server_vad` 또는 `semantic_vad`로 발화 종료를 감지한다.
- `input_audio_transcription` 이벤트로 실시간 자막을 표시한다.
- 모델 음성 출력은 원격 audio track으로 재생한다.
- 문항 종료마다 별도의 Responses API 평가를 수행해 점수 JSON을 확정한다.

Realtime 모델이 대화를 자연스럽게 진행하더라도 시험 문항 인덱스와 최대 15문항 제한은 앱 상태 머신이 통제해야 한다.

## 5. 평가 기준

각 문항 100점:

- 과업 수행 25
- 구체성·내용 전개 20
- 구성·연결 15
- 어휘 다양성 15
- 문법 정확성 15
- 유창성·답변 길이 10

오디오 기반 고도화 시 말하기 속도, 긴 침묵, filler, 반복, 자기 수정도 별도 feature로 저장한다. transcript만으로 발음 점수를 단정하지 않는다.

## 6. 보고서

첫 장:

- 전체 평균 점수
- 연습용 추정 등급
- 핵심 강점 3개
- 우선 개선 과제 3개
- 다음 세션 권장 난이도·주제

문항별:

- 질문과 답변 transcript
- 0–100 점수
- 핵심 키워드
- 잘한 점
- 개선점
- 개선된 예시 답변(후속 단계)

내보내기:

- 1차: 브라우저 인쇄/PDF
- 2차: Go PDF 생성 및 세션 JSON/오디오 내보내기

## 7. 인증과 보안

- ChatGPT 웹 로그인/구독은 API 인증으로 재사용하지 않는다.
- 개인용 MVP: 사용자가 Platform API 키를 입력하며 메모리에만 유지한다.
- 배포용: 앱에 공용 API 키를 포함하지 않는다. 자체 백엔드에서 사용자 로그인, 사용량 제한, ephemeral Realtime 인증을 처리한다.
- 장기 로컬 저장이 필요하면 Windows Credential Manager/macOS Keychain/libsecret을 사용한다.
- 오디오는 기본적으로 평가 후 폐기하고, 사용자가 명시적으로 저장을 선택한 경우에만 로컬 암호화 저장한다.

## 8. 단계별 로드맵

### Phase 1 — 현재

- Wails/Vue 앱 골격
- 설정·시험·보고서 UI
- 15문항 상태 머신
- 녹음, 전사, 구조화 평가
- 데모 모드

### Phase 2

- Realtime API WebRTC
- VAD, 실시간 자막, 모델 음성, 끼어들기
- 답변 특징에 따른 제한된 후속 질문 분기
- 마이크/스피커 사전 점검

### Phase 3

- SQLite 세션 기록
- 오디오 파형과 침묵/filler 분석
- 개선 답변 생성과 쉐도잉
- PDF/JSON 내보내기

### Phase 4

- 사용자 계정과 서버 측 API 프록시
- 기기 간 동기화
- 평가 prompt/eval 데이터셋 및 회귀 테스트
- 비용·속도·품질 관측 대시보드

## 9. 검증 계획

- 질문 수와 순서 상태 머신 단위 테스트
- 점수 스키마 0–100 경계 테스트
- 빈 오디오, 마이크 거부, API 401/429/5xx 오류 테스트
- 25MB 오디오 업로드 제한 전 사전 차단
- 세션 완료 전 보고서 접근 차단
- 난이도별 대표 transcript를 사용한 평가 회귀 테스트

