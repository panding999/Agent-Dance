# Doubao Go Backend Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use `superpowers:subagent-driven-development` (recommended) or `superpowers:executing-plans` to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build the Go backend for the Doubao-first AI simultaneous interpretation assistant.

**Architecture:** The frontend sends microphone PCM frames and upload jobs to a Go backend. The backend acts as the audio gateway: it manages sessions, validates and packetizes audio, connects to Doubao AST WebSocket/protobuf, normalizes provider events, stabilizes subtitles, runs background correction, and persists session history.

**Tech Stack:** Go 1.22+, Fastify-compatible REST style through Go HTTP handlers, Gorilla/WebSocket or nhooyr WebSocket, protobuf, SQLite, ffmpeg, Doubao AST, Doubao AUC, Doubao TTS.

---

## 1. Backend Responsibility Map

The backend should be split by responsibility, not by generic layers.

```text
cmd/server
  main.go                         # process entrypoint

internal/config
  config.go                       # env loading and validation

internal/httpapi
  router.go                       # REST route registration
  health.go                       # health and readiness endpoints
  sessions.go                     # session REST handlers
  uploads.go                      # upload REST handlers
  glossary.go                     # glossary REST handlers

internal/live
  gateway.go                      # browser WebSocket gateway
  browser_protocol.go             # browser frame/event protocol
  session_runner.go               # one live session orchestration

internal/audio
  pcm.go                          # PCM frame types and validation
  packetizer.go                   # 80ms packet splitting
  resample.go                     # fallback resampling hooks
  chunk_cache.go                  # recent audio cache for repair

internal/doubao/ast
  client.go                       # AST WebSocket lifecycle
  protocol.go                     # protobuf request/response wrappers
  events.go                       # provider event types
  mapper.go                       # AST event -> internal event mapping

internal/doubao/auc
  client.go                       # recording recognition client
  request.go                      # upload/url recognition request models

internal/doubao/tts
  client.go                       # optional standalone TTS client

internal/subtitle
  event.go                        # internal InterpretationEvent types
  stabilizer.go                   # partial/final/revision handling
  diff.go                         # correction diff rules

internal/repair
  engine.go                       # background correction pipeline
  queue.go                        # repair job queue

internal/upload
  worker.go                       # upload processing orchestration
  ffmpeg.go                       # audio extraction commands
  storage.go                      # local temp file handling

internal/store
  db.go                           # SQLite connection
  migrations.go                   # schema setup
  sessions.go                     # session repository
  segments.go                     # segment/revision repository
  glossary.go                     # glossary repository

internal/observability
  logging.go                      # structured logs
  metrics.go                      # latency counters and gauges
```

## 2. Delivery Milestones

### M1: Go Backend Skeleton

Goal: backend can start, read config, expose health endpoints, and persist a basic session.

- [ ] Create `go.mod` with module name `github.com/panding999/agent-dance/backend`.
- [ ] Create `cmd/server/main.go`.
- [ ] Add `internal/config/config.go` with required env vars:
  - `DOUBAO_APP_KEY`
  - `DOUBAO_ACCESS_KEY`
  - `DOUBAO_AST_RESOURCE_ID`
  - `DOUBAO_AUC_RESOURCE_ID`
  - `DATABASE_URL`
  - `UPLOAD_DIR`
- [ ] Add `GET /healthz`.
- [ ] Add `GET /readyz`.
- [ ] Add SQLite initialization.
- [ ] Add `sessions`, `segments`, `segment_revisions`, `glossary_terms`, `audio_chunks`, `provider_events` tables.
- [ ] Add `POST /api/sessions` and `GET /api/sessions/:id`.

Verification:

```powershell
go test ./...
go run ./cmd/server
curl http://localhost:8080/healthz
```

Expected result: tests pass, server starts, health endpoint returns `200`.

### M2: Browser Live WebSocket Gateway

Goal: browser can open a session WebSocket and stream audio frames into the backend without contacting Doubao yet.

- [x] Add `GET /api/live/ws?sessionId=...`.
- [x] Define browser binary audio frame format:
  - 4 bytes sequence number
  - 8 bytes timestamp milliseconds
  - remaining bytes Int16 PCM little-endian
- [x] Define server JSON event format for state changes.
- [x] Validate sequence ordering.
- [x] Validate sample size and mono PCM alignment.
- [x] Store recent frames in `audio.ChunkCache`.
- [x] Add ping/pong timeout.
- [x] Add close handling that marks session closed.

Verification:

