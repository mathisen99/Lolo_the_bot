"""
User rules/memory tool implementation.

Allows users to store multiple memories/rules that get injected into the system prompt.
Admins and owners can manage memories for any user.
"""

import json
from pathlib import Path
from typing import Any, Dict, Optional, List
from .base import Tool


# Permission levels that can manage other users' rules
ADMIN_PERMISSION_LEVELS = ["owner", "admin"]


class UserRulesTool(Tool):
    """Tool for managing per-user memories and custom rules."""
    
    def __init__(self):
        """Initialize user rules tool."""
        self.rules_file = Path("data/user_rules.json")
        self._ensure_file_exists()
        self._migrate_if_needed()
    
    @property
    def name(self) -> str:
        return "manage_user_rules"
    
    def _ensure_file_exists(self) -> None:
        """Ensure the rules file exists."""
        self.rules_file.parent.mkdir(parents=True, exist_ok=True)
        if not self.rules_file.exists():
            self._save_rules({})
    
    def _migrate_if_needed(self) -> None:
        """Migrate old single-rule format to new multi-entry format."""
        all_rules = self._load_rules()
        migrated = False
        
        for nick, data in all_rules.items():
            # Old format: {"rules": "...", "enabled": true}
            # New format: {"entries": [...], "next_id": N}
            if "rules" in data and "entries" not in data:
                old_rules = data.get("rules", "")
                old_enabled = data.get("enabled", True)
                
                all_rules[nick] = {
                    "entries": [
                        {
                            "id": 1,
                            "content": old_rules,
                            "enabled": old_enabled
                        }
                    ] if old_rules else [],
                    "next_id": 2 if old_rules else 1
                }
                migrated = True
        
        if migrated:
            self._save_rules(all_rules)
    
    def _load_rules(self) -> Dict[str, Any]:
        """Load all user rules from file."""
        try:
            with open(self.rules_file, "r") as f:
                return json.load(f)
        except (json.JSONDecodeError, FileNotFoundError):
            return {}
    
    def _save_rules(self, rules: Dict[str, Any]) -> None:
        """Save all user rules to file."""
        with open(self.rules_file, "w") as f:
            json.dump(rules, f, indent=2)
    
    def _get_user_data(self, nick: str) -> Dict[str, Any]:
        """Get or create user data structure."""
        all_rules = self._load_rules()
        nick_lower = nick.lower()
        
        if nick_lower not in all_rules:
            return {"entries": [], "next_id": 1}
        
        return all_rules[nick_lower]
    
    def _save_user_data(self, nick: str, data: Dict[str, Any]) -> None:
        """Save user data structure."""
        all_rules = self._load_rules()
        all_rules[nick.lower()] = data
        self._save_rules(all_rules)
    
    def get_definition(self) -> Dict[str, Any]:
        """Get tool definition for OpenAI API."""
        return {
            "type": "function",
            "name": "manage_user_rules",
            "description": """Manage per-user memories and custom rules that affect how you respond to them.
Each user can have MULTIPLE separate entries (memories, facts, roleplay rules, preferences).

Use this when a user:
- Asks you to remember something: "remember I like cats", "remember my name is Bob"
- Wants to add a roleplay/persona: "roleplay as a pirate", "act like a helpful butler"
- Wants to see what you remember: "what do you remember about me?", "list my rules"
- Wants to forget something specific: "forget that I like cats", "delete entry 2"
- Wants to update a specific memory: "update entry 1 to say I love cats"
- Wants to clear everything: "forget everything about me", "clear all my rules"
- Wants to enable/disable specific entries or all entries

Owners and admins can manage memories for OTHER users by specifying target_user.

IMPORTANT: Use 'add' to create NEW entries. Use 'update' only to MODIFY existing ones.""",
            "parameters": {
                "type": "object",
                "properties": {
                    "action": {
                        "type": "string",
                        "enum": ["list", "add", "update", "delete", "clear", "enable", "disable"],
                        "description": "Action: list (view all), add (new entry), update (modify entry), delete (remove entry), clear (remove all), enable/disable (toggle entry or all)"
                    },
                    "requesting_user": {
                        "type": "string",
                        "description": "The nick of the user making the request (from the conversation)"
                    },
                    "target_user": {
                        "type": ["string", "null"],
                        "description": "Target user's nick. Only admins can specify a different user. If null, applies to requesting_user."
                    },
                    "content": {
                        "type": ["string", "null"],
                        "description": "The memory/rule content (for 'add' or 'update' actions). Be concise but complete."
                    },
                    "entry_id": {
                        "type": ["integer", "null"],
                        "description": "Specific entry ID (for 'update', 'delete', 'enable', 'disable' on single entry). If null for enable/disable, affects all entries."
                    },
                    "search_term": {
                        "type": ["string", "null"],
                        "description": "Search term to find entry by content (alternative to entry_id for delete/update). Case-insensitive partial match."
                    }
                },
                "required": ["action", "requesting_user"],
                "additionalProperties": False
            }
        }
    
    def is_admin(self, permission_level: str) -> bool:
        """Check if a user has admin/owner permissions."""
        return permission_level in ADMIN_PERMISSION_LEVELS
    
    def get_active_rules(self, nick: str) -> Optional[str]:
        """
        Get all active (enabled) rules for a user as a formatted string.
        
        Returns combined rules text if any enabled entries exist, None otherwise.
        """
        user_data = self._get_user_data(nick)
        entries = user_data.get("entries", [])
        
        active_entries = [e for e in entries if e.get("enabled", True)]
        
        if not active_entries:
            return None
        
        # Format as numbered list for clarity
        lines = []
        for entry in active_entries:
            lines.append(f"- {entry['content']}")
        
        return "\n".join(lines)
    
    def _find_entry_by_search(self, entries: List[Dict], search_term: str) -> Optional[Dict]:
        """Find an entry by partial content match."""
        search_lower = search_term.lower()
        for entry in entries:
            if search_lower in entry.get("content", "").lower():
                return entry
        return None
    
    def execute(
        self,
        action: str,
        requesting_user: str,
        target_user: Optional[str] = None,
        content: Optional[str] = None,
        entry_id: Optional[int] = None,
        search_term: Optional[str] = None,
        permission_level: str = "normal",
        **kwargs
    ) -> str:
        """
        Execute a user rules management action.
        
        Args:
            action: list, add, update, delete, clear, enable, disable
            requesting_user: Nick of user making the request
            target_user: Nick of target user (admin only for other users)
            content: Memory/rule content (for add/update)
            entry_id: Specific entry ID to operate on
            search_term: Search term to find entry by content
            permission_level: User's permission level from the bot
            
        Returns:
            Result message
        """
        effective_target = (target_user or requesting_user).lower()
        requesting_lower = requesting_user.lower()
        
        # Check permissions for managing other users
        if target_user and target_user.lower() != requesting_lower:
            if not self.is_admin(permission_level):
                return "Permission denied: Only admins/owners can manage other users' memories."
        
        is_self = effective_target == requesting_lower
        target_display = "your" if is_self else f"{target_user}'s"
        
        user_data = self._get_user_data(effective_target)
        entries = user_data.get("entries", [])
        next_id = user_data.get("next_id", 1)
        
        # === LIST ===
        if action == "list":
            if not entries:
                return f"No memories stored for {target_display.replace('your', 'you')}." if is_self else f"No memories stored for {target_user}."
            
            lines = [f"Memories for {effective_target}:"]
            for entry in entries:
                status = "✓" if entry.get("enabled", True) else "✗"
                lines.append(f"  [{entry['id']}] {status} {entry['content']}")
            
            return " | ".join(lines)
        
        # === ADD ===
        elif action == "add":
            if not content:
                return "No content provided. What should I remember?"
            
            new_entry = {
                "id": next_id,
                "content": content,
                "enabled": True
            }
            entries.append(new_entry)
            user_data["entries"] = entries
            user_data["next_id"] = next_id + 1
            self._save_user_data(effective_target, user_data)
            
            return f"Got it! Added memory #{next_id}: \"{content}\""
        
        # === UPDATE ===
        elif action == "update":
            if not content:
                return "No new content provided. What should I update it to?"
            
            target_entry = None
            
            if entry_id is not None:
                target_entry = next((e for e in entries if e["id"] == entry_id), None)
                if not target_entry:
                    return f"Entry #{entry_id} not found."
            elif search_term:
                target_entry = self._find_entry_by_search(entries, search_term)
                if not target_entry:
                    return f"No entry found matching \"{search_term}\"."
            else:
                return "Please specify which entry to update (by ID or search term)."
            
            old_content = target_entry["content"]
            target_entry["content"] = content
            self._save_user_data(effective_target, user_data)
            
            return f"Updated entry #{target_entry['id']}: \"{old_content}\" → \"{content}\""
        
        # === DELETE ===
        elif action == "delete":
            target_entry = None
            
            if entry_id is not None:
                target_entry = next((e for e in entries if e["id"] == entry_id), None)
                if not target_entry:
                    return f"Entry #{entry_id} not found."
            elif search_term:
                target_entry = self._find_entry_by_search(entries, search_term)
                if not target_entry:
                    return f"No entry found matching \"{search_term}\"."
            else:
                return "Please specify which entry to delete (by ID or search term)."
            
            entries.remove(target_entry)
            user_data["entries"] = entries
            self._save_user_data(effective_target, user_data)
            
            return f"Deleted entry #{target_entry['id']}: \"{target_entry['content']}\""
        
        # === CLEAR ===
        elif action == "clear":
            if not entries:
                return "No memories to clear."
            
            count = len(entries)
            user_data["entries"] = []
            user_data["next_id"] = 1
            self._save_user_data(effective_target, user_data)
            
            return f"Cleared all {count} memories for {effective_target}."
        
        # === ENABLE ===
        elif action == "enable":
            if entry_id is not None:
                target_entry = next((e for e in entries if e["id"] == entry_id), None)
                if not target_entry:
                    return f"Entry #{entry_id} not found."
                
                target_entry["enabled"] = True
                self._save_user_data(effective_target, user_data)
                return f"Enabled entry #{entry_id}."
            
            elif search_term:
                target_entry = self._find_entry_by_search(entries, search_term)
                if not target_entry:
                    return f"No entry found matching \"{search_term}\"."
                
                target_entry["enabled"] = True
                self._save_user_data(effective_target, user_data)
                return f"Enabled entry #{target_entry['id']}."
            
            else:
                # Enable all
                if not entries:
                    return "No memories to enable."
                
                for entry in entries:
                    entry["enabled"] = True
                self._save_user_data(effective_target, user_data)
                return f"Enabled all {len(entries)} memories."
        
        # === DISABLE ===
        elif action == "disable":
            if entry_id is not None:
                target_entry = next((e for e in entries if e["id"] == entry_id), None)
                if not target_entry:
                    return f"Entry #{entry_id} not found."
                
                target_entry["enabled"] = False
                self._save_user_data(effective_target, user_data)
                return f"Disabled entry #{entry_id}. It's saved but won't be applied."
            
            elif search_term:
                target_entry = self._find_entry_by_search(entries, search_term)
                if not target_entry:
                    return f"No entry found matching \"{search_term}\"."
                
                target_entry["enabled"] = False
                self._save_user_data(effective_target, user_data)
                return f"Disabled entry #{target_entry['id']}. It's saved but won't be applied."
            
            else:
                # Disable all
                if not entries:
                    return "No memories to disable."
                
                for entry in entries:
                    entry["enabled"] = False
                self._save_user_data(effective_target, user_data)
                return f"Disabled all {len(entries)} memories. They're saved but won't be applied."
        
        else:
            return f"Unknown action: {action}"
