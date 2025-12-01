"""
Shell execution tool - OWNER ONLY.

Allows the bot owner to execute shell commands on the system.
This is a privileged tool that requires owner permission level.
"""

import subprocess
import shlex
from typing import Any, Dict, Optional
from .base import Tool


class ShellExecTool(Tool):
    """Tool for executing shell commands. Owner only."""
    
    def __init__(self, timeout: int = 30):
        """
        Initialize shell execution tool.
        
        Args:
            timeout: Command execution timeout in seconds (default 30)
        """
        self.timeout = timeout
    
    @property
    def name(self) -> str:
        return "execute_shell"
    
    def get_definition(self) -> Dict[str, Any]:
        """Get tool definition for OpenAI API."""
        return {
            "type": "function",
            "name": "execute_shell",
            "description": """Execute shell commands on the Linux system. OWNER ONLY - requires owner permission level.

Use this when the owner asks to:
- Check system status (uptime, disk space, memory, processes)
- Run diagnostic commands (curl, ping, netstat, etc.)
- Manage services or files
- Execute scripts or chain multiple commands
- Any system administration task

The command runs in a bash shell with full system access including sudo.
Commands can be chained using && or || or piped with |.

Examples:
- "check disk space" -> command="df -h"
- "show memory usage" -> command="free -h"
- "restart nginx" -> command="sudo systemctl restart nginx"
- "check if google is reachable" -> command="ping -c 3 google.com"
- "show running docker containers" -> command="docker ps"
- "get system info" -> command="uname -a && uptime && free -h"

IMPORTANT: This tool will REFUSE to execute if the requesting user is not the owner.""",
            "parameters": {
                "type": "object",
                "properties": {
                    "command": {
                        "type": "string",
                        "description": "The shell command to execute. Can include pipes, redirects, and command chaining (&&, ||, ;)."
                    },
                    "working_dir": {
                        "type": ["string", "null"],
                        "description": "Working directory for command execution. Defaults to current directory if not specified."
                    },
                    "timeout": {
                        "type": ["integer", "null"],
                        "description": "Custom timeout in seconds for this command. Defaults to 30 seconds."
                    }
                },
                "required": ["command"],
                "additionalProperties": False
            }
        }
    
    def execute(
        self,
        command: str,
        working_dir: Optional[str] = None,
        timeout: Optional[int] = None,
        permission_level: str = "normal",
        **kwargs
    ) -> str:
        """
        Execute a shell command.
        
        Args:
            command: Shell command to execute
            working_dir: Working directory (optional)
            timeout: Command timeout in seconds (optional)
            permission_level: User's permission level - MUST be "owner"
            
        Returns:
            Command output or error message
        """
        # CRITICAL: Owner-only check
        if permission_level != "owner":
            return "Permission denied: This tool is restricted to the bot owner only."
        
        if not command or not command.strip():
            return "Error: No command provided."
        
        exec_timeout = timeout if timeout is not None else self.timeout
        
        try:
            # Run command in bash shell to support pipes, redirects, chaining
            result = subprocess.run(
                command,
                shell=True,
                executable="/bin/bash",
                capture_output=True,
                text=True,
                timeout=exec_timeout,
                cwd=working_dir
            )
            
            output_parts = []
            
            if result.stdout:
                output_parts.append(f"STDOUT:\n{result.stdout.strip()}")
            
            if result.stderr:
                output_parts.append(f"STDERR:\n{result.stderr.strip()}")
            
            if not output_parts:
                output_parts.append("(no output)")
            
            output_parts.append(f"Exit code: {result.returncode}")
            
            output = "\n\n".join(output_parts)
            
            # Truncate if too long (keep last part with exit code)
            max_len = 4000
            if len(output) > max_len:
                output = f"(output truncated, showing last {max_len} chars)\n...{output[-(max_len-50):]}"
            
            return output
            
        except subprocess.TimeoutExpired:
            return f"Error: Command timed out after {exec_timeout} seconds."
        
        except Exception as e:
            return f"Error executing command: {str(e)}"