```powershell
go test ./internal/live ./internal/audio
```

Expected result: gateway tests cover valid frames, malformed frames, out-of-order frames, and close behavior.

### M3: Audio Packetizer

Goal: backend can produce Doubao-compatible audio packets.

- [x] Implement `audio.PCMFrame`.
- [x] Implement `audio.Packetizer` for 80ms packets.
- [x] Enforce 16kHz, 16bit, mono.
- [x] Add fallback path for non-16kHz input that returns a clear error first.
- [x] Keep ffmpeg resampling as a later optional path, not part of first live MVP.

Verification:

```powershell
go test ./internal/audio
```

Expected result: packetizer splits exactly 2560 bytes per 80ms packet at 16kHz Int16 mono.

### M4: Doubao AST Bridge

Goal: backend can create a Doubao AST session and send audio packets.

- [x] Add `internal/doubao/ast/client.go`.
- [x] Load AST endpoint `wss://openspeech.bytedance.com/api/v4/ast/v2/translate`.
- [x] Attach required headers:
  - `X-Api-App-Key`
  - `X-Api-Resource-Id: volc.service_type.10053`
- [x] Add protobuf request and response wrappers.
- [x] Implement StartSession with:
  - `mode=s2t` or `mode=s2s`
  - `source_language`
  - `target_language`
  - `source_audio.format=wav`
  - `source_audio.codec=raw`
  - `source_audio.rate=16000`
  - `source_audio.bits=16`
  - `source_audio.channel=1`
  - `corpus` terms
- [x] Implement SendAudio packet call.
- [x] Implement FinishSession.
- [x] Capture provider logid and errors.

Verification:

```powershell
go test ./internal/doubao/ast
```

Expected result: AST client unit tests pass with a fake WebSocket server. Real provider smoke test is separate and must require explicit credentials.

### M5: Provider Event Normalizer

Goal: Doubao AST events are converted into app-level events that the frontend can consume.

- [x] Define `subtitle.InterpretationEvent`.
- [x] Map `SourceSubtitleResponse` to `segment.partial.sourceText`.
- [x] Map `SourceSubtitleEnd` to source final metadata.
- [x] Map `TranslationSubtitleResponse` to `segment.partial.text`.
- [x] Map `TranslationSubtitleEnd` to `segment.final`.
- [x] Map `TTSResponse` to `audio.delta`.
- [x] Map `SessionFailed` to `session.error`.
- [x] Store provider event summaries for debugging.

Verification:

```powershell
go test ./internal/doubao/ast ./internal/subtitle
```

Expected result: fixture provider events produce stable internal events with consistent segment ids.

### M6: Subtitle Stabilizer

Goal: the UI receives readable subtitle state instead of raw provider noise.

- [x] Implement current segment tracking.
- [x] Merge source and translation events by provider segment id or time window.
- [x] Update current line for partial text.
- [x] Persist final segments.
- [x] Ignore trivial partial churn.
- [x] Emit `segment.final` once per segment.
- [x] Add max line length and max duration guards.

Verification:

```powershell
go test ./internal/subtitle
```

Expected result: partial updates do not create history rows; final events persist exactly once.

### M7: Live Session Runner

Goal: one browser session can flow through browser WebSocket -> packetizer -> Doubao AST -> normalized events -> frontend.

- [ ] Implement `live.SessionRunner`.
- [ ] Connect browser WebSocket and AST client lifecycle.
- [ ] Forward packetized audio to AST.
- [ ] Forward normalized subtitle events to browser.
- [ ] Forward `audio.delta` when `mode=s2s`.
- [ ] On browser close, finish AST session.
- [ ] On AST failure, close browser session with reason.

Verification:

```powershell
go test ./internal/live ./internal/doubao/ast ./internal/subtitle
```

Expected result: fake AST server receives audio packets and browser receives normalized events.

### M8: Glossary, Hot Words, and Replacement Words

Goal: users can configure technical terms before and during a session.

- [ ] Add `POST /api/glossaries`.
- [ ] Add `PATCH /api/glossaries/:id`.
- [ ] Store hot words, replacement words, and glossary pairs.
- [ ] Map glossary config to Doubao `corpus`.
- [ ] Add `POST /api/sessions/:id/glossary/update`.
- [ ] Send AST UpdateConfig during live sessions.

Verification:

```powershell
go test ./internal/httpapi ./internal/store ./internal/doubao/ast
```

Expected result: a glossary containing `RAG -> 检索增强生成` appears in StartSession corpus.

