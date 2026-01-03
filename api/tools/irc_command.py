"""
IRC Command tool - Execute IRC commands through the bot.

Provides a comprehensive IRC command interface with permission-based access control.
Owner/Admin: Full access to channel ops (kick, ban, mode, topic, etc.)
Normal users: Limited to informational commands (whois, nickserv info, alis search)
"""

import os
import requests
from typing import Any, Dict, Optional, List
from .base import Tool
from api.utils.output import log_info, log_error, log_warning, log_success


# Commands that normal users can execute (informational only)
NORMAL_USER_COMMANDS = {
    # User info
    "whois", "whowas",
    # NickServ info queries
    "ns_info", "nickserv_info",
    # ChanServ info queries  
    "cs_info", "chanserv_info",
    # ALIS channel search
    "alis_list", "alis_search",
    # Version/time queries
    "version", "time",
    # Bot channel/user database queries (anyone can ask)
    "bot_status", "channel_info", "channel_list", "user_status",
    "channel_ops", "channel_voiced", "channel_topic", "find_user",
}

# Commands that require admin or owner
ADMIN_COMMANDS = {
    # Channel moderation
    "kick", "ban", "unban", "quiet", "unquiet",
    # Channel modes
    "op", "deop", "voice", "devoice", "halfop", "dehalfop",
    # Channel management
    "topic", "mode", "invite",
    # ChanServ management
    "cs_op", "cs_deop", "cs_voice", "cs_devoice",
    "cs_kick", "cs_ban", "cs_unban", "cs_quiet", "cs_unquiet",
    "cs_topic", "cs_flags", "cs_access", "cs_akick",
    "cs_invite", "cs_clear",
    # NickServ management
    "ns_ghost", "ns_release", "ns_regain",
}

# Commands that are owner-only (dangerous operations)
OWNER_COMMANDS = {
    # Nothing extra for now - admin commands are sufficient
    # Could add things like cs_drop, ns_drop if needed
}


