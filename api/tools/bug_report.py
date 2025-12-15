"""
Bug reporting tool implementation.

Allows users to report bugs and admins/owners to manage them.
Stores bug reports in SQLite database.
"""

import sqlite3
import json
from datetime import datetime
from pathlib import Path
from typing import Any, Dict, Optional, List
from .base import Tool
from api.utils.output import log_info, log_success, log_error, log_warning


class BugReportTool(Tool):
    """Tool for managing bug reports."""
    
    # Database path
    DB_PATH = Path(__file__).parent.parent.parent / "data" / "bugs.db"
    
    def __init__(self):
        """Initialize bug report tool and create database if needed."""
        self._init_database()
    
    @property
    def name(self) -> str:
        return "bug_report"
    
    def _init_database(self) -> None:
        """Initialize the SQLite database."""
        self.DB_PATH.parent.mkdir(parents=True, exist_ok=True)
        
        conn = sqlite3.connect(str(self.DB_PATH))
        cursor = conn.cursor()
        
        cursor.execute("""
            CREATE TABLE IF NOT EXISTS bugs (
                id INTEGER PRIMARY KEY AUTOINCREMENT,
                reporter TEXT NOT NULL,
                channel TEXT,
                description TEXT NOT NULL,
                status TEXT DEFAULT 'open',
                priority TEXT DEFAULT 'normal',
                created_at TEXT NOT NULL,
                updated_at TEXT NOT NULL,
                resolved_by TEXT,
                resolution_note TEXT
            )
        """)
        
        conn.commit()
        conn.close()
    
    def get_definition(self) -> Dict[str, Any]:
        """Get tool definition for OpenAI API."""
        return {
            "type": "function",
            "name": "bug_report",
            "description": """Manage bug reports for the bot.

Actions:
- report: Submit a new bug report (any user)
- list: List bug reports (admin/owner only, or user can see their own)
- update: Update bug status/priority (admin/owner only)
- delete: Delete a bug report (admin/owner only)
- resolve: Mark a bug as resolved with a note (admin/owner only)

Use this when:
- User wants to report a bug or issue with the bot
- User says something isn't working correctly
- Admin/owner wants to see reported bugs
- Admin/owner wants to manage bug reports

Example triggers:
- "I want to report a bug"
- "X feature is broken"
- "something isn't working"
- "show me the bug reports" (admin/owner)
- "mark bug #5 as resolved" (admin/owner)""",
            "parameters": {
                "type": "object",
                "properties": {
                    "action": {
                        "type": "string",
                        "enum": ["report", "list", "update", "delete", "resolve"],
                        "description": "Action to perform"
                    },
                    "description": {
                        "type": "string",
                        "description": "Bug description (required for 'report' action)"
                    },
                    "bug_id": {
                        "type": "integer",
                        "description": "Bug ID (required for update/delete/resolve actions)"
                    },
                    "status": {
                        "type": "string",
                        "enum": ["open", "in_progress", "resolved", "wontfix", "duplicate"],
                        "description": "New status (for 'update' action)"
                    },
                    "priority": {
                        "type": "string",
                        "enum": ["low", "normal", "high", "critical"],
                        "description": "Priority level (for 'update' action)"
                    },
                    "resolution_note": {
                        "type": "string",
                        "description": "Note explaining the resolution (for 'resolve' action)"
                    },
                    "filter_status": {
                        "type": "string",
                        "enum": ["all", "open", "in_progress", "resolved", "wontfix", "duplicate"],
                        "description": "Filter bugs by status (for 'list' action, default: 'open')"
                    }
                },
                "required": ["action"],
                "additionalProperties": False
            }
        }
    
    def _report_bug(self, reporter: str, channel: str, description: str) -> str:
        """Submit a new bug report."""
        if not description or len(description.strip()) < 10:
            return "Error: Bug description must be at least 10 characters."
        
        now = datetime.utcnow().isoformat()
        
        conn = sqlite3.connect(str(self.DB_PATH))
        cursor = conn.cursor()
        
        cursor.execute("""
            INSERT INTO bugs (reporter, channel, description, created_at, updated_at)
            VALUES (?, ?, ?, ?, ?)
        """, (reporter, channel, description.strip(), now, now))
        
        bug_id = cursor.lastrowid
        conn.commit()
        conn.close()
        
        log_success(f"[BUG_REPORT] New bug #{bug_id} reported by {reporter}")
        return f"Bug report #{bug_id} submitted successfully. Thank you for reporting!"
    
    def _list_bugs(self, requester: str, permission_level: str, filter_status: str = "open") -> str:
        """List bug reports."""
        conn = sqlite3.connect(str(self.DB_PATH))
        conn.row_factory = sqlite3.Row
        cursor = conn.cursor()
        
        # Admins/owners see all bugs, regular users only see their own
        if permission_level in ("owner", "admin"):
            if filter_status == "all":
                cursor.execute("SELECT * FROM bugs ORDER BY created_at DESC LIMIT 20")
            else:
                cursor.execute("SELECT * FROM bugs WHERE status = ? ORDER BY created_at DESC LIMIT 20", (filter_status,))
        else:
            if filter_status == "all":
                cursor.execute("SELECT * FROM bugs WHERE reporter = ? ORDER BY created_at DESC LIMIT 10", (requester,))
            else:
                cursor.execute("SELECT * FROM bugs WHERE reporter = ? AND status = ? ORDER BY created_at DESC LIMIT 10", 
                             (requester, filter_status))
        
        bugs = cursor.fetchall()
        conn.close()
        
        if not bugs:
            if permission_level in ("owner", "admin"):
                return f"No bug reports found with status '{filter_status}'."
            return "You haven't submitted any bug reports."
        
        # Format bug list
        lines = []
        for bug in bugs:
            priority_marker = {"critical": "🔴", "high": "🟠", "normal": "🟡", "low": "🟢"}.get(bug["priority"], "⚪")
            status_marker = {"open": "📋", "in_progress": "🔧", "resolved": "✅", "wontfix": "❌", "duplicate": "📎"}.get(bug["status"], "❓")
            
            created = bug["created_at"][:10]  # Just the date
            lines.append(f"#{bug['id']} {status_marker}{priority_marker} [{bug['status']}] by {bug['reporter']} ({created}): {bug['description'][:50]}...")
        
        header = f"Bug reports ({filter_status}):" if permission_level in ("owner", "admin") else "Your bug reports:"
        return header + " | ".join(lines)
    
    def _update_bug(self, bug_id: int, status: Optional[str], priority: Optional[str], updater: str) -> str:
        """Update bug status or priority."""
        conn = sqlite3.connect(str(self.DB_PATH))
        cursor = conn.cursor()
        
        # Check bug exists
        cursor.execute("SELECT id FROM bugs WHERE id = ?", (bug_id,))
        if not cursor.fetchone():
            conn.close()
            return f"Error: Bug #{bug_id} not found."
        
        updates = []
        params = []
        
        if status:
            updates.append("status = ?")
            params.append(status)
        
        if priority:
            updates.append("priority = ?")
            params.append(priority)
        
        if not updates:
            conn.close()
            return "Error: No updates specified. Provide status or priority."
        
        updates.append("updated_at = ?")
        params.append(datetime.utcnow().isoformat())
        params.append(bug_id)
        
        cursor.execute(f"UPDATE bugs SET {', '.join(updates)} WHERE id = ?", params)
        conn.commit()
        conn.close()
        
        log_info(f"[BUG_REPORT] Bug #{bug_id} updated by {updater}")
        return f"Bug #{bug_id} updated successfully."
    
    def _resolve_bug(self, bug_id: int, resolution_note: str, resolver: str) -> str:
        """Mark a bug as resolved."""
        conn = sqlite3.connect(str(self.DB_PATH))
        cursor = conn.cursor()
        
        cursor.execute("SELECT id FROM bugs WHERE id = ?", (bug_id,))
        if not cursor.fetchone():
            conn.close()
            return f"Error: Bug #{bug_id} not found."
        
        now = datetime.utcnow().isoformat()
        cursor.execute("""
            UPDATE bugs 
            SET status = 'resolved', resolved_by = ?, resolution_note = ?, updated_at = ?
            WHERE id = ?
        """, (resolver, resolution_note or "Resolved", now, bug_id))
        
        conn.commit()
        conn.close()
        
        log_success(f"[BUG_REPORT] Bug #{bug_id} resolved by {resolver}")
        return f"Bug #{bug_id} marked as resolved."
    
    def _delete_bug(self, bug_id: int, deleter: str) -> str:
        """Delete a bug report."""
        conn = sqlite3.connect(str(self.DB_PATH))
        cursor = conn.cursor()
        
        cursor.execute("SELECT id FROM bugs WHERE id = ?", (bug_id,))
        if not cursor.fetchone():
            conn.close()
            return f"Error: Bug #{bug_id} not found."
        
        cursor.execute("DELETE FROM bugs WHERE id = ?", (bug_id,))
        conn.commit()
        conn.close()
        
        log_warning(f"[BUG_REPORT] Bug #{bug_id} deleted by {deleter}")
        return f"Bug #{bug_id} deleted."
    
    def execute(
        self,
        action: str,
        description: Optional[str] = None,
        bug_id: Optional[int] = None,
        status: Optional[str] = None,
        priority: Optional[str] = None,
        resolution_note: Optional[str] = None,
        filter_status: str = "open",
        permission_level: str = "normal",
        requesting_user: str = "unknown",
        channel: str = "",
        **kwargs
    ) -> str:
        """Execute bug report action."""
        
        if action == "report":
            if not description:
                return "Error: Please provide a description of the bug."
            return self._report_bug(requesting_user, channel, description)
        
        elif action == "list":
            return self._list_bugs(requesting_user, permission_level, filter_status)
        
        elif action == "update":
            if permission_level not in ("owner", "admin"):
                return "Permission denied: Only admins and owners can update bug reports."
            if not bug_id:
                return "Error: Please specify a bug ID to update."
            return self._update_bug(bug_id, status, priority, requesting_user)
        
        elif action == "resolve":
            if permission_level not in ("owner", "admin"):
                return "Permission denied: Only admins and owners can resolve bugs."
            if not bug_id:
                return "Error: Please specify a bug ID to resolve."
            return self._resolve_bug(bug_id, resolution_note or "", requesting_user)
        
        elif action == "delete":
            if permission_level not in ("owner", "admin"):
                return "Permission denied: Only admins and owners can delete bug reports."
            if not bug_id:
                return "Error: Please specify a bug ID to delete."
            return self._delete_bug(bug_id, requesting_user)
        
        else:
            return f"Error: Unknown action '{action}'. Use: report, list, update, resolve, delete"
