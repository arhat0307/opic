# opic

Wails 3 + Vue TypeScript 기반 AI OPIc 말하기 연습 앱입니다.

## AI 공급자

설정에서 OpenAI 또는 Google Gemini를 선택할 수 있습니다. 키는 앱 프로세스 메모리에만 보관됩니다.

```powershell
$env:OPENAI_API_KEY="sk-..."
$env:GEMINI_API_KEY="AIza..."
```

Gemini는 평가, 음성 전사, 질문 TTS에 사용할 모델을 각각 여러 개 지정할 수 있습니다. 위에서부터 호출하며 HTTP 오류, 기능 미지원, 빈 응답, 잘못된 평가 JSON, 전사 또는 오디오 누락 시 다음 모델을 자동으로 시도합니다.

기본 Gemini 우선순위:

- 평가·전사: `gemini-3.5-flash` → `gemini-2.5-flash` → `gemini-2.5-flash-lite`
- TTS: `gemini-3.1-flash-tts-preview` → `gemini-2.5-flash-preview-tts`

## 빌드

```powershell
cd frontend
npm install
cd ..
wails3 generate bindings -clean -ts -i
wails3 build
```
