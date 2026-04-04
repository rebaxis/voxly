## Voxly

A Telegram bot that transcribes voice and audio files by SaluteSpeech, summarises them with GigaChat, and stores the results in PostgreSQL for keyword search.

### Architecture

```
cmd/bot/main.go          — entry point, fx dependency wiring
internal/
  config/config.go       — configuration
  lib/logger/logger.go   — structured logging
  bot/
    bot.go               — Bot lifecycle: handler registration, worker management, result dispatch
    handler.go           — Telegram handlers
    client.go            — Telegram file downloader
    processor.go         — Job processor (download -> transcribe -> summarise)
    queue.go             — Buffered work queue backed by goroutine workers
```

### Request Flow

1. User sends a voice/audio file → `OnVoice` / `OnAudio` handler enqueues a `Job`.
2. A worker goroutine picks up the job, downloads the file via `Client`, and runs `Processor.Process`.
3. The result is sent to the `results` channel.
4. The `dispatchResults` goroutine reads results and delivers the response to the correct user's chat.

### Configuration

Configuration is loaded in ascending priority order: **defaults → config file → flags → environment variables**.

#### Config file

Copy `config.example.json` to `config.json` and fill in your values:

```bash
cp config.example.json config.json
```

The file path defaults to `config.json` in the working directory and can be changed via the `-config` flag or the `CONFIG_FILE` environment variable.

#### Flags and environment variables

ENV variables take the highest priority and always override flags and the config file.

| Flag          | ENV variable     | JSON key          | Default       | Description                           |
|---------------|------------------|-------------------|---------------|---------------------------------------|
| `-config`     | `CONFIG_FILE`    | —                 | `config.json` | Path to JSON config file              |
| `-token`      | `TELEGRAM_TOKEN` | `telegram_token`  | —             | Telegram bot token (required)         |
| `-log-level`  | `LOG_LEVEL`      | `log_level`       | `info`        | Log level: `debug / info / warn / error` |
| `-workers`    | `WORKER_COUNT`   | `worker_count`    | `5`           | Number of queue worker goroutines     |
| `-queue-size` | `QUEUE_SIZE`     | `queue_size`      | `100`         | Job queue buffer size                 |

### Running

```bash
# Config file
go run ./cmd/bot

# Flags
go run ./cmd/bot -token=YOUR_TOKEN -log-level=debug

# Custom config file path via flag
go run ./cmd/bot -config=/etc/voxly/config.json

# Environment variables
TELEGRAM_TOKEN=YOUR_TOKEN LOG_LEVEL=debug go run ./cmd/bot
```

### Bot Commands

| Command           | Description                         |
|-------------------|-------------------------------------|
| `/start`          | Register and see usage              |
| `/list`           | List all saved meetings             |
| `/get <id>`       | Retrieve a meeting transcript       |
| `/find <keyword>` | Search meetings by keyword          |
| `/chat <question>`| Ask the GigaChat AI assistant       |
