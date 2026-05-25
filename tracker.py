#!/usr/bin/env python3
import os
import json
import time
import argparse
import requests
from datetime import datetime
from pathlib import Path
from dotenv import load_dotenv

load_dotenv()

TELEGRAM_BOT_TOKEN = os.getenv("TELEGRAM_BOT_TOKEN", "")
TELEGRAM_CHAT_ID = os.getenv("TELEGRAM_CHAT_ID", "")
TRACKING_CODES = [c.strip() for c in os.getenv("TRACKING_CODES", "").split(",") if c.strip()]
POLL_INTERVAL = int(os.getenv("POLL_INTERVAL", "3600"))
STATE_FILE = Path(os.getenv("STATE_FILE", "state.json"))

# Shipment status codes observed from the CE Express API
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

# Event codes that signal the package has been delivered
DELIVERED_EVENT_CODES = {"70", "DELIVERED", "POD"}


def load_state() -> dict:
    if STATE_FILE.exists():
        return json.loads(STATE_FILE.read_text())
    return {}


def save_state(state: dict) -> None:
    STATE_FILE.write_text(json.dumps(state, indent=2))


def fetch_tracking(code: str) -> dict | None:
    ts = int(time.time() * 1000)
    url = f"https://cp.cambodianexpress.com/api/public/shipment/track?code={code}&_={ts}"
    resp = requests.get(url, timeout=30, headers={"User-Agent": "Mozilla/5.0"})
    resp.raise_for_status()
    data = resp.json()
    if not data.get("success"):
        return None
    return data["data"]


def send_telegram(message: str) -> None:
    url = f"https://api.telegram.org/bot{TELEGRAM_BOT_TOKEN}/sendMessage"
    resp = requests.post(
        url,
        json={"chat_id": TELEGRAM_CHAT_ID, "text": message, "parse_mode": "HTML"},
        timeout=10,
    )
    resp.raise_for_status()


def format_event_line(event: dict) -> str:
    time_str = event.get("eventTime", "").replace("T", " ")
    desc = event.get("trackingEventDesc") or event.get("status", "")
    shop = event.get("eventShop", "")
    shop_part = f" [{shop}]" if shop else ""
    return f"  {time_str}{shop_part}: {desc}"


def is_delivered(shipment_status: str, events: list[dict]) -> bool:
    if shipment_status == "60":
        return True
    for ev in events:
        if ev.get("eventCode") in DELIVERED_EVENT_CODES:
            return True
        desc = (ev.get("status") or "").upper()
        if "DELIVER" in desc and "ASSIGN" not in desc:
            if ev.get("eventCode") not in {"60"} or "POD" in desc:
                return True
    return False


def build_message(code: str, shipment: dict, events: list[dict], delivered: bool) -> str:
    status_code = shipment.get("shipmentStatus", "")
    status_label = SHIPMENT_STATUS.get(status_code, f"Status {status_code}")
    dest = shipment.get("destAddrAll") or shipment.get("destAddress", "")
    weight = shipment.get("totalWeight", 0)

    lines = []
    if delivered:
        lines.append("✅ <b>PACKAGE DELIVERED!</b>")
    else:
        lines.append("\U0001f4e6 <b>CE Express Update</b>")

    lines += [
        f"Tracking: <code>{code}</code>",
        f"Status: <b>{status_label}</b>",
    ]
    if dest:
        lines.append(f"Destination: {dest}")
    if weight:
        lines.append(f"Weight: {weight} kg")

    if events:
        lines.append("")
        lines.append("<b>Latest events:</b>")
        for ev in events[-4:]:
            lines.append(format_event_line(ev))

    if not delivered and status_code == "40" and events:
        last = events[-1]
        courier = last.get("operatorName", "")
        mobile = last.get("operatorMobile", "")
        if courier:
            lines.append("")
            lines.append(f"Courier: <b>{courier}</b>  \U0001f4de {mobile}")
            shop_phone = "0965018377"
            lines.append(f"Shop: {shop_phone}")

    return "\n".join(lines)


def check_code(code: str, state: dict, notify: bool = True) -> None:
    try:
        data = fetch_tracking(code)
    except Exception as e:
        print(f"[{code}] Fetch error: {e}")
        return

    if not data:
        print(f"[{code}] API returned no data")
        return

    shipment_list = data.get("shipmentList", [])
    if not shipment_list:
        print(f"[{code}] No shipment list")
        return

    shipment_info = shipment_list[0]
    shipment = shipment_info.get("shipment", {})
    events = shipment_info.get("fullEventList", [])

    current_status = shipment.get("shipmentStatus", "")
    last_event = events[-1] if events else {}
    last_event_code = last_event.get("eventCode", "")
    last_event_time = last_event.get("eventTime", "")

    prev = state.get(code, {})
    prev_status = prev.get("shipmentStatus", "")
    prev_event_code = prev.get("lastEventCode", "")

    delivered = is_delivered(current_status, events)
    changed = current_status != prev_status or last_event_code != prev_event_code

    if not changed:
        status_label = SHIPMENT_STATUS.get(current_status, current_status)
        print(f"[{code}] No change — {status_label} ({current_status})")
    else:
        status_label = SHIPMENT_STATUS.get(current_status, current_status)
        print(f"[{code}] Status changed: {prev_status} → {current_status} ({status_label})")

        if notify:
            message = build_message(code, shipment, events, delivered)
            try:
                send_telegram(message)
                print(f"[{code}] Telegram notification sent")
            except Exception as e:
                print(f"[{code}] Telegram error: {e}")

    state[code] = {
        "shipmentStatus": current_status,
        "lastEventCode": last_event_code,
        "lastEventTime": last_event_time,
        "delivered": delivered,
        "checkedAt": datetime.now().isoformat(),
    }


def main() -> None:
    parser = argparse.ArgumentParser(description="CE Express shipment tracker")
    parser.add_argument("--once", action="store_true", help="Run one check then exit")
    parser.add_argument(
        "--codes",
        help="Comma-separated tracking codes (overrides TRACKING_CODES env var)",
    )
    args = parser.parse_args()

    codes = [c.strip() for c in args.codes.split(",") if c.strip()] if args.codes else TRACKING_CODES

    if not codes:
        print("No tracking codes configured. Set TRACKING_CODES in .env or pass --codes")
        return
    if not TELEGRAM_BOT_TOKEN or not TELEGRAM_CHAT_ID:
        print("TELEGRAM_BOT_TOKEN and TELEGRAM_CHAT_ID must be set in .env")
        return

    print(f"CE Express Tracker — codes: {codes}  interval: {POLL_INTERVAL}s")

    while True:
        state = load_state()
        for code in codes:
            check_code(code, state)
        save_state(state)

        if args.once:
            break
        print(f"Next check in {POLL_INTERVAL}s …")
        time.sleep(POLL_INTERVAL)


if __name__ == "__main__":
    main()