### M9: Upload Processing

Goal: uploaded course videos can be converted into transcript and subtitle timeline.

- [ ] Add `POST /api/uploads`.
- [ ] Save upload to `UPLOAD_DIR`.
- [ ] Add file type and size limits.
- [ ] Add `upload.FFmpegExtractAudio`.
- [ ] Convert video to audio.
- [ ] Add AUC client request by local data or object URL.
- [ ] Persist utterances as source segments.
- [ ] Translate source text with configured translation provider.
- [ ] Persist translated segments.
- [ ] Add `GET /api/uploads/:id/result`.

Verification:

```powershell
go test ./internal/upload ./internal/doubao/auc ./internal/store
```

Expected result: fake AUC response creates timestamped source segments and translated target segments.

### M10: Background Repair Engine

Goal: stable subtitles can be automatically corrected later.

- [ ] Add repair job queue.
- [ ] Trigger repair for high-risk segments:
  - numbers
  - units
  - dates
  - glossary terms
  - abnormal source/target length ratio
- [ ] Extract audio window from `audio.ChunkCache`.
- [ ] Submit window to AUC recognizer.
- [ ] Re-translate with glossary constraints.
- [ ] Compare before/after with `subtitle.Diff`.
- [ ] Persist `segment_revisions`.
- [ ] Emit `segment.revision`.

Verification:

```powershell
go test ./internal/repair ./internal/subtitle ./internal/store
```

Expected result: `fifteen hundred` correction changes `十五` to `一千五百` and records reason `number_correction`.

### M11: TTS Queue

Goal: Chinese speech playback does not block subtitles.

- [ ] Prefer AST `TTSResponse` in `s2s` mode.
- [ ] Add standalone TTS queue for upload replay and `s2t` sessions.
- [ ] Skip expired TTS jobs.
- [ ] Support pause/resume.
- [ ] Emit audio chunks to browser.

Verification:

```powershell
go test ./internal/doubao/tts ./internal/live
```

Expected result: queued audio skips stale segments and only plays current stable subtitles.

### M12: Observability and Demo Readiness

Goal: failures are diagnosable during a live demo.

- [ ] Add structured logs for session id, provider logid, state, and latency.
- [ ] Add metrics for:
  - websocket connected sessions
  - first subtitle latency
  - final subtitle latency
  - repair queue length
  - upload processing duration
- [ ] Add `/api/sessions/:id/debug` for local demo diagnostics.
- [ ] Redact secrets from all logs.
- [ ] Add demo fixture scripts.

Verification:

```powershell
go test ./...
```

Expected result: all backend tests pass and logs show provider logid without exposing credentials.

## 3. Recommended Build Order

1. M1: Skeleton and storage.
2. M2: Browser WebSocket gateway.
3. M3: Audio packetizer.
4. M4: Doubao AST bridge with fake server tests.
5. M5: Event normalizer.
6. M6: Subtitle stabilizer.
7. M7: End-to-end live session runner.
8. M8: Glossary and UpdateConfig.
9. M9: Upload processing.
10. M10: Background repair.
11. M11: TTS queue.
12. M12: Observability and demo readiness.

This order produces a working demo early: after M7, the app can already show real-time subtitles. M8-M12 add quality, fallback, and presentation polish.

## 4. First Coding Sprint

The first sprint should stop at a backend that starts and accepts fake live audio.

- [ ] Create Go module and server skeleton.
- [ ] Add config validation.
- [ ] Add SQLite schema.
- [ ] Add session REST API.
- [ ] Add browser WebSocket endpoint.
- [ ] Add audio frame validation.
- [ ] Add packetizer unit tests.
- [x] Add fake AST server test scaffold.

Exit criteria:

- `go test ./...` passes.
- `go run ./cmd/server` starts.
- A local WebSocket test can send fake PCM frames.
- No real Doubao credential is required yet.

## 5. Provider Credential Checklist

Before real Doubao smoke testing, prepare:

- `DOUBAO_APP_KEY`
- `DOUBAO_ACCESS_KEY`
- `DOUBAO_AST_RESOURCE_ID=volc.service_type.10053`
- `DOUBAO_AUC_RESOURCE_ID=volc.bigasr.auc_turbo`
- confirmed language pair, for example `en -> zh`
- one 30-second English demo clip
- one glossary sample, for example `RAG -> 检索增强生成`

Real provider tests should be manual or gated by `RUN_DOUBAO_SMOKE=1`, so CI does not spend quota or fail without credentials.
