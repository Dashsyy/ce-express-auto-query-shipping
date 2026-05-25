#!/usr/bin/env python3
import os
import json
import asyncio
import logging
import time
from pathlib import Path
from datetime import datetime

import requests
from dotenv import load_dotenv
from telegram import Update, InlineKeyboardButton, InlineKeyboardMarkup
from telegram.ext import (
    Application,
    CommandHandler,
    MessageHandler,
    CallbackQueryHandler,
    filters,
    ContextTypes,
)

load_dotenv()

logging.basicConfig(format="%(asctime)s %(levelname)s %(message)s", level=logging.INFO)
log = logging.getLogger(__name__)

TOKEN = os.getenv("TELEGRAM_BOT_TOKEN", "")
POLL_INTERVAL = int(os.getenv("POLL_INTERVAL", "3600"))
STATE_FILE = Path(os.getenv("STATE_FILE", "state.json"))

SHIPMENT_STATUS = {
    "10": "Created",
    "20": "Pickup Assigned",
    "30": "Picked Up / In Transit",
    "40": "Out for Delivery",
    "50": "Delivery Attempted",
    "60": "Delivered",
    "70": "Exception",
    "80": "Returned",
}
DELIVERED_EVENT_CODES = {"70", "DELIVERED", "POD"}


# ── State ──────────────────────────────────────────────

def load_state() -> dict:
    if STATE_FILE.exists():
        return json.loads(STATE_FILE.read_text())
    return {"chats": {}}


def save_state(state: dict) -> None:
    STATE_FILE.parent.mkdir(parents=True, exist_ok=True)
    STATE_FILE.write_text(json.dumps(state, indent=2))


def get_chat(state: dict, chat_id: str) -> dict:
    return state["chats"].setdefault(chat_id, {"codes": [], "code_states": {}})


# ── CE Express API ─────────────────────────────────

def _fetch_sync(code: str) -> dict | None:
    ts = int(time.time() * 1000)
    url = f"https://cp.cambodianexpress.com/api/public/shipment/track?code={code}&_={ts}"
    resp = requests.get(url, timeout=30, headers={"User-Agent": "Mozilla/5.0"})
    resp.raise_for_status()
    data = resp.json()
    return data["data"] if data.get("success") else None


async def fetch_tracking(code: str) -> dict | None:
    return await asyncio.to_thread(_fetch_sync, code)


def is_delivered(status: str, events: list) -> bool:
    if status == "60":
        return True
    for ev in events:
        if ev.get("eventCode") in DELIVERED_EVENT_CODES:
            return True
        desc = (ev.get("status") or "").upper()
        if "DELIVER" in desc and "ASSIGN" not in desc:
            if ev.get("eventCode") not in {"60"} or "POD" in desc:
                return True
    return False


def fmt_event(ev: dict) -> str:
    t = ev.get("eventTime", "").replace("T", " ")
    desc = ev.get("trackingEventDesc") or ev.get("status", "")
    shop = ev.get("eventShop", "")
    return f"  {t}{f' [{shop}]' if shop else ''}: {desc}"


def build_message(code: str, shipment: dict, events: list, delivered: bool) -> str:
    sc = shipment.get("shipmentStatus", "")
    label = SHIPMENT_STATUS.get(sc, f"Status {sc}")
    dest = shipment.get("destAddrAll") or shipment.get("destAddress", "")
    weight = shipment.get("totalWeight", 0)

    lines = ["✅ <b>PACKAGE DELIVERED!</b>" if delivered else "\U0001f4e6 <b>CE Express Tracker</b>"]
    lines += [f"Tracking: <code>{code}</code>", f"Status: <b>{label}</b>"]
    if dest:
        lines.append(f"Destination: {dest}")
    if weight:
        lines.append(f"Weight: {weight} kg")
    if events:
        lines += ["", "<b>Latest events:</b>"] + [fmt_event(e) for e in events[-4:]]
    if not delivered and sc == "40" and events:
        last = events[-1]
        courier, mobile = last.get("operatorName", ""), last.get("operatorMobile", "")
        if courier:
            lines += ["", f"Courier: <b>{courier}</b>  \U0001f4de {mobile}"]
    return "\n".join(lines)


# ── Helpers ─────────────────────────────────────────────

def codes_keyboard(codes: list) -> InlineKeyboardMarkup:
    rows = []
    for i in range(0, len(codes), 2):
        rows.append([InlineKeyboardButton(c, callback_data=f"track:{c}") for c in codes[i : i + 2]])
    return InlineKeyboardMarkup(rows)


