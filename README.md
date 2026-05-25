# CE Express Auto Tracker (Go)

An interactive Telegram bot that tracks CE Express shipments and alerts you when the status changes.

**Rewritten in Go + Gin for production-grade reliability, better performance, and lower costs.**

## Features

- Send `/start` to see previous tracking codes as tap-able buttons
- Tap a button to re-check that code instantly
- Send any new tracking code to add it
- The bot polls every hour in the background and sends an alert automatically when status changes
- Per-user rate limiting (max 10 codes, 5-min cooldown between checks)
- Telegram webhooks (event-driven, not polling)

## Setup

### 1. Create a Telegram Bot

1. Open Telegram and search for **@BotFather**
2. Send `/newbot` and follow the prompts
3. BotFather gives you a token like `123456789:ABCdef...` — save it

### 2. Configure

```bash
cp .env.example .env
```

Edit `.env`:
```
TELEGRAM_BOT_TOKEN=123456789:ABCdef...
BOT_WEBHOOK_URL=https://your-domain.com/webhook    # Your public bot URL
POLL_INTERVAL=3600                                   # 1 hour (in seconds)
MAX_CODES_PER_USER=10                                # Max codes per user
MIN_CHECK_INTERVAL=300                               # 5 min cooldown between checks
```

---

## Run with Docker

```bash
docker compose up -d --build
docker compose logs -f
```

Two services:
- **bot**: Gin HTTP server listening on `:8080` — handles Telegram webhooks
- **worker**: Background poller — checks all tracking codes every hour

Both services share a persistent volume (`/data/state.json`).

---

## Run Locally

Requires Go 1.22+.

### Terminal 1: Bot service

```bash
go run ./cmd/bot/main.go
```

### Terminal 2: Worker service (in another terminal)

```bash
go run ./cmd/worker/main.go
```

---

## Deploy to Railway

### Connect your GitHub repo

1. Go to [railway.app](https://railway.app)
2. Create a new project → "Deploy from GitHub"
3. Select your repo

### Environment variables

Set in Railway project settings:
```
TELEGRAM_BOT_TOKEN=your_token_here
BOT_WEBHOOK_URL=https://<railway-domain>.up.railway.app/webhook
POLL_INTERVAL=3600
MAX_CODES_PER_USER=10
MIN_CHECK_INTERVAL=300
```

### Create a shared volume

Railway will automatically use `docker-compose.yml` and `Dockerfile.bot`/`Dockerfile.worker` to deploy both services with a shared volume.

Auto-deploy on push to `main` branch.

---

## How it works

```
You: /start
Bot: 👋 Welcome back!
     [TBKH000649682]  [TBKH000649683]
     Tap a previous code or send a new one.

You: (tap TBKH000649682)
Bot: 🔍 Checking TBKH000649682…
Bot: 📦 CE Express Tracker
     Tracking: TBKH000649682
     Status: Out for Delivery
     Courier: SIM SOKHOK  📞 0977365630

(1 hour later, status changes)
Bot: 🔔 STATUS UPDATE!
     Tracking: TBKH000649682
     Status changed: Out for Delivery → Delivered
     ...
```

---

## Architecture

### Bot Service (`cmd/bot/main.go`)

- Gin HTTP server (`POST /webhook`, `GET /health`)
- Routes Telegram updates to handlers
- Registers webhook with Telegram on startup
- Per-user state via JSON file

**Handlers:**
- `/start` command
- Text messages (new tracking codes)
- Button callbacks (re-check existing codes)

### Worker Service (`cmd/worker/main.go`)

- Goroutine-based background poller
- Tickers every `POLL_INTERVAL` seconds
- Checks all tracking codes for status changes
- Sends alerts via Telegram API if status changed
- Reads/writes same JSON state file as bot

### Shared State

- Mounted at `/data/state.json` (Railway persistent volume)
- Both services read/write concurrently
- Bot uses `sync.Mutex` for safe concurrent access
- Worker uses file-level locking (retry-on-busy)

---

## Performance

**Binary size:**
- Bot: ~12MB (compressed Docker image ~5MB)
- Worker: ~8MB (compressed Docker image ~4MB)

**Memory usage:** ~20-30MB per service (vs ~200MB for Python)

**Response time:** <100ms for Telegram webhook

---

## Rate Limiting

Prevents abuse on a public bot:
- Max 10 tracking codes per user
- 5-minute cooldown between checks of the same code

Configurable via `.env`:
```
MAX_CODES_PER_USER=10
MIN_CHECK_INTERVAL=300
```

---

## Project Structure

```
ce-tracker/
├── cmd/
│   ├── bot/main.go           # Gin server + webhook handler
│   └── worker/main.go        # Background poller
├── internal/
│   ├── config/config.go      # Environment config
│   ├── state/state.go        # JSON state management (mutex-safe)
│   ├── telegram/client.go    # Telegram API client
│   ├── tracker/ceexpress.go  # CE Express API client
│   ├── handler/webhook.go    # Telegram update routing
│   └── ratelimit/ratelimit.go # Rate limit checks
├── Dockerfile.bot            # Multi-stage build for bot
├── Dockerfile.worker         # Multi-stage build for worker
├── docker-compose.yml        # Local dev (two services + volume)
├── railway.toml              # Railway deployment config
├── go.mod & go.sum          # Go dependencies
└── .env.example             # Configuration template
```

---

## License

MIT
