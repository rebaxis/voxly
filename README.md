## Voxly

A Telegram bot that transcribes voice and audio files with **SaluteSpeech**, stores transcripts in **PostgreSQL**, and optionally generates a **short meeting summary** with **GigaChat** (saved in the database). Users can list meetings, open a transcript with summary, search by keyword, and ask **GigaChat** open questions via **`/chat`** when credentials are configured.

### Architecture

```
cmd/bot/main.go              — entry point, fx dependency wiring
migrations/                  — SQL migration files (golang-migrate)
internal/
  config/config.go           — configuration (file + flags + ENV)
  lib/logger/logger.go       — structured logging (zap, UTC timestamps)
  model/meeting.go           — domain types: User, Meeting
  db/db.go                   — PostgreSQL connection pool + auto-migration
  repository/
    meeting.go               — MeetingRepository: save, get, list, search, update summary
    user.go                  — UserRepository: idempotent registration
  service/                   — business-logic layer (between bot and repositories)
    meeting.go               — MeetingService: register, list, get, search, /chat → GigaChat
    transcription.go         — TranscriptionService: SaluteSpeech + save meeting + GigaChat summary
  salutespeech/client.go     — SaluteSpeech REST client (OAuth2 + async recognition)
  gigachat/client.go         — GigaChat REST client (OAuth2 + chat completions: summaries + /chat)
  bot/
    bot.go                   — Bot lifecycle: handler registration, workers, result dispatch
    handler.go               — Telegram handlers: OnText (commands), OnVoice, OnAudio
    client.go                — Telegram file downloader
    processor.go             — Job processor: download → delegate to TranscriptionService
    queue.go                 — Buffered work queue backed by goroutine workers
```

### Layer Diagram

```
Telegram API
    │
    ▼
Handler  (bot/handler.go)          ← OnText, OnVoice, OnAudio
    │   depends on
    ├──▶ MeetingService            ← Register, List, Get, Search, Chat (/chat)
    │       │ depends on
    │       ├──▶ MeetingRepository
    │       ├──▶ UserRepository
    │       └──▶ GigaChat client
    │
    └──▶ Queue  (bot/queue.go)
             │
             ▼
         Processor  (bot/processor.go)
             │   depends on
             ├──▶ Client           ← downloads file from Telegram
             └──▶ TranscriptionService  ← Transcribe + Save + optional summary
                     │ depends on
                     ├──▶ SaluteSpeech client
                     ├──▶ GigaChat client   ← SummarizeTranscript after save
                     └──▶ MeetingRepository
```

### Request Flow

1. User sends a voice/audio file → `OnVoice` / `OnAudio` calls `MeetingService.Register` and enqueues a `Job`.
2. A worker goroutine picks up the job and calls `Processor.Process`.
3. `Processor` downloads the raw audio bytes from Telegram via `Client`.
4. `Processor` delegates to `TranscriptionService.Transcribe`:
   - sends audio to SaluteSpeech for transcription,
   - saves the resulting `Meeting` row in PostgreSQL,
   - if GigaChat is configured, requests a short Russian summary and updates `meetings.summary` (failures are logged; the transcript is still kept).
5. The result string (meeting id and optional summary) is written to the `results` channel.
6. The `dispatchResults` goroutine reads results and sends the reply to the correct user's chat.

Bot commands (`/start`, `/list`, `/get`, `/find`, `/chat`) are handled synchronously via `MeetingService` (and GigaChat for `/chat`) without using the work queue, except for voice/audio uploads which enqueue jobs as above.

### Service Layer

`internal/service` contains the business-logic layer that sits between the Telegram bot handlers and the persistence layer. Neither the handler nor the processor import `repository`, `salutespeech`, or `gigachat` directly — those dependencies live inside the services.

| Service | Interface methods | Used by |
|---|---|---|
| `MeetingService` | `Register`, `List`, `Get`, `Search`, `Chat` | `Handler` (text commands, including `/chat`) |
| `TranscriptionService` | `Transcribe` | `Processor` (voice/audio pipeline: speech + DB + summary) |