async def fetch_and_reply(code: str, reply_fn) -> dict | None:
    """Query CE Express, send a formatted reply, return the new state entry or None on error."""
    try:
        data = await fetch_tracking(code)
    except Exception as e:
        await reply_fn(f"❌ Could not fetch <code>{code}</code>: {e}", parse_mode="HTML")
        return None

    if not data or not data.get("shipmentList"):
        await reply_fn(
            f"❌ No data found for <code>{code}</code>. Double-check the tracking code.",
            parse_mode="HTML",
        )
        return None

    si = data["shipmentList"][0]
    shipment = si.get("shipment", {})
    events = si.get("fullEventList", [])
    last_ev = events[-1] if events else {}
    sc = shipment.get("shipmentStatus", "")
    delivered = is_delivered(sc, events)

    await reply_fn(build_message(code, shipment, events, delivered), parse_mode="HTML")

    return {
        "shipmentStatus": sc,
        "lastEventCode": last_ev.get("eventCode", ""),
        "lastEventTime": last_ev.get("eventTime", ""),
        "delivered": delivered,
        "checkedAt": datetime.now().isoformat(),
    }


# ── Handlers ────────────────────────────────────────────

async def cmd_start(update: Update, context: ContextTypes.DEFAULT_TYPE) -> None:
    chat_id = str(update.effective_chat.id)
    state = load_state()
    codes = get_chat(state, chat_id).get("codes", [])

    if codes:
        await update.message.reply_text(
            "\U0001f44b Welcome back!\n\nTap a previous code to re-check, or send a new tracking code:",
            reply_markup=codes_keyboard(codes),
        )
    else:
        await update.message.reply_text(
            "\U0001f44b Welcome to <b>CE Express Tracker</b>!\n\n"
            "Send me a tracking code (e.g. <code>TBKH000649682</code>) and I'll check its "
            "status and alert you automatically whenever it changes.",
            parse_mode="HTML",
        )


async def handle_text(update: Update, context: ContextTypes.DEFAULT_TYPE) -> None:
    code = update.message.text.strip().upper()
    chat_id = str(update.effective_chat.id)

    await update.message.reply_text(f"\U0001f50d Checking <code>{code}</code>…", parse_mode="HTML")

    new_state = await fetch_and_reply(code, update.message.reply_text)
    if new_state:
        state = load_state()
        chat = get_chat(state, chat_id)
        if code not in chat["codes"]:
            chat["codes"].append(code)
        chat["code_states"][code] = new_state
        save_state(state)


async def handle_button(update: Update, context: ContextTypes.DEFAULT_TYPE) -> None:
    q = update.callback_query
    await q.answer()
    if not q.data.startswith("track:"):
        return

    code = q.data.split("track:", 1)[1]
    chat_id = str(update.effective_chat.id)

    await q.message.reply_text(f"\U0001f50d Checking <code>{code}</code>…", parse_mode="HTML")

    new_state = await fetch_and_reply(code, q.message.reply_text)
    if new_state:
        state = load_state()
        chat = get_chat(state, chat_id)
        chat.setdefault("code_states", {})[code] = new_state
        save_state(state)


# ── Background poller ──────────────────────────────────────

async def poll_all(context: ContextTypes.DEFAULT_TYPE) -> None:
    state = load_state()
    log.info("Background poll starting")

    for chat_id, chat in state.get("chats", {}).items():
        for code in list(chat.get("codes", [])):
            prev = chat.get("code_states", {}).get(code, {})
            try:
                data = await fetch_tracking(code)
            except Exception as e:
                log.warning(f"[{code}] fetch error: {e}")
                continue

            if not data or not data.get("shipmentList"):
                continue

            si = data["shipmentList"][0]
            shipment = si.get("shipment", {})
            events = si.get("fullEventList", [])
            last_ev = events[-1] if events else {}
            sc = shipment.get("shipmentStatus", "")
            delivered = is_delivered(sc, events)

            new_state = {
                "shipmentStatus": sc,
                "lastEventCode": last_ev.get("eventCode", ""),
                "lastEventTime": last_ev.get("eventTime", ""),
                "delivered": delivered,
                "checkedAt": datetime.now().isoformat(),
            }

            changed = sc != prev.get("shipmentStatus", "") or last_ev.get("eventCode", "") != prev.get("lastEventCode", "")
            if changed:
                log.info(f"[{code}] change detected — notifying chat {chat_id}")
                try:
                    msg = build_message(code, shipment, events, delivered)
                    await context.bot.send_message(chat_id=int(chat_id), text=msg, parse_mode="HTML")
                except Exception as e:
                    log.error(f"[{code}] failed to notify {chat_id}: {e}")

            chat.setdefault("code_states", {})[code] = new_state

    save_state(state)
    log.info("Background poll done")


# ── Main ────────────────────────────────────────────────

def main() -> None:
    if not TOKEN:
        raise SystemExit("TELEGRAM_BOT_TOKEN not set in .env")

    app = Application.builder().token(TOKEN).build()
    app.add_handler(CommandHandler("start", cmd_start))
    app.add_handler(CallbackQueryHandler(handle_button))
    app.add_handler(MessageHandler(filters.TEXT & ~filters.COMMAND, handle_text))
    app.job_queue.run_repeating(poll_all, interval=POLL_INTERVAL, first=POLL_INTERVAL)

    log.info(f"Bot started — poll interval: {POLL_INTERVAL}s")
    app.run_polling(drop_pending_updates=True)


if __name__ == "__main__":
    main()
