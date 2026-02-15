"""
AI configuration loader.

Loads and validates AI settings from TOML configuration file.
"""

import os
from pathlib import Path
from typing import Optional, List
import tomli
from dotenv import load_dotenv

# Load .env file
load_dotenv()


class AIConfig:
    """AI configuration settings."""
    
    def __init__(self, config_path: Optional[str] = None):
        """
        Load AI configuration from TOML file.
        
        Args:
            config_path: Path to config file. If None, uses default location.
        """
        if config_path is None:
            # Default to api/config/ai_settings.toml
            config_path = Path(__file__).parent.parent / "config" / "ai_settings.toml"
        
        with open(config_path, "rb") as f:
            config = tomli.load(f)
        
        # Model settings
        self.model_name: str = config["model"]["name"]
        self.reasoning_effort: str = config["model"]["reasoning_effort"]
        self.verbosity: str = config["model"]["verbosity"]
        
        # Limits
        self.max_output_tokens: int = config["limits"]["max_output_tokens"]
        self.timeout: int = config["limits"]["timeout"]
        
        # System prompt
        self.system_prompt: str = config["system_prompt"]["text"]
        
        # Tools
        self.web_search_enabled: bool = config["tools"]["web_search_enabled"]
        self.python_exec_enabled: bool = config["tools"]["python_exec_enabled"]
        self.flux_create_enabled: bool = config["tools"]["flux_create_enabled"]
        self.flux_edit_enabled: bool = config["tools"]["flux_edit_enabled"]
        self.image_analysis_enabled: bool = config["tools"]["image_analysis_enabled"]
        self.fetch_url_enabled: bool = config["tools"].get("fetch_url_enabled", True)
        self.user_rules_enabled: bool = config["tools"].get("user_rules_enabled", True)
        
        self.chat_history_enabled: bool = config["tools"].get("chat_history_enabled", True)
        self.paste_enabled: bool = config["tools"].get("paste_enabled", True)
        self.shell_exec_enabled: bool = config["tools"].get("shell_exec_enabled", True)
        self.voice_speak_enabled: bool = config["tools"].get("voice_speak_enabled", True)
        self.null_response_enabled: bool = config["tools"].get("null_response_enabled", True)
        self.bug_report_enabled: bool = config["tools"].get("bug_report_enabled", True)
        self.gpt_image_enabled: bool = config["tools"].get("gpt_image_enabled", True)
        self.gemini_image_enabled: bool = config["tools"].get("gemini_image_enabled", True)
        self.usage_stats_enabled: bool = config["tools"].get("usage_stats_enabled", True)
        self.youtube_search_enabled: bool = config["tools"].get("youtube_search_enabled", True)
        self.source_code_enabled: bool = config["tools"].get("source_code_enabled", True)
        self.irc_command_enabled: bool = config["tools"].get("irc_command_enabled", True)
        self.claude_code_enabled: bool = config["tools"].get("claude_code_enabled", True)
        
        # Knowledge Base tools
        self.kb_learn_enabled: bool = config["tools"].get("kb_learn_enabled", True)
        self.kb_search_enabled: bool = config["tools"].get("kb_search_enabled", True)
        self.kb_list_enabled: bool = config["tools"].get("kb_list_enabled", True)
        self.kb_forget_enabled: bool = config["tools"].get("kb_forget_enabled", True)
        
        # Moltbook posting
        self.moltbook_post_enabled: bool = config["tools"].get("moltbook_post_enabled", True)
        
        # Reminder tool
        self.reminder_enabled: bool = config["tools"].get("reminder_enabled", True)
        
        # Shell execution settings
        self.shell_exec_timeout: int = config.get("shell_exec", {}).get("timeout", 30)
        
        # IRC command settings
        self.irc_command_timeout: int = config.get("irc_command", {}).get("timeout", 30)
        
        # Web search settings
        self.web_search_external_access: bool = config["web_search"]["external_web_access"]
        self.web_search_allowed_domains: List[str] = config["web_search"]["allowed_domains"]
        
        # Python execution settings (Firecracker VM)
        self.python_exec_timeout: int = config["python_exec"].get("execution_timeout", 180)
        
        # OpenAI API key from environment
        self.openai_api_key: str = os.getenv("OPENAI_API_KEY", "")
        if not self.openai_api_key:
            raise ValueError("OPENAI_API_KEY environment variable not set")
    
    def get_enabled_tools(self) -> List[str]:
        """Get list of enabled tool names."""
        tools = []
        if self.web_search_enabled:
            tools.append("web_search")
        if self.python_exec_enabled:
            tools.append("python_exec")
        if self.flux_create_enabled:
            tools.append("flux_create_image")
        if self.flux_edit_enabled:
            tools.append("flux_edit_image")
        if self.image_analysis_enabled:
            tools.append("analyze_image")
        if self.fetch_url_enabled:
            tools.append("fetch_url")
        if self.user_rules_enabled:
            tools.append("manage_user_rules")
        if self.chat_history_enabled:
            tools.append("query_chat_history")
        if self.paste_enabled:
            tools.append("create_paste")
        if self.shell_exec_enabled:
            tools.append("execute_shell")
        if self.voice_speak_enabled:
            tools.append("voice_speak")
        if self.null_response_enabled:
            tools.append("null_response")
        if self.bug_report_enabled:
            tools.append("bug_report")
        if self.gpt_image_enabled:
            tools.append("gpt_image")
        if self.gemini_image_enabled:
            tools.append("gemini_image")
        if self.youtube_search_enabled:
            tools.append("youtube_search")
        if self.source_code_enabled:
            tools.append("source_code")
        if self.irc_command_enabled:
            tools.append("irc_command")
        if self.claude_code_enabled:
            tools.append("claude_tech")
        if self.reminder_enabled:
            tools.append("reminder")
        return tools
