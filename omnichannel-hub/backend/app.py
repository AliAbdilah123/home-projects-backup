import os
import sqlite3
import json
import time
import hmac
import hashlib
import threading
import subprocess
import shutil
import queue
import select
from datetime import datetime, timezone
from flask import Flask, request, jsonify, Response
from flask_cors import CORS
from pathlib import Path

DB_PATH = os.environ.get("DB_PATH", "/home/opc/omnichannel-hub/db/hub.db")
WEBHOOK_SECRET = os.environ.get("WEBHOOK_SECRET", "")
ADMIN_TOKEN = os.environ.get("ADMIN_TOKEN", "")

app = Flask(__name__)
CORS(app, resources={r"/api/*": {"origins": "*"}})
_db_init_lock = threading.Lock()
_conn_pool = threading.local()


def utcnow():
    return datetime.now(timezone.utc).isoformat()


def get_db():
    if not hasattr(_conn_pool, "conn") or _conn_pool.conn is None:
        Path(DB_PATH).parent.mkdir(parents=True, exist_ok=True)
        conn = sqlite3.connect(DB_PATH, check_same_thread=False)
        conn.row_factory = sqlite3.Row
        conn.execute("PRAGMA journal_mode=WAL")
        conn.execute("PRAGMA foreign_keys=ON")
        _conn_pool.conn = conn
    return _conn_pool.conn


def init_db():
    with _db_init_lock:
        q = queue.Queue()
        conn = get_db()

        def run_migrations():
            try:
                schema_path = os.path.join(os.path.dirname(__file__), "db", "schema.sql")
                if os.path.exists(schema_path):
                    with open(schema_path, "r") as f:
                        sql = f.read()
                    for stmt in sql.split(";"):
                        s = stmt.strip()
                        if s:
                            conn.execute(s)
                    conn.commit()
                    q.put(("ok", None))
            except Exception as e:
                q.put(("error", str(e)))

        t = threading.Thread(target=run_migrations)
        t.start()
        t.join(timeout=10)
        try:
            status, err = q.get_nowait()
            if status != "ok":
                raise RuntimeError(err)
        except queue.Empty:
            raise RuntimeError("migration timeout")


_init_done = False
_db_init_lock2 = threading.Lock()


def ensure_db():
    global _init_done
    if not _init_done:
        with _db_init_lock2:
            if not _init_done:
                init_db()
                _init_done = True


# Auth helpers

def require_admin():
    token = request.headers.get("Authorization", "")
    if token.startswith("Bearer "):
        token = token[7:]
    return token == ADMIN_TOKEN


def _row_to_channel(row):
    return {
        "id": row["id"],
        "platform": row["platform"],
        "identifier": row["identifier"],
        "name": row["name"],
        "status": row["status"],
        "connected_at": row["connected_at"],
        "created_at": row["created_at"],
        "creds_raw": row["creds_raw"] if "creds_raw" in row.keys() else None,
    }


def _row_to_convo(row):
    return {
        "id": row["id"],
        "channel_id": row["channel_id"],
        "external_id": row["external_id"],
        "contact": row["contact"],
        "display_name": row["display_name"],
        "unread_count": row["unread_count"],
        "last_message_at": row["last_message_at"],
        "created_at": row["created_at"],
    }


def _row_to_message(row):
    return {
        "id": row["id"],
        "conv_id": row["conv_id"],
        "direction": row["direction"],
        "sender": row["sender"],
        "content": row["content"],
        "media_url": row["media_url"],
        "status": row["status"],
        "created_at": row["created_at"],
    }


# Channels

@app.route("/api/v1/channels", methods=["GET"])
def api_list_channels():
    ensure_db()
    conn = get_db()
    rows = conn.execute(
        "SELECT id, platform, identifier, name, status, connected_at, created_at, creds_raw FROM channels ORDER BY created_at DESC"
    ).fetchall()
    return jsonify([_row_to_channel(r) for r in rows])


@app.route("/api/v1/channels", methods=["POST"])
def api_add_channel():
    if not require_admin():
        return jsonify({"detail": "unauthorized"}), 401
    ensure_db()
    data = request.get_json() or {}
    platform = data.get("platform")
    identifier = data.get("identifier")
    name = data.get("name", f"{platform} account")
    creds_raw = data.get("creds_raw")
    if not platform or not identifier:
        return jsonify({"detail": "platform and identifier required"}), 400
    channel_id = f"ch_{int(time.time()*1000)}"
    now = utcnow()
    conn = get_db()
    conn.execute(
        "INSERT INTO channels (id, platform, identifier, name, status, connected_at, created_at, creds_raw) VALUES (?,?,?,?,?,?,?,?)",
        (channel_id, platform, identifier, name, "connected", now, now, creds_raw),
    )
    conn.commit()
    row = conn.execute("SELECT * FROM channels WHERE id=?", (channel_id,)).fetchone()
    return jsonify(_row_to_channel(row)), 201


