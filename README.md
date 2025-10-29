# ZANA – Interactive Speech (Go + React)

ZANA is an interactive, voice-enabled assistant built on OpenAI APIs. Backend in Go, frontend in React (Vite). You can chat with ZANA via text or voice; voice requests are transcribed to text and replies are synthesized back to audio.

## Prerequisites

- Go 1.21+
- Node 18+
- An OpenAI API key

## Quick start

1. Backend

```bash
cp .env.example .env
# edit .env and set OPENAI_API_KEY
# optional: add ELEVEN_API_KEY and ELEVEN_VOICE_ID to enable ElevenLabs TTS
cd backend/cmd/zana-server
go run .
```

The server listens on port 8080 by default.

Endpoints:

- GET /api/health
- POST /api/chat # JSON: { sessionId?, message, system? }
- POST /api/chat/stream # same request; streamed text/plain response
- POST /api/voice # multipart: file(webm/mp3/wav), sessionId? -> JSON { transcript, reply }
- POST /api/tts # JSON: { text } -> audio/mpeg (uses ElevenLabs when configured)

2. Frontend

```bash
cd web
npm install
# optionally set VITE_API_BASE in web/.env
# set VITE_TTS_PROVIDER=eleven to use ElevenLabs output via /api/tts
npm run dev
```

Open http://localhost:5173 to use the UI. By default it targets http://localhost:8080.

## Environment variables (.env)

- OPENAI_API_KEY – required
- PORT – default 8080
- ALLOWED_ORIGIN – default \* (set to http://localhost:5173 for dev)
- OPENAI_MODEL – default gpt-4o-mini
- OPENAI_TTS_MODEL – default tts-1
- OPENAI_STT_MODEL – default whisper-1
- ELEVEN_API_KEY – optional, enables ElevenLabs TTS
- ELEVEN_VOICE_ID – ElevenLabs voice id to use
- ELEVEN_MODEL_ID – default eleven_multilingual_v2

## Notes

- The session store is in-memory; replace for persistence.
- Frontend uses browser speech synthesis by default; voice endpoint returns transcript+reply. When VITE_TTS_PROVIDER=eleven, replies are played from /api/tts.
- Recording uses MediaRecorder with Opus in WebM, MP4 or other codecs depending on browser support.