class IRCCommandTool(Tool):
    """Tool for executing IRC commands through the bot."""
    
    # Go bot callback endpoint
    GO_BOT_CALLBACK_URL = os.getenv("GO_BOT_CALLBACK_URL", "http://localhost:8001")
    
    def __init__(self, timeout: int = 30):
        """
        Initialize IRC command tool.
        
        Args:
            timeout: Command execution timeout in seconds
        """
        self.timeout = timeout
    
    @property
    def name(self) -> str:
        return "irc_command"
    
    def get_definition(self) -> Dict[str, Any]:
        """Get tool definition for OpenAI API."""
        return {
            "type": "function",
            "name": "irc_command",
            "description": """Execute IRC commands through the bot. Permission-based access control.

NORMAL USERS can use:
- whois <nick> - Get user information (channels, idle time, account)
- whowas <nick> - Get info on disconnected user
- ns_info <nick> - NickServ INFO (who owns a nick, registration date, last seen)
- cs_info <channel> - ChanServ INFO (channel info, founder, registration date)
- alis_search <pattern> [options] - Search channels via ALIS service
  Options: -min <N> (min users), -max <N> (max users), -topic <text>
  Example: alis_search "*linux*" or alis_search "* -min 50 -topic help"
- version <nick> - Get user's client version (CTCP)
- time <nick> - Get user's local time (CTCP)
- bot_status <channel> - Check if bot has op in a channel
- channel_info <channel> - Get channel user/op/voice counts and topic
- channel_list - List all channels the bot is in with user counts
- user_status <channel> <nick> - Check if a user has op/voice in a channel
- channel_ops <channel> - List all ops in a channel
- channel_voiced <channel> - List all voiced users in a channel
- channel_topic <channel> - Get just the topic of a channel
- find_user <nick> - Find which channels a user is in

ADMIN/OWNER can also use:
Channel Ops (bot must have op - use bot_status to check first!):
- kick <channel> <nick> [reason] - Kick user from channel
- ban <channel> <mask> - Ban a hostmask (+b)
- unban <channel> <mask> - Remove ban (-b)
- quiet <channel> <mask> - Quiet a user (+q)
- unquiet <channel> <mask> - Remove quiet (-q)
- op <channel> <nick> - Give operator status
- deop <channel> <nick> - Remove operator status
- voice <channel> <nick> - Give voice
- devoice <channel> <nick> - Remove voice
- topic <channel> <new_topic> - Change channel topic
- mode <channel> <modes> - Set channel modes
- invite <channel> <nick> - Invite user to channel

ChanServ Commands (use when bot doesn't have op):
- cs_op <channel> [nick] - Request op from ChanServ
- cs_voice <channel> [nick] - Request voice from ChanServ
- cs_kick <channel> <nick> [reason] - Kick via ChanServ
- cs_ban <channel> <nick> [reason] - Ban via ChanServ
- cs_unban <channel> <mask> - Unban via ChanServ
- cs_quiet <channel> <nick> - Quiet via ChanServ
- cs_topic <channel> <topic> - Set topic via ChanServ
- cs_flags <channel> [nick] [flags] - View/set ChanServ flags
- cs_access <channel> - View channel access list
- cs_akick <channel> <add|del> <mask> [reason] - Manage auto-kick list
- cs_invite <channel> <nick> - Invite via ChanServ
- cs_clear <channel> <what> - Clear channel (ops, voices, bans, etc.)

NickServ Commands:
- ns_ghost <nick> - Ghost a nick (disconnect impersonator)
- ns_release <nick> - Release a held nick
- ns_regain <nick> - Regain your nick

IMPORTANT: Before using kick/ban/op/voice commands, check bot_status first!
If bot doesn't have op, use ChanServ commands (cs_*) instead.

Examples:
- "who owns the nick foobar" -> command="ns_info", args=["foobar"]
- "search for python channels" -> command="alis_search", args=["python"]
- "do you have op in #channel" -> command="bot_status", args=["#channel"]
- "how many users in #channel" -> command="channel_info", args=["#channel"]
- "kick baduser from #channel" -> First check bot_status, then kick or cs_kick
- "give me op in #mychannel" -> command="cs_op", args=["#mychannel"]""",
            "parameters": {
                "type": "object",
                "properties": {
                    "command": {
                        "type": "string",
                        "description": "The IRC command to execute (e.g., 'whois', 'kick', 'ns_info', 'cs_op', 'alis_search')"
                    },
                    "args": {
                        "type": "array",
                        "items": {"type": "string"},
                        "description": "Arguments for the command (e.g., ['#channel', 'nick', 'reason'])"
                    },
                    "channel": {
                        "type": "string",
                        "description": "Target channel (optional, some commands infer from context)"
                    }
                },
                "required": ["command"],
                "additionalProperties": False
            }
        }
    
    def _check_permission(self, command: str, permission_level: str) -> tuple[bool, str]:
        """
        Check if user has permission to execute command.
        
        Returns:
            Tuple of (allowed, error_message)
        """
        command_lower = command.lower()
        
        # Owner can do everything
        if permission_level == "owner":
            return True, ""
        
        # Admin can do admin commands and normal commands
        if permission_level == "admin":
            if command_lower in OWNER_COMMANDS:
                return False, f"Command '{command}' is restricted to bot owner only."
            return True, ""
        
        # Normal users can only do informational commands
        if permission_level == "normal":
            if command_lower in NORMAL_USER_COMMANDS:
                return True, ""
            return False, f"Command '{command}' requires admin privileges. You can use: whois, ns_info, cs_info, alis_search, version, time"
        
        # Ignored users can't do anything
        return False, "You don't have permission to use IRC commands."
    
    def execute(
        self,
        command: str,
        args: Optional[List[str]] = None,
        channel: Optional[str] = None,
        permission_level: str = "normal",
        **kwargs
    ) -> str:
        """
        Execute an IRC command.
        
        Args:
            command: IRC command to execute
            args: Command arguments
            channel: Target channel (optional)
            permission_level: User's permission level
            
        Returns:
            Command output or error message
        """
        args = args or []
        
        # Check permission
        allowed, error_msg = self._check_permission(command, permission_level)
        if not allowed:
            log_warning(f"IRC command '{command}' denied for permission level '{permission_level}'")
            return f"Permission denied: {error_msg}"
        
        log_info(f"Executing IRC command: {command} {args} (permission: {permission_level})")
        
        try:
            # Call Go bot's IRC execute endpoint
            response = requests.post(
                f"{self.GO_BOT_CALLBACK_URL}/irc/execute",
                json={
                    "command": command,
                    "args": args,
                    "channel": channel,
                },
                timeout=self.timeout
            )
            
            if response.status_code == 200:
                result = response.json()
                if result.get("status") == "success":
                    output = result.get("output", "Command executed successfully.")
                    log_success(f"IRC command '{command}' executed successfully")
                    return output
                else:
                    error = result.get("error", "Unknown error")
                    log_error(f"IRC command '{command}' failed: {error}")
                    return f"Error: {error}"
            else:
                log_error(f"IRC command endpoint returned {response.status_code}")
                return f"Error: Failed to execute IRC command (HTTP {response.status_code})"
                
        except requests.exceptions.ConnectionError:
            log_error("Cannot connect to Go bot IRC callback endpoint")
            return "Error: Cannot connect to IRC command service. The bot may not support this feature yet."
        except requests.exceptions.Timeout:
            log_error(f"IRC command '{command}' timed out")
            return f"Error: Command timed out after {self.timeout} seconds."
        except Exception as e:
            log_error(f"IRC command error: {e}")
            return f"Error executing IRC command: {str(e)}"
