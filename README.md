# CE Express Auto Tracker

Polls the CE Express public tracking API and sends a Telegram message whenever your shipment status changes — no more manually checking the slow portal.

## Setup

### 1. Create a Telegram Bot

1. Open Telegram and search for **@BotFather**
2. Send `/newbot` and follow the prompts (pick any name/username)
3. BotFather gives you a token like `123456789:ABCdef...` — save it

### 2. Get your Chat ID

1. Start a conversation with your new bot (send it any message)
2. Visit this URL in your browser (replace `YOUR_TOKEN`):
   ```
   https://api.telegram.org/botYOUR_TOKEN/getUpdates
   ```
3. Look for `"chat":{"id":XXXXXXX}` — that number is your Chat ID

### 3. Configure

```bash
cp .env.example .env
```

Edit `.env`:
```
TELEGRAM_BOT_TOKEN=123456789:ABCdef...
TELEGRAM_CHAT_ID=987654321
TRACKING_CODES=TBKH000649682
POLL_INTERVAL=3600
```

You can track multiple packages by comma-separating them:
```
TRACKING_CODES=TBKH000649682,TBKH000649683
```

---

## Run with Docker (recommended)

```bash
# Build and start in background
docker compose up -d --build

# Watch the logs
docker compose logs -f

# Stop
docker compose down
```

The container restarts automatically if it crashes or the machine reboots.

---

## Run directly with Python

Requires Python 3.12+.

```bash
pip install -r requirements.txt

# Run continuously
python tracker.py

# Single check only (useful for testing)
python tracker.py --once

# Override tracking codes without editing .env
python tracker.py --codes TBKH000649682,TBKH000649999 --once
```

---

## What you get

When the status changes you'll receive a Telegram message like:

**Out for delivery:**
```
📦 CE Express Update
Tracking: TBKH000649682
Status: Out for Delivery
Destination: Borey Phum Sakura, Phnom Penh, Cambodia
Weight: 9.0 kg

Latest events:
  2026-05-25 06:27:29 [CCSL]: Parcel pickup successfully...
  2026-05-25 11:21:54 [CCSL]: Assigning courier...

Courier: SIM SOKHOK  📞 0977365630
Shop: 0965018377
```

**Delivered:**
```
✅ PACKAGE DELIVERED!
Tracking: TBKH000649682
Status: Delivered
...
```

## State file

`state.json` is auto-created and stores the last known status per tracking code. Delete it to force a re-notification of the current status on next run.
