# CE Express Auto Tracker

An interactive Telegram bot that tracks CE Express shipments and alerts you when the status changes.

- Send `/start` to the bot — it shows your previous tracking codes as tap-able buttons
- Tap a button to re-check that code instantly
- Send any new tracking code as a message to add it
- The bot polls every hour in the background and sends an alert automatically when anything changes

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
POLL_INTERVAL=3600
```

No chat ID needed — the bot discovers it automatically when you message it.

---

## Run with Docker (recommended)

```bash
docker compose up -d --build
docker compose logs -f
```

The container restarts automatically on crash or reboot. State is stored in a named Docker volume so it survives container rebuilds.

---

## Run directly with Python

Requires Python 3.12+.

```bash
pip install -r requirements.txt
python tracker.py
```

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

(1 hour later, status changes automatically)
Bot: ✅ PACKAGE DELIVERED!
     Tracking: TBKH000649682
     ...
```