This design makes each layer independently testable: tests for the handler inject a `mockMeetingService`; tests for the processor inject a `mockProcessor` without needing real repositories or APIs.

### Database

PostgreSQL is used for storage. Migrations run automatically on startup (including `000002_meeting_summary` which adds `meetings.summary`).

#### Schema

| Table      | Columns                                          | Notes                                  |
|------------|--------------------------------------------------|----------------------------------------|
| `users`    | `user_id`, `created_at`                          | Registered on `/start` or first upload |
| `meetings` | `id` (UUID), `user_id`, `file_id`, `transcript`, `summary`, `created_at` | `summary` filled by GigaChat after transcription; GIN index on combined transcript + summary for `/find` |

#### Full-text search

`/find` uses PostgreSQL `to_tsvector` / `plainto_tsquery` with the `russian` text search configuration over **transcript and summary** (combined).

### SaluteSpeech Integration

`internal/salutespeech` implements the [SaluteSpeech REST API](https://developers.sber.ru/docs/ru/salutespeech/api/grpc/recognition-async-grpc):

- **Auth** — OAuth2 token from `ngw.devices.sberbank.ru`, cached with a 60-second pre-expiry buffer using `go-cache`. The token request must send `Authorization: Basic <key>` where **`<key>` is the “Authorization Key” from Sber GigaChat / SaluteSpeech Studio**. Paste it as `salutespeech_authorization_key`; if it already includes a `Basic ` prefix, it is sent as-is.
- **Recognition** — async only: `data:upload` → `speech:async_recognize` → `task:get` (poll) → `data:download` (see [async guide](https://developers.sber.ru/docs/ru/salutespeech/guides/recognition/recognition-async)).
- **Stub mode** — if `salutespeech_authorization_key` is empty, a placeholder client is used so the bot runs without the API.

### GigaChat integration

`internal/gigachat` calls the [GigaChat REST API](https://developers.sber.ru/docs/ru/gigachat/api/reference/rest/gigachat-api) for **chat completions**:

- **Auth** — same OAuth2 gateway as other Sber APIs: `POST https://ngw.devices.sberbank.ru:9443/api/v2/oauth` with `Authorization: Basic <key>` (the **Authorization Key** from Sber Studio for the GigaChat API project). Tokens are cached with a 60-second pre-expiry buffer.
- **Chat** — `POST https://gigachat.devices.sberbank.ru/api/v1/chat/completions` with `Authorization: Bearer <token>`.
- **Summaries** — after a meeting is saved, `SummarizeTranscript` asks the model for a short Russian summary (transcript truncated for context limits); the result is stored in `meetings.summary`.
- **`/chat`** — `MeetingService.Chat` sends the user’s question with no meeting-specific context (general assistant behaviour).
- **Default model** — if `gigachat_model` is omitted, the client uses **`GigaChat`**.
- **TLS** — like SaluteSpeech, certificate verification is disabled for these endpoints (custom CA); do not use that pattern for arbitrary hosts.
- **Stub mode** — if `gigachat_authorization_key` is empty, summarization is skipped (transcripts only) and `/chat` replies that GigaChat is not configured.

### Configuration

Configuration is loaded in ascending priority order: **defaults → config file → flags → environment variables**.

#### Config file

Copy `config.example.json` to `config.json` and fill in your values:

```bash
cp config.example.json config.json
```

The file path defaults to `config.json` in the working directory and can be changed via the `-config` flag or the `CONFIG_FILE` environment variable.

`config.example.json` lists **SaluteSpeech** (`salutespeech_*`) and **GigaChat** (`gigachat_*`) keys. Only `telegram_token` and `database_dsn` are required for a minimal run; speech and AI keys are optional (each subsystem falls back to a stub when its key is empty).

#### Flags and environment variables

ENV variables take the highest priority and always override flags and the config file.

| Flag                    | ENV variable                  | JSON key                      | Default             | Description                              |
|-------------------------|-------------------------------|-------------------------------|---------------------|------------------------------------------|
| `-config`               | `CONFIG_FILE`                 | —                             | `config.json`       | Path to JSON config file                 |
| `-token`                | `TELEGRAM_TOKEN`              | `telegram_token`              | —                   | Telegram bot token (required)            |
| `-log-level`            | `LOG_LEVEL`                   | `log_level`                   | `info`              | `debug / info / warn / error`            |
| `-workers`              | `WORKER_COUNT`                | `worker_count`                | `5`                 | Number of queue worker goroutines        |
| `-queue-size`           | `QUEUE_SIZE`                  | `queue_size`                  | `100`               | Job queue buffer size                    |
| `-db-dsn`               | `DATABASE_DSN`                | `database_dsn`                | —                   | PostgreSQL DSN (required)                |
| `-db-migrations`        | `DB_MIGRATIONS_PATH`          | `db_migrations_path`          | `file://migrations` | Path to migration files                  |
| `-db-max-open-conns`    | `DB_MAX_OPEN_CONNS`           | `db_max_open_conns`           | `10`                | Max open DB connections                  |
| `-db-max-idle-conns`    | `DB_MAX_IDLE_CONNS`           | `db_max_idle_conns`           | `5`                 | Max idle DB connections                  |
| `-db-conn-max-lifetime` | `DB_CONN_MAX_LIFETIME`        | `db_conn_max_lifetime`        | `5m`                | Max DB connection lifetime               |
| `-ss-auth-key`          | `SALUTESPEECH_AUTHORIZATION_KEY` | `salutespeech_authorization_key` | —                | SaluteSpeech Basic credential from Studio |
| `-ss-scope`             | `SALUTESPEECH_SCOPE`         | `salutespeech_scope`          | `SALUTE_SPEECH_PERS` | OAuth scope (e.g. `SALUTE_SPEECH_PERS`)   |
| `-gc-auth-key`          | `GIGACHAT_AUTHORIZATION_KEY` | `gigachat_authorization_key`  | —                | GigaChat Basic credential from Studio (optional) |
| `-gc-scope`             | `GIGACHAT_SCOPE`             | `gigachat_scope`              | `GIGACHAT_API_PERS`* | GigaChat OAuth scope (e.g. `GIGACHAT_API_PERS`) |
| `-gc-model`             | `GIGACHAT_MODEL`             | `gigachat_model`              | `GigaChat`* | Model id passed to chat completions |

\*Defaults apply when the JSON field is absent or empty in config; the code also defaults the model to **`GigaChat`** if unset when constructing the client.

### Running

```bash
# Using config file
cp config.example.json config.json
# fill in telegram_token and database_dsn at minimum
go run ./cmd/bot

# Using flags only
go run ./cmd/bot \
  -token=YOUR_TOKEN \
  -db-dsn=postgres://user:pass@localhost/voxly \
  -ss-auth-key=YOUR_SS_STUDIO_KEY \
  -gc-auth-key=YOUR_GC_STUDIO_KEY \
  -log-level=debug

# Using environment variables
TELEGRAM_TOKEN=YOUR_TOKEN \
DATABASE_DSN=postgres://user:pass@localhost/voxly \
SALUTESPEECH_AUTHORIZATION_KEY=YOUR_SS_STUDIO_KEY \
GIGACHAT_AUTHORIZATION_KEY=YOUR_GC_STUDIO_KEY \
GIGACHAT_MODEL="GigaChat" \
go run ./cmd/bot

# Custom config file path with flag override
go run ./cmd/bot -config=/etc/voxly/config.json -log-level=debug -workers=10
```

### Bot Commands

| Command           | Description                                        |
|-------------------|----------------------------------------------------|
| `/start`          | Register and display usage                         |
| `/list`           | List all saved meetings sorted by date             |
| `/get <id>`       | Your meeting’s **summary** (if any) and full **transcript** (scoped to your user) |
| `/find <keyword>` | Full-text search (`russian`) across your transcripts and summaries |
| `/chat <question>`| Ask **GigaChat** a general question (requires `gigachat_authorization_key`; otherwise the bot explains that GigaChat is not configured) |
