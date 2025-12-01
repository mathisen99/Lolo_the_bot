"""
User rules tool implementation.

Allows users to set personal rules/preferences that get injected into the system prompt.
Admins and owners can manage rules for any user.
"""

import json
import os
from pathlib import Path
from typing import Any, Dict, Optional, List
from .base import Tool


# Permission levels that can manage other users' rules
ADMIN_PERMISSION_LEVELS = ["owner", "admin"]


class UserRulesTool(Tool):
    """Tool for managing per-user custom rules."""
    
    def __init__(self):
        """Initialize user rules tool."""
        # Store rules in data directory
        self.rules_file = Path("data/user_rules.json")
        self._ensure_file_exists()
    
    @property
    def name(self) -> str:
        return "manage_user_rules"
    
    def _ensure_file_exists(self) -> None:
        """Ensure the rules file exists."""
        self.rules_file.parent.mkdir(parents=True, exist_ok=True)
        if not self.rules_file.exists():
            self._save_rules({})
    
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
    
    def get_definition(self) -> Dict[str, Any]:
        """Get tool definition for OpenAI API."""
        return {
            "type": "function",
            "name": "manage_user_rules",
            "description": """Manage per-user custom rules/preferences that affect how you respond to them.
Use this when a user wants to:
- Set a roleplay character or persona for you to use with them
- Add custom instructions for how you should respond to them
- View their current rules
- Update or modify their existing rules
- Remove/clear their rules
- Enable or disable their rules temporarily

Owners and admins can also manage rules for OTHER users by specifying target_user.

Examples of when to use:
- "roleplay as Doc Brown" -> action=set, rules="Roleplay as Doc Brown from Back to the Future"
- "what rules do I have?" -> action=get
- "turn off my roleplay" -> action=disable
- "turn my rules back on" -> action=enable
- "clear my rules" -> action=clear
- "update my rules to be more sarcastic" -> action=set, rules="<updated rules>"
- Owner/Admin: "set rules for bob to speak french" -> action=set, target_user=bob, rules="Always respond in French" """,
            "parameters": {
                "type": "object",
                "properties": {
                    "action": {
                        "type": "string",
                        "enum": ["get", "set", "clear", "enable", "disable"],
                        "description": "Action to perform: get (view rules), set (create/update rules), clear (remove rules), enable/disable (toggle rules)"
                    },
                    "requesting_user": {
                        "type": "string",
                        "description": "The nick of the user making the request (from the conversation)"
                    },
                    "target_user": {
                        "type": ["string", "null"],
                        "description": "Target user's nick. Only admins can specify a different user. If null, applies to requesting_user."
                    },
                    "rules": {
                        "type": ["string", "null"],
                        "description": "The rules/instructions to set (only for 'set' action). Describe the persona, roleplay, or custom behavior."
                    }
                },
                "required": ["action", "requesting_user"],
                "additionalProperties": False
            }
        }
    
    def is_admin(self, permission_level: str) -> bool:
        """Check if a user has admin/owner permissions."""
        return permission_level in ADMIN_PERMISSION_LEVELS
    
    def get_user_rules(self, nick: str) -> Optional[Dict[str, Any]]:
        """
        Get rules for a specific user.
        
        Returns None if no rules exist, otherwise returns:
        {
            "rules": "the rules text",
            "enabled": true/false
        }
        """
        all_rules = self._load_rules()
        return all_rules.get(nick.lower())
    
    def get_active_rules(self, nick: str) -> Optional[str]:
        """
        Get active rules for a user (only if enabled).
        
        Returns the rules text if rules exist and are enabled, None otherwise.
        """
        user_rules = self.get_user_rules(nick)
        if user_rules and user_rules.get("enabled", True):
            return user_rules.get("rules")
        return None
    
    def execute(
        self,
        action: str,
        requesting_user: str,
        target_user: Optional[str] = None,
        rules: Optional[str] = None,
        permission_level: str = "normal",
        **kwargs
    ) -> str:
        """
        Execute a user rules management action.
        
        Args:
            action: get, set, clear, enable, or disable
            requesting_user: Nick of user making the request
            target_user: Nick of target user (admin only for other users)
            rules: Rules text (for set action)
            permission_level: User's permission level from the bot
            
        Returns:
            Result message
        """
        # Determine effective target
        effective_target = (target_user or requesting_user).lower()
        requesting_lower = requesting_user.lower()
        
        # Check permissions for managing other users
        if target_user and target_user.lower() != requesting_lower:
            if not self.is_admin(permission_level):
                return f"Permission denied: Only admins/owners can manage other users' rules."
        
        all_rules = self._load_rules()
        
        if action == "get":
            user_rules = all_rules.get(effective_target)
            if not user_rules:
                if effective_target == requesting_lower:
                    return "You don't have any custom rules set."
                else:
                    return f"{target_user} doesn't have any custom rules set."
            
            status = "enabled" if user_rules.get("enabled", True) else "disabled"
            rules_text = user_rules.get("rules", "")
            
            if effective_target == requesting_lower:
                return f"Your rules ({status}): {rules_text}"
            else:
                return f"Rules for {target_user} ({status}): {rules_text}"
        
        elif action == "set":
            if not rules:
                return "No rules provided. Please specify what rules you want to set."
            
            all_rules[effective_target] = {
                "rules": rules,
                "enabled": True
            }
            self._save_rules(all_rules)
            
            if effective_target == requesting_lower:
                return f"Your rules have been set and enabled: {rules}"
            else:
                return f"Rules for {target_user} have been set: {rules}"
        
        elif action == "clear":
            if effective_target in all_rules:
                del all_rules[effective_target]
                self._save_rules(all_rules)
                
                if effective_target == requesting_lower:
                    return "Your rules have been cleared."
                else:
                    return f"Rules for {target_user} have been cleared."
            else:
                if effective_target == requesting_lower:
                    return "You don't have any rules to clear."
                else:
                    return f"{target_user} doesn't have any rules to clear."
        
        elif action == "enable":
            if effective_target not in all_rules:
                if effective_target == requesting_lower:
                    return "You don't have any rules to enable. Set some rules first."
                else:
                    return f"{target_user} doesn't have any rules to enable."
            
            all_rules[effective_target]["enabled"] = True
            self._save_rules(all_rules)
            
            if effective_target == requesting_lower:
                return "Your rules have been enabled."
            else:
                return f"Rules for {target_user} have been enabled."
        
        elif action == "disable":
            if effective_target not in all_rules:
                if effective_target == requesting_lower:
                    return "You don't have any rules to disable."
                else:
                    return f"{target_user} doesn't have any rules to disable."
            
            all_rules[effective_target]["enabled"] = False
            self._save_rules(all_rules)
            
            if effective_target == requesting_lower:
                return "Your rules have been disabled. They're saved but won't be applied until you enable them again."
            else:
                return f"Rules for {target_user} have been disabled."
        
        else:
            return f"Unknown action: {action}"
