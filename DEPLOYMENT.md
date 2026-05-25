# CE Express Tracker — Deployment Guide

Complete step-by-step guide to deploy the bot to Railway with webhook setup.

---

## Prerequisites

- [x] GitHub account
- [x] Railway account ([railway.app](https://railway.app))
- [x] Telegram account
- [x] Git installed locally
- [x] Docker installed (optional, for local testing)

---

## Part 1: Create Your Telegram Bot

### Step 1.1: Talk to BotFather

1. Open Telegram
2. Search for **@BotFather** and start a chat
3. Send: `/newbot`
4. BotFather asks for a name → enter e.g. `CE Express Tracker`
5. BotFather asks for a username → enter e.g. `my_ce_tracker_bot` (must end in `_bot`)
6. **Save the token** BotFather gives you — format looks like:
   ```
   1234567890:AAxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx
   ```

### Step 1.2: (Optional) Set Bot Profile Picture

1. In BotFather chat, send `/setuserpic`
2. Select your bot
3. Upload `bot_icon.svg` (convert to PNG first at [convertio.co/svg-png](https://convertio.co/svg-png/))

### Step 1.3: (Optional) Set Bot Description

1. Send `/setdescription` to BotFather
2. Select your bot
3. Enter: `Track CE Express shipments and get automatic status updates`

---

## Part 2: Push Code to GitHub

### Step 2.1: Verify Local Changes

```bash
cd /Users/heng.sunhour/Desktop/personal/new
git status
```

Should show: `nothing to commit, working tree clean`

### Step 2.2: Push to GitHub

```bash
git push origin main
```

---

## Part 3: Deploy to Railway

### Step 3.1: Create Railway Project

1. Go to [railway.app](https://railway.app) and log in
2. Click **"New Project"**
3. Select **"Deploy from GitHub repo"**
4. Authorize Railway to access your GitHub (first time only)
5. Select your repo (e.g., `Dashsyy/personal-new` or whatever you named it)

### Step 3.2: Railway Auto-Detects Two Services

Railway will read `docker-compose.yml` and create **two services**:
- `bot` — Gin HTTP server
- `worker` — Background poller

If Railway only creates one service, manually add the second:
1. Click **"+ New"** → **"Empty Service"**
2. Connect to the same GitHub repo
3. In Settings → Build, set Dockerfile path to `Dockerfile.worker`

### Step 3.3: Create Shared Volume

Both services need access to the same `state.json`:

1. In Railway project → Click **Volumes** tab (or Settings → Volumes)
2. Click **"+ New Volume"**
3. Name: `tracker-state`
4. Mount path: `/data`
5. Attach to **both** `bot` and `worker` services

### Step 3.4: Generate Public Domain for Bot

1. Click on the **bot** service
2. Go to **Settings** → **Networking**
3. Click **"Generate Domain"**
4. Railway gives you a URL like:
   ```
   https://ce-tracker-bot-production.up.railway.app
   ```
5. **Save this URL** — you'll need it for the webhook

### Step 3.5: Set Environment Variables

For **EACH service** (bot and worker), add these environment variables:

Go to service → **Variables** tab → Add:

```bash
TELEGRAM_BOT_TOKEN=your_bot_token_from_botfather
POLL_INTERVAL=3600
MAX_CODES_PER_USER=10
MIN_CHECK_INTERVAL=300
STATE_FILE=/data/state.json
```

For the **bot service only**, also add:

```bash
BOT_WEBHOOK_URL=https://ce-tracker-bot-production.up.railway.app/webhook
PORT=8080
```

⚠️ **Important:** Replace the URL with YOUR Railway domain from Step 3.4

### Step 3.6: Deploy

Railway auto-deploys after env vars are set. To force redeploy:
1. Click service → **Deployments** tab
2. Click **"Deploy"** button

Wait 2-3 minutes for both services to build and start.

---

## Part 4: Verify Deployment

### Step 4.1: Check Bot Service Logs

In Railway → bot service → **Logs** tab. You should see:

```
Bot v2.1.0 started — webhook URL: https://ce-tracker-bot-production.up.railway.app/webhook
Webhook registered: https://ce-tracker-bot-production.up.railway.app/webhook
Bot server listening on port 8080
[GIN-debug] Listening and serving HTTP on :8080
```

✅ If you see `Webhook registered:` — your bot is ready!
❌ If you see `Warning: failed to set webhook` — check your token

### Step 4.2: Check Worker Service Logs

In Railway → worker service → **Logs** tab. You should see:

```
Worker v2.1.0 started — poll interval: 3600s
Background poll starting
Background poll done
```

### Step 4.3: Test the Health Endpoint

Open in browser:
```
https://ce-tracker-bot-production.up.railway.app/health
```

Should return:
```json
{"status":"ok","version":"2.1.0"}
```

### Step 4.4: Verify Webhook is Set with Telegram

Run this curl command (replace `{TOKEN}` with your actual token):

```bash
curl "https://api.telegram.org/bot{TOKEN}/getWebhookInfo"
```

Should return:
```json
{
  "ok": true,
  "result": {
    "url": "https://ce-tracker-bot-production.up.railway.app/webhook",
    "pending_update_count": 0,
    "max_connections": 40
  }
}
```

✅ `url` matches your Railway domain → success!

---

## Part 5: Test the Bot

### Step 5.1: Send /start

1. Open Telegram, find your bot
2. Send: `/start`
3. Bot should reply:
   ```
   👋 Welcome to CE Express Tracker!
   Send me a tracking code (e.g. TBKH000649682)...
   
   v2.1.0
   ```

### Step 5.2: Send a Tracking Code

1. Send any valid CE Express tracking code: `TBKH000649682`
2. Bot should reply:
   ```
   🔍 Checking TBKH000649682…
   ```
3. Then:
   ```
   📦 CE Express Tracker
   Tracking: TBKH000649682
   Status: Out for Delivery
   ...
   ```

### Step 5.3: Tap a Button

1. Send `/start` again
2. You should see your previous codes as buttons
3. Tap one → bot re-checks (or shows cooldown if checked recently)

### Step 5.4: Wait for Auto-Alert

1. Wait until tracking status changes naturally
2. Within 1 hour (POLL_INTERVAL), worker will detect change
3. You'll receive:
   ```
   🔔 STATUS UPDATE!
   Tracking: TBKH000649682
   Status changed: Out for Delivery → Delivered
   ```

---

## Part 6: Auto-Deploy on Push

Already configured! Every `git push origin main` triggers:

1. Railway detects the push
2. Builds new Docker images for both services
3. Deploys with zero downtime
4. Logs visible in Railway dashboard

### Test Auto-Deploy

```bash
# Make a small change
git commit --allow-empty -m "test auto-deploy"
git push origin main
```

Watch Railway dashboard → both services rebuild → new version live.

---

## Troubleshooting

### Bot doesn't respond to /start

**Check 1: Webhook registered?**
```bash
curl "https://api.telegram.org/bot{TOKEN}/getWebhookInfo"
```
If `url` is empty, webhook isn't set. Restart the bot service to re-register.

**Check 2: Bot service running?**
- Railway → bot service → Deployments → check status (should be "Active")
- View logs for errors

**Check 3: Domain reachable?**
```bash
curl https://your-railway-domain.up.railway.app/health
```
Should return `{"status":"ok","version":"..."}`. If not, domain/service is down.

### Bot responds but no auto-alerts

**Check 1: Worker service running?**
- Railway → worker service → Deployments → check status
- Logs should show "Background poll done" every hour

**Check 2: Shared volume mounted?**
- Both services need volume `tracker-state` mounted at `/data`
- Check Railway → service → Settings → Volumes

### Logs show "failed to set webhook"

**Causes:**
1. **Wrong token**: Verify `TELEGRAM_BOT_TOKEN` in env vars
2. **URL not HTTPS**: Telegram only accepts HTTPS webhooks
3. **URL not public**: Localhost/private IPs don't work

**Fix:**
```bash
# Manually set webhook
curl -X POST "https://api.telegram.org/bot{TOKEN}/setWebhook" \
  -H "Content-Type: application/json" \
  -d '{"url":"https://your-domain.up.railway.app/webhook"}'
```

### "no data found for code" error

The CE Express API didn't return data. Check:
1. Tracking code is valid (try in their website first)
2. CE Express API isn't down

### State not persisting between deploys

**Cause:** Volume not attached properly.

**Fix:**
1. Railway → service → Settings → Volumes
2. Verify `tracker-state` is attached to **both** bot and worker
3. Mount path must be `/data` for both

---

## Cost Estimation (Railway)

| Component | Resources | Est. Cost |
|-----------|-----------|-----------|
| Bot service | 256MB RAM, idle most of time | ~$2/mo |
| Worker service | 128MB RAM, runs 1x per hour | ~$1/mo |
| Volume (1GB) | Persistent storage | ~$0.25/mo |
| **Total** | | **~$3-5/mo** |

Compared to Python version (~$10-15/mo) → **60-70% cheaper**.

---

## Useful Commands

### View webhook info
```bash
curl "https://api.telegram.org/bot{TOKEN}/getWebhookInfo"
```

### Delete webhook (force re-register)
```bash
curl "https://api.telegram.org/bot{TOKEN}/deleteWebhook"
```

### Manually set webhook
```bash
curl -X POST "https://api.telegram.org/bot{TOKEN}/setWebhook" \
  -H "Content-Type: application/json" \
  -d '{"url":"https://your-domain.up.railway.app/webhook"}'
```

### Trigger worker poll manually (locally)
```bash
docker compose run --rm worker
```

### View Railway logs from CLI
```bash
# Install Railway CLI
npm i -g @railway/cli

# Login
railway login

# Link to project
railway link

# Stream logs
railway logs --service bot
railway logs --service worker
```

---

## Architecture Summary

```
┌─────────────────────────────────────────────────────────────┐
│                       Telegram User                          │
└────────────────────┬────────────────────────────────────────┘
                     │
                     │ sends /start or tracking code
                     ▼
┌─────────────────────────────────────────────────────────────┐
│                    Telegram API                              │
└────────────────────┬────────────────────────────────────────┘
                     │
                     │ POST /webhook
                     ▼
┌─────────────────────────────────────────────────────────────┐
│        Railway: Bot Service (Gin HTTP Server)               │
│        - Port 8080                                           │
│        - Receives webhook updates                            │
│        - Routes to handlers (/start, text, callbacks)        │
│        - Calls CE Express API                                │
│        - Sends responses via Telegram API                    │
└────────────────────┬────────────────────────────────────────┘
                     │
                     │ read/write
                     ▼
┌─────────────────────────────────────────────────────────────┐
│       Shared Volume: /data/state.json                        │
│       (Railway persistent volume)                            │
└────────────────────▲────────────────────────────────────────┘
                     │
                     │ read/write
                     │
┌────────────────────┴────────────────────────────────────────┐
│        Railway: Worker Service (Background Poller)          │
│        - Runs every POLL_INTERVAL seconds                    │
│        - Checks all tracking codes for status changes        │
│        - Sends alerts via Telegram API if changed            │
└─────────────────────────────────────────────────────────────┘
```

---

## Next Steps After Deploy

- [ ] Set bot description/about in BotFather
- [ ] Add bot profile picture
- [ ] Share bot link with users
- [ ] Monitor Railway logs first week for any issues
- [ ] Set up Railway alerts (Settings → Notifications)
- [ ] Consider adding `/stop` command to remove tracking codes (future feature)

---

## Support

- Railway docs: https://docs.railway.app
- Telegram Bot API: https://core.telegram.org/bots/api
- CE Express: https://cambodianexpress.com