@app.route("/api/v1/channels/<channel_id>/disconnect", methods=["POST"])
def api_disconnect_channel(channel_id):
    if not require_admin():
        return jsonify({"detail": "unauthorized"}), 401
    ensure_db()
    conn = get_db()
    conn.execute("UPDATE channels SET status='disconnected' WHERE id=?", (channel_id,))
    conn.commit()
    return jsonify({"id": channel_id, "status": "disconnected"})


@app.route("/api/v1/channels/<channel_id>/connect", methods=["POST"])
def api_connect_channel(channel_id):
    if not require_admin():
        return jsonify({"detail": "unauthorized"}), 401
    ensure_db()
    conn = get_db()
    row = conn.execute("SELECT * FROM channels WHERE id=?", (channel_id,)).fetchone()
    if not row:
        return jsonify({"detail": "channel not found"}), 404
    conn.execute(
        "UPDATE channels SET status='connected', connected_at=? WHERE id=?",
        (utcnow(), channel_id),
    )
    conn.commit()
    row = conn.execute("SELECT * FROM channels WHERE id=?", (channel_id,)).fetchone()
    return jsonify(_row_to_channel(row))


@app.route("/api/v1/channels/<channel_id>/logs", methods=["GET"])
def api_channel_logs(channel_id):
    ensure_db()
    limit = int(request.args.get("limit", 200))
    path = f"/home/opc/omnichannel-hub/bridge/logs/{channel_id}.log"
    entries = []
    if os.path.exists(path) and os.path.getsize(path) > 0:
        try:
            with open(path, "r", errors="ignore") as f:
                lines = f.readlines()
            raw = lines[-limit:]
            for line in raw:
                line = line.strip()
                if not line:
                    continue
                try:
                    entries.append(json.loads(line))
                except Exception:
                    entries.append({"raw": line, "parsed": False})
        except Exception as e:
            return jsonify({"detail": str(e)}), 500
    # sort newest last, then reverse to make tail order by timestamp
    entries.sort(key=lambda x: x.get("ts", ""))
    entries.reverse()
    return jsonify(entries)


# Conversations

@app.route("/api/v1/conversations", methods=["GET"])
def api_list_conversations():
    ensure_db()
    channel_id = request.args.get("channel_id")
    limit = int(request.args.get("limit", 100))
    conn = get_db()
    if channel_id:
        rows = conn.execute(
            "SELECT * FROM conversations WHERE channel_id=? ORDER BY last_message_at DESC LIMIT ?",
            (channel_id, limit),
        ).fetchall()
    else:
        rows = conn.execute(
            "SELECT * FROM conversations ORDER BY last_message_at DESC LIMIT ?", (limit,)
        ).fetchall()
    return jsonify([_row_to_convo(r) for r in rows])


@app.route("/api/v1/conversations", methods=["POST"])
def api_upsert_conversation():
    if not require_admin():
        return jsonify({"detail": "unauthorized"}), 401
    ensure_db()
    data = request.get_json() or {}
    channel_id = data.get("channel_id")
    external_id = data.get("external_id")
    contact = data.get("contact")
    display_name = data.get("display_name", contact)
    if not channel_id or not contact:
        return jsonify({"detail": "channel_id and contact required"}), 400
    conv_id = f"cv_{int(time.time()*1000)}"
    now = utcnow()
    conn = get_db()
    existing = conn.execute(
        "SELECT id FROM conversations WHERE channel_id=? AND external_id=?",
        (channel_id, external_id),
    ).fetchone()
    if existing:
        conn.execute(
            "UPDATE conversations SET last_message_at=?, display_name=? WHERE id=?",
            (now, display_name, existing["id"]),
        )
        conn.commit()
        row = conn.execute("SELECT * FROM conversations WHERE id=?", (existing["id"],)).fetchone()
        return jsonify(_row_to_convo(row))
    conn.execute(
        "INSERT INTO conversations (id, channel_id, external_id, contact, display_name, unread_count, last_message_at, created_at) VALUES (?,?,?,?,?,?,?,?)",
        (conv_id, channel_id, external_id, contact, display_name, 0, now, now),
    )
    conn.commit()
    row = conn.execute("SELECT * FROM conversations WHERE id=?", (conv_id,)).fetchone()
    return jsonify(_row_to_convo(row)), 201


