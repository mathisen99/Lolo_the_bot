"""
Reminder tool implementation.

Allows users to set time-based and event-based reminders.
Supports:
- Time-based reminders (deliver at a specific time, with online-check)
- On-join reminders (deliver when target user joins the channel)
- Recurring reminders (daily/weekly)
- Listing and cancelling reminders

Reminders are persisted in SQLite. A background scheduler checks for due
time-based reminders and delivers them via the Go bot callback server.
"""

import os
import sqlite3
import threading
import time as time_module
from datetime import datetime, timedelta
from pathlib import Path
from typing import Any, Dict, Optional, List
from .base import Tool
from api.utils.output import log_info, log_success, log_error, log_warning

import requests


class ReminderTool(Tool):
    """Tool for managing reminders."""

    DB_PATH = Path(__file__).parent.parent.parent / "data" / "reminders.db"
    GO_BOT_CALLBACK_URL = os.getenv("GO_BOT_CALLBACK_URL", "http://localhost:8001")

    # Scheduler singleton
    _scheduler_started = False
    _scheduler_lock = threading.Lock()

    def __init__(self):
        """Initialize reminder tool and start background scheduler."""
        self._init_database()
        self._start_scheduler()

    @property
    def name(self) -> str:
        return "reminder"

    def _init_database(self) -> None:
        """Initialize the SQLite database."""
        self.DB_PATH.parent.mkdir(parents=True, exist_ok=True)

        conn = sqlite3.connect(str(self.DB_PATH))
        cursor = conn.cursor()

        cursor.execute("""
            CREATE TABLE IF NOT EXISTS reminders (
                id INTEGER PRIMARY KEY AUTOINCREMENT,
                creator_nick TEXT NOT NULL,
                target_nick TEXT NOT NULL,
                channel TEXT NOT NULL,
                message TEXT NOT NULL,
                reminder_type TEXT NOT NULL,
                deliver_at TEXT,
                recurrence TEXT,
                status TEXT DEFAULT 'pending',
                created_at TEXT NOT NULL,
                delivered_at TEXT,
                delivery_attempts INTEGER DEFAULT 0,
                expires_at TEXT
            )
        """)

        # Index for scheduler queries
        cursor.execute("""
            CREATE INDEX IF NOT EXISTS idx_reminders_pending_time
            ON reminders (status, reminder_type, deliver_at)
        """)
        cursor.execute("""
            CREATE INDEX IF NOT EXISTS idx_reminders_pending_join
            ON reminders (status, reminder_type, target_nick, channel)
        """)

        conn.commit()
        conn.close()

    def _start_scheduler(self) -> None:
        """Start the background scheduler thread (singleton)."""
        with ReminderTool._scheduler_lock:
            if ReminderTool._scheduler_started:
                return
            ReminderTool._scheduler_started = True

        thread = threading.Thread(target=self._scheduler_loop, daemon=True)
        thread.start()
        log_info("Reminder scheduler started")

    def _scheduler_loop(self) -> None:
        """Background loop that checks for due time-based reminders every 15 seconds."""
        # Initial delay to let the bot fully start
        time_module.sleep(10)

        while True:
            try:
                self._process_due_reminders()
            except Exception as e:
                log_error(f"Reminder scheduler error: {e}")
            time_module.sleep(15)

    def _process_due_reminders(self) -> None:
        """Find and deliver all due time-based reminders."""
        now = datetime.utcnow().isoformat()

        conn = sqlite3.connect(str(self.DB_PATH))
        conn.row_factory = sqlite3.Row
        cursor = conn.cursor()

        # Get due time-based reminders
        cursor.execute("""
            SELECT * FROM reminders
            WHERE status = 'pending'
              AND reminder_type = 'time'
              AND deliver_at <= ?
            ORDER BY deliver_at ASC
            LIMIT 20
        """, (now,))

        reminders = cursor.fetchall()
        conn.close()

        for rem in reminders:
            self._attempt_delivery(dict(rem))

    def _attempt_delivery(self, reminder: dict) -> None:
        """Attempt to deliver a reminder. Checks if user is online first."""
        rid = reminder["id"]
        target = reminder["target_nick"]
        channel = reminder["channel"]
        creator = reminder["creator_nick"]
        message = reminder["message"]
        recurrence = reminder.get("recurrence")

        # Check if target user is online in the channel
        online = self._check_user_online(target, channel)

        if online:
            # Deliver it
            if creator == target:
                irc_msg = f"{target}: Reminder: {message}"
            else:
                irc_msg = f"{target}: Reminder from {creator}: {message}"

            success = self._send_to_irc(channel, irc_msg)

            if success:
                log_success(f"[REMINDER] Delivered #{rid} to {target} in {channel}")
                if recurrence:
                    self._reschedule_recurring(rid, recurrence)
                else:
                    self._mark_delivered(rid)
            else:
                self._increment_attempts(rid)
        else:
            # User is offline — convert to on-join delivery if not too many attempts
            conn = sqlite3.connect(str(self.DB_PATH))
            cursor = conn.cursor()
            attempts = reminder.get("delivery_attempts", 0) + 1

            if attempts >= 10:
                # Too many attempts, mark as failed
                cursor.execute(
                    "UPDATE reminders SET status = 'failed', delivery_attempts = ? WHERE id = ?",
                    (attempts, rid)
                )
                log_warning(f"[REMINDER] #{rid} failed after {attempts} attempts (user never online)")
            else:
                # Keep as pending, increment attempts — scheduler will retry
                cursor.execute(
                    "UPDATE reminders SET delivery_attempts = ? WHERE id = ?",
                    (attempts, rid)
                )

            conn.commit()
            conn.close()

    def _check_user_online(self, nick: str, channel: str) -> bool:
        """Check if a user is currently in a channel via Go bot callback."""
        try:
            resp = requests.post(
                f"{self.GO_BOT_CALLBACK_URL}/irc/execute",
                json={"command": "user_status", "args": [channel, nick]},
                timeout=5
            )
            if resp.status_code == 200:
                data = resp.json()
                # If user_status returns success and doesn't say "not in channel"
                if data.get("status") == "success":
                    output = data.get("output", "")
                    if "not in channel" in output.lower() or "not tracked" in output.lower():
                        return False
                    return True
            return False
        except Exception as e:
            log_error(f"[REMINDER] Failed to check user online status: {e}")
            return False

    def _send_to_irc(self, channel: str, message: str) -> bool:
        """Send a message to IRC via Go bot callback."""
        try:
            resp = requests.post(
                f"{self.GO_BOT_CALLBACK_URL}/irc/execute",
                json={"command": "send_message", "args": [channel, message]},
                timeout=10
            )
            return resp.status_code == 200 and resp.json().get("status") == "success"
        except Exception as e:
            log_error(f"[REMINDER] Failed to send to IRC: {e}")
            return False

    def _mark_delivered(self, reminder_id: int) -> None:
        """Mark a reminder as delivered."""
        conn = sqlite3.connect(str(self.DB_PATH))
        cursor = conn.cursor()
        cursor.execute(
            "UPDATE reminders SET status = 'delivered', delivered_at = ? WHERE id = ?",
            (datetime.utcnow().isoformat(), reminder_id)
        )
        conn.commit()
        conn.close()

    def _increment_attempts(self, reminder_id: int) -> None:
        """Increment delivery attempt counter."""
        conn = sqlite3.connect(str(self.DB_PATH))
        cursor = conn.cursor()
        cursor.execute(
            "UPDATE reminders SET delivery_attempts = delivery_attempts + 1 WHERE id = ?",
            (reminder_id,)
        )
        conn.commit()
        conn.close()

    def _reschedule_recurring(self, reminder_id: int, recurrence: str) -> None:
        """Reschedule a recurring reminder for the next occurrence."""
        conn = sqlite3.connect(str(self.DB_PATH))
        cursor = conn.cursor()

        cursor.execute("SELECT deliver_at FROM reminders WHERE id = ?", (reminder_id,))
        row = cursor.fetchone()
        if not row:
            conn.close()
            return

        current_time = datetime.fromisoformat(row[0])

        if recurrence == "daily":
            next_time = current_time + timedelta(days=1)
        elif recurrence == "weekly":
            next_time = current_time + timedelta(weeks=1)
        elif recurrence == "hourly":
            next_time = current_time + timedelta(hours=1)
        else:
            # Unknown recurrence, just mark delivered
            self._mark_delivered(reminder_id)
            conn.close()
            return

        cursor.execute(
            "UPDATE reminders SET deliver_at = ?, delivered_at = ?, delivery_attempts = 0 WHERE id = ?",
            (next_time.isoformat(), datetime.utcnow().isoformat(), reminder_id)
        )
        conn.commit()
        conn.close()
        log_info(f"[REMINDER] #{reminder_id} rescheduled ({recurrence}) to {next_time.isoformat()}")

    def check_join_reminders(self, nick: str, channel: str) -> List[str]:
        """
        Check for pending on-join reminders for a user in a channel.
        Called by the Go bot via API when a user JOINs.
        Returns list of messages to deliver.
        """
        conn = sqlite3.connect(str(self.DB_PATH))
        conn.row_factory = sqlite3.Row
        cursor = conn.cursor()

        # Case-insensitive nick match
        cursor.execute("""
            SELECT * FROM reminders
            WHERE status = 'pending'
              AND reminder_type = 'join'
              AND LOWER(target_nick) = LOWER(?)
              AND LOWER(channel) = LOWER(?)
            ORDER BY created_at ASC
        """, (nick, channel))

        reminders = cursor.fetchall()
        messages = []

        for rem in reminders:
            creator = rem["creator_nick"]
            message = rem["message"]

            if creator.lower() == rem["target_nick"].lower():
                irc_msg = f"{nick}: Reminder: {message}"
            else:
                irc_msg = f"{nick}: Reminder from {creator}: {message}"

            messages.append(irc_msg)

            # Mark as delivered
            cursor.execute(
                "UPDATE reminders SET status = 'delivered', delivered_at = ? WHERE id = ?",
                (datetime.utcnow().isoformat(), rem["id"])
            )

        conn.commit()
        conn.close()

        if messages:
            log_success(f"[REMINDER] Delivering {len(messages)} on-join reminder(s) to {nick} in {channel}")

        return messages

    # ---- Tool interface methods ----

    def get_definition(self) -> Dict[str, Any]:
        """Get tool definition for OpenAI API."""
        return {
            "type": "function",
            "name": "reminder",
            "description": """Manage reminders for IRC users. Supports time-based and event-based reminders.

REMINDER TYPES:
- "time": Deliver at a specific time. If user is offline, retries until they come online (up to 10 attempts).
- "join": Deliver when the target user next joins the channel (or speaks if already present but was away).
- "recurring": Like time-based but repeats (daily, weekly, hourly). Use recurrence parameter.

ACTIONS:
- "create": Set a new reminder
- "list": List pending reminders (your own, or all if admin/owner)
- "cancel": Cancel a pending reminder by ID
- "check": Check your pending reminders count

EXAMPLES:
- "Remind me in 30 seconds to check tea" → create, type=time, target=self, deliver_at=+30s
- "Remind me in 1 hour to make food" → create, type=time, target=self, deliver_at=+1h
- "Remind User2 when he joins to ping me" → create, type=join, target=User2
- "Remind me every day at 14:00 to check logs" → create, type=time, recurrence=daily
- "Cancel reminder #3" → cancel, reminder_id=3
- "What reminders do I have?" → list

TIME FORMAT for deliver_at:
- Relative: "+30s", "+30m", "+2h", "+1d" (seconds, minutes, hours, days from now)
- Absolute: "2025-03-15T14:00:00" (ISO 8601 UTC)

IMPORTANT: The deliver_at time should be in UTC. Convert from the user's context if needed.""",
            "parameters": {
                "type": "object",
                "properties": {
                    "action": {
                        "type": "string",
                        "enum": ["create", "list", "cancel", "check"],
                        "description": "Action to perform"
                    },
                    "reminder_type": {
                        "type": "string",
                        "enum": ["time", "join", "recurring"],
                        "description": "Type of reminder (required for 'create')"
                    },
                    "target_nick": {
                        "type": "string",
                        "description": "Nick to remind. Defaults to the requesting user if omitted."
                    },
                    "message": {
                        "type": "string",
                        "description": "The reminder message (required for 'create')"
                    },
                    "deliver_at": {
                        "type": "string",
                        "description": "When to deliver. Relative: '+30s', '+5m', '+2h', '+1d'. Absolute: ISO 8601 UTC. Required for time/recurring types."
                    },
                    "recurrence": {
                        "type": "string",
                        "enum": ["hourly", "daily", "weekly"],
                        "description": "Recurrence interval (only for 'recurring' type)"
                    },
                    "reminder_id": {
                        "type": "integer",
                        "description": "Reminder ID (required for 'cancel')"
                    },
                    "channel": {
                        "type": "string",
                        "description": "Channel for the reminder (auto-filled from context)"
                    }
                },
                "required": ["action"],
                "additionalProperties": False
            }
        }

    def _parse_deliver_at(self, deliver_at: str) -> Optional[datetime]:
        """Parse a deliver_at string into a datetime."""
        if not deliver_at:
            return None

        deliver_at = deliver_at.strip()

        # Relative time: +30s, +30m, +2h, +1d, +1w
        if deliver_at.startswith("+"):
            try:
                value = int(deliver_at[1:-1])
                unit = deliver_at[-1].lower()
                now = datetime.utcnow()

                if unit == "s":
                    return now + timedelta(seconds=value)
                elif unit == "m":
                    return now + timedelta(minutes=value)
                elif unit == "h":
                    return now + timedelta(hours=value)
                elif unit == "d":
                    return now + timedelta(days=value)
                elif unit == "w":
                    return now + timedelta(weeks=value)
                else:
                    return None
            except (ValueError, IndexError):
                return None

        # Absolute ISO 8601
        try:
            return datetime.fromisoformat(deliver_at.replace("Z", "+00:00").replace("+00:00", ""))
        except ValueError:
            return None

    def _create_reminder(
        self, creator: str, target: str, channel: str, message: str,
        reminder_type: str, deliver_at: Optional[str], recurrence: Optional[str]
    ) -> str:
        """Create a new reminder."""
        if not message or len(message.strip()) < 2:
            return "Error: Reminder message is too short."

        if not channel:
            return "Error: Channel is required for reminders."

        # Default target to creator
        if not target:
            target = creator

        # Validate type-specific requirements
        if reminder_type in ("time", "recurring"):
            if not deliver_at:
                return "Error: deliver_at is required for time-based reminders."
            parsed_time = self._parse_deliver_at(deliver_at)
            if not parsed_time:
                return f"Error: Could not parse deliver_at '{deliver_at}'. Use '+30m', '+2h', '+1d', or ISO 8601 format."
            if parsed_time < datetime.utcnow():
                return "Error: deliver_at is in the past."
            deliver_at_iso = parsed_time.isoformat()
        else:
            deliver_at_iso = None

        if reminder_type == "recurring" and not recurrence:
            return "Error: recurrence is required for recurring reminders (hourly, daily, weekly)."

        # Set expiry: 30 days for on-join, 1 year for recurring, none for one-time
        if reminder_type == "join":
            expires_at = (datetime.utcnow() + timedelta(days=30)).isoformat()
        elif reminder_type == "recurring":
            expires_at = (datetime.utcnow() + timedelta(days=365)).isoformat()
        else:
            expires_at = None

        now = datetime.utcnow().isoformat()

        conn = sqlite3.connect(str(self.DB_PATH))
        cursor = conn.cursor()

        # Limit: max 20 pending reminders per user
        cursor.execute(
            "SELECT COUNT(*) FROM reminders WHERE LOWER(creator_nick) = LOWER(?) AND status = 'pending'",
            (creator,)
        )
        count = cursor.fetchone()[0]
        if count >= 20:
            conn.close()
            return "Error: You have too many pending reminders (max 20). Cancel some first."

        cursor.execute("""
            INSERT INTO reminders
            (creator_nick, target_nick, channel, message, reminder_type, deliver_at, recurrence, status, created_at, expires_at)
            VALUES (?, ?, ?, ?, ?, ?, ?, 'pending', ?, ?)
        """, (creator, target, channel, message.strip(), reminder_type, deliver_at_iso, recurrence, now, expires_at))

        rid = cursor.lastrowid
        conn.commit()
        conn.close()

        log_success(f"[REMINDER] #{rid} created by {creator} for {target} ({reminder_type})")

        # Build confirmation
        if reminder_type == "join":
            return f"Reminder #{rid} set! I'll remind {target} when they join {channel}: \"{message.strip()}\""
        elif reminder_type == "recurring":
            return f"Recurring reminder #{rid} set ({recurrence})! First delivery at {deliver_at_iso} UTC for {target}: \"{message.strip()}\""
        else:
            return f"Reminder #{rid} set for {deliver_at_iso} UTC! I'll remind {target} in {channel}: \"{message.strip()}\""

    def _list_reminders(self, nick: str, permission_level: str) -> str:
        """List pending reminders."""
        conn = sqlite3.connect(str(self.DB_PATH))
        conn.row_factory = sqlite3.Row
        cursor = conn.cursor()

        if permission_level in ("owner", "admin"):
            cursor.execute(
                "SELECT * FROM reminders WHERE status = 'pending' ORDER BY created_at DESC LIMIT 25"
            )
        else:
            cursor.execute(
                "SELECT * FROM reminders WHERE status = 'pending' AND (LOWER(creator_nick) = LOWER(?) OR LOWER(target_nick) = LOWER(?)) ORDER BY created_at DESC LIMIT 15",
                (nick, nick)
            )

        reminders = cursor.fetchall()
        conn.close()

        if not reminders:
            return "No pending reminders found."

        lines = []
        for r in reminders:
            rtype = r["reminder_type"]
            if rtype == "join":
                trigger = f"on-join in {r['channel']}"
            elif r["recurrence"]:
                trigger = f"{r['recurrence']} at {r['deliver_at'][:16]}"
            else:
                trigger = f"at {r['deliver_at'][:16]} UTC"

            target_info = f"→ {r['target_nick']}" if r['creator_nick'] != r['target_nick'] else "(self)"
            lines.append(f"#{r['id']} [{rtype}] {trigger} {target_info}: {r['message'][:40]}...")

        return "Pending reminders: " + " | ".join(lines)

    def _cancel_reminder(self, reminder_id: int, nick: str, permission_level: str) -> str:
        """Cancel a pending reminder."""
        conn = sqlite3.connect(str(self.DB_PATH))
        conn.row_factory = sqlite3.Row
        cursor = conn.cursor()

        cursor.execute("SELECT * FROM reminders WHERE id = ?", (reminder_id,))
        rem = cursor.fetchone()

        if not rem:
            conn.close()
            return f"Error: Reminder #{reminder_id} not found."

        if rem["status"] != "pending":
            conn.close()
            return f"Error: Reminder #{reminder_id} is already {rem['status']}."

        # Permission check: creator, target, or admin/owner
        if permission_level not in ("owner", "admin"):
            if rem["creator_nick"].lower() != nick.lower() and rem["target_nick"].lower() != nick.lower():
                conn.close()
                return "Permission denied: You can only cancel your own reminders."

        cursor.execute(
            "UPDATE reminders SET status = 'cancelled' WHERE id = ?",
            (reminder_id,)
        )
        conn.commit()
        conn.close()

        log_info(f"[REMINDER] #{reminder_id} cancelled by {nick}")
        return f"Reminder #{reminder_id} cancelled."

    def _check_reminders(self, nick: str) -> str:
        """Quick count of pending reminders for a user."""
        conn = sqlite3.connect(str(self.DB_PATH))
        cursor = conn.cursor()

        cursor.execute(
            "SELECT COUNT(*) FROM reminders WHERE status = 'pending' AND (LOWER(creator_nick) = LOWER(?) OR LOWER(target_nick) = LOWER(?))",
            (nick, nick)
        )
        count = cursor.fetchone()[0]
        conn.close()

        if count == 0:
            return f"{nick} has no pending reminders."
        return f"{nick} has {count} pending reminder(s). Use action 'list' to see them."

    def execute(
        self,
        action: str,
        reminder_type: Optional[str] = None,
        target_nick: Optional[str] = None,
        message: Optional[str] = None,
        deliver_at: Optional[str] = None,
        recurrence: Optional[str] = None,
        reminder_id: Optional[int] = None,
        channel: str = "",
        permission_level: str = "normal",
        requesting_user: str = "unknown",
        **kwargs
    ) -> str:
        """Execute reminder action."""

        if action == "create":
            if not reminder_type:
                return "Error: reminder_type is required (time, join, or recurring)."
            if not message:
                return "Error: message is required."
            actual_type = "time" if reminder_type == "recurring" else reminder_type
            return self._create_reminder(
                creator=requesting_user,
                target=target_nick or requesting_user,
                channel=channel,
                message=message,
                reminder_type=actual_type,
                deliver_at=deliver_at,
                recurrence=recurrence if reminder_type == "recurring" else None
            )

        elif action == "list":
            return self._list_reminders(requesting_user, permission_level)

        elif action == "cancel":
            if not reminder_id:
                return "Error: reminder_id is required."
            return self._cancel_reminder(reminder_id, requesting_user, permission_level)

        elif action == "check":
            return self._check_reminders(requesting_user)

        else:
            return f"Error: Unknown action '{action}'. Use: create, list, cancel, check"


# Global instance for join-check access from the router
_reminder_tool_instance: Optional[ReminderTool] = None


def get_reminder_tool() -> Optional[ReminderTool]:
    """Get the global reminder tool instance."""
    return _reminder_tool_instance


def set_reminder_tool(tool: ReminderTool) -> None:
    """Set the global reminder tool instance."""
    global _reminder_tool_instance
    _reminder_tool_instance = tool