# Messages

@app.route("/api/v1/conversations/<conv_id>/messages", methods=["GET"])
def api_list_messages(conv_id):
    ensure_db()
    limit = int(request.args.get("limit", 100))
    conn = get_db()
    rows = conn.execute(
        "SELECT * FROM messages WHERE conv_id=? ORDER BY created_at ASC LIMIT ?",
        (conv_id, limit),
    ).fetchall()
    return jsonify([_row_to_message(r) for r in rows])


@app.route("/api/v1/conversations/<conv_id>/messages", methods=["POST"])
def api_send_message(conv_id):
    if not require_admin():
        return jsonify({"detail": "unauthorized"}), 401
    ensure_db()
    data = request.get_json() or {}
    sender = data.get("sender", "agent")
    content = data.get("content", "")
    media_url = data.get("media_url")
    now = utcnow()
    msg_id = f"msg_{int(time.time()*1000)}"
    conn = get_db()
    conn.execute(
        "INSERT INTO messages (id, conv_id, direction, sender, content, media_url, status, created_at) VALUES (?,?,?,?,?,?,?,?)",
        (msg_id, conv_id, "outbound", sender, content, media_url, "sent", now),
    )
    conn.commit()
    row = conn.execute("SELECT * FROM messages WHERE id=?", (msg_id,)).fetchone()
    return jsonify(_row_to_message(row)), 201


# Stats

@app.route("/api/v1/stats", methods=["GET"])
def api_stats():
    ensure_db()
    conn = get_db()
    channels = conn.execute("SELECT COUNT(1) as n FROM channels").fetchone()["n"]
    convos = conn.execute("SELECT COUNT(1) as n FROM conversations").fetchone()["n"]
    messages = conn.execute("SELECT COUNT(1) as n FROM messages").fetchone()["n"]
    return jsonify(
        {
            "channels": channels,
            "conversations": convos,
            "messages": messages,
        }
    )


# Webhook / ingest entrypoint for external bridges

@app.route("/api/v1/ingest", methods=["POST"])
def api_ingest():
    ensure_db()
    payload = request.get_data(as_text=True)
    sig = request.headers.get("X-Hub-Signature-256", "")
    if WEBHOOK_SECRET:
        expected = hmac.new(
            WEBHOOK_SECRET.encode(), payload.encode(), hashlib.sha256
        ).hexdigest()
        if not hmac.compare_digest(expected, sig):
            return jsonify({"detail": "bad signature"}), 401
    try:
        event = json.loads(payload)
    except Exception:
        return jsonify({"detail": "invalid json"}), 400

    event_type = event.get("type")
    conn = get_db()
    now = utcnow()

    if event_type == "message.inbound":
        channel_id = event["channel_id"]
        external_conv = event["conversation_id"]
        sender = event.get("sender", "user")
        content = event.get("content", "")
        media_url = event.get("media_url")
        contact = event.get("contact", external_conv)
        display_name = event.get("display_name", contact)

        row = conn.execute(
            "SELECT id FROM conversations WHERE channel_id=? AND external_id=?",
            (channel_id, external_conv),
        ).fetchone()
        if row:
            conv_id = row["id"]
            conn.execute(
                "UPDATE conversations SET last_message_at=?, display_name=? WHERE id=?",
                (now, display_name, conv_id),
            )
        else:
            conv_id = f"cv_{int(time.time()*1000)}"
            conn.execute(
                "INSERT INTO conversations (id, channel_id, external_id, contact, display_name, unread_count, last_message_at, created_at) VALUES (?,?,?,?,?,?,?,?)",
                (conv_id, channel_id, external_conv, contact, display_name, 1, now, now),
            )

        msg_id = f"msg_{int(time.time()*1000)}"
        conn.execute(
            "INSERT INTO messages (id, conv_id, direction, sender, content, media_url, status, created_at) VALUES (?,?,?,?,?,?,?,?)",
            (msg_id, conv_id, "inbound", sender, content, media_url, "received", now),
        )
        conn.commit()
        return jsonify({"status": "ingested", "conv_id": conv_id, "msg_id": msg_id}), 202

    return jsonify({"detail": "unsupported event type"}), 400


@app.route("/", methods=["GET"])
def root_health():
    return jsonify({"app": "omnichannel-hub", "status": "ok"})


if __name__ == "__main__":
    ensure_db()
    port = int(os.environ.get("PORT", 8080))
    app.run(host="0.0.0.0", port=port, debug=False)
