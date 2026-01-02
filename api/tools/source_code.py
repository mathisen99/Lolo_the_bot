"""
Source code introspection tool.

Allows the bot to read and browse its own source code to answer questions
about how it works. Implements strict security boundaries to prevent
access to sensitive files.

Uses system tools: rg (ripgrep), wc
"""

import os
import subprocess
import shutil
from pathlib import Path
from typing import Any, Dict, Optional, Tuple
from .base import Tool


class SourceCodeTool(Tool):
    """Tool for browsing, searching, and reading the bot's own source code."""
    
    # Project root (where the bot runs from)
    PROJECT_ROOT = Path(__file__).parent.parent.parent.resolve()
    
    # ALLOWED base paths (whitelist approach - only these can be accessed)
    ALLOWED_PATHS = [
        "internal",         # Go bot code
        "api",              # Python API code  
        "cmd",              # Entry points
        "go.mod",           # Go dependencies
        "go.sum",           # Go dependency checksums
        "requirements.txt", # Python dependencies
        "README.md",        # Documentation
        "LICENSE",          # License
        "how-to-use.md",    # Usage docs
    ]
    
    # BLOCKED paths (explicit denials that override allowed paths)
    BLOCKED_PATTERNS = [
        # Secrets and credentials
        ".env",
        "*.key",
        "*.pem", 
        "*.p12",
        "*.pfx",
        
        # Config with secrets (root config/, not api/config/)
        "config/",
        
        # Data and runtime
        "data/",
        "*.db",
        "*.log",
        
        # Git and IDE
        ".git/",
        ".kiro/",
        ".vscode/",
        ".idea/",
        
        # Virtual environments and cache
        ".venv/",
        "venv/",
        "__pycache__/",
        "*.pyc",
        
        # Subprojects with their own secrets
        "CosyVoice/",
        "IsolateVoice/",
        
        # Firecracker VM (has keys, sockets)
        "scripts/firecracker/",
        
        # Non-code directories
        "img/",
        "docs/",
        "images/",
        "uploads/",
    ]
    
    # Allowed file extensions
    ALLOWED_EXTENSIONS = [
        ".go", ".py", ".sql", ".md", ".toml", ".txt",
        ".mod", ".sum", ".json", ".yaml", ".yml",
    ]
    
    # Limits
    MAX_LINES = 500
    MAX_FILE_SIZE = 50 * 1024  # 50KB
    MAX_SEARCH_RESULTS = 30
    
    def __init__(self):
        """Initialize source code tool."""
        self._rg_available = shutil.which("rg") is not None
    
    @property
    def name(self) -> str:
        return "source_code"
    
    def get_definition(self) -> Dict[str, Any]:
        """Get tool definition for OpenAI API."""
        return {
            "type": "function",
            "name": self.name,
            "description": """Browse, search, and read Lolo's own source code.

=== PROJECT MAP (use this to go directly to files) ===
GO BOT (internal/):
  irc/client.go - IRC connection, SASL, reconnection
  handler/handler.go - Message routing, command dispatch
  handler/mention.go - Mention detection, API calls
  handler/api_client.go - Python API HTTP client
  commands/ - Core commands (!help, !admin, !ignore)
  user/manager.go - Permissions, WHOIS
  database/db.go - SQLite, message logging
  ratelimit/ - Rate limiting
  splitter/ - Message splitting
  config/ - TOML loading
  output/ - Terminal logging

PYTHON API (api/):
  main.py - FastAPI startup
  router.py - HTTP endpoints
  mention.py - AI mention handler
  ai/client.py - GPT integration, tool loop
  ai/config.py - Settings loader
  config/ai_settings.toml - System prompt, model config
  tools/*.py - Tool implementations
  commands/*.py - IRC commands
  utils/ - Helpers

=== ACTIONS ===
search: Find code by pattern (ripgrep). USE THIS FIRST when unsure where code is.
list_files: Browse directory contents
read_file: Read file (use start_line/end_line to save tokens)

=== WORKFLOW ===
1. Know the file? → read_file directly
2. Unsure? → search first, then read_file with line range

Blocked: .env, config/bot.toml, data/, all secrets""",
            "parameters": {
                "type": "object",
                "properties": {
                    "action": {
                        "type": "string",
                        "enum": ["search", "list_files", "read_file"],
                        "description": "search=find code, list_files=browse dir, read_file=read content"
                    },
                    "path": {
                        "type": "string",
                        "description": "Relative path. Optional for search (searches all allowed paths)"
                    },
                    "query": {
                        "type": "string",
                        "description": "Search pattern (for search action). Supports regex."
                    },
                    "start_line": {
                        "type": "integer",
                        "description": "Start line for read_file (1-indexed)"
                    },
                    "end_line": {
                        "type": "integer",
                        "description": "End line for read_file (inclusive)"
                    }
                },
                "required": ["action"],
                "additionalProperties": False
            }
        }
    
    def _is_path_allowed(self, rel_path: str) -> Tuple[bool, str]:
        """Check if a path is allowed. Returns (allowed, error_msg)."""
        rel_path = rel_path.strip().lstrip("/").lstrip("\\")
        
        if not rel_path:
            return False, "Empty path not allowed"
        
        # Resolve to prevent traversal attacks
        try:
            abs_path = (self.PROJECT_ROOT / rel_path).resolve()
        except (ValueError, OSError) as e:
            return False, f"Invalid path: {e}"
        
        # Must be within project root
        try:
            abs_path.relative_to(self.PROJECT_ROOT)
        except ValueError:
            return False, "Access denied: Path outside project"
        
        rel_normalized = str(abs_path.relative_to(self.PROJECT_ROOT))
        
        # Check blocked patterns (deny first)
        for pattern in self.BLOCKED_PATTERNS:
            if pattern.endswith("/"):
                dir_name = pattern.rstrip("/")
                if rel_normalized.startswith(dir_name) or f"/{dir_name}" in f"/{rel_normalized}":
                    return False, f"Access denied: {dir_name}/ is restricted"
            elif pattern.startswith("*."):
                if rel_normalized.endswith(pattern[1:]):
                    return False, f"Access denied: {pattern} files restricted"
            else:
                if rel_normalized == pattern or rel_normalized.startswith(pattern):
                    return False, f"Access denied: {pattern} is restricted"
        
        # Block root config/ but allow api/config/
        if rel_normalized.startswith("config/") or rel_normalized == "config":
            return False, "Access denied: config/ contains secrets"
        
        # Check allowlist
        for allowed in self.ALLOWED_PATHS:
            if rel_normalized == allowed or rel_normalized.startswith(allowed + "/"):
                return True, ""
        
        return False, "Access denied: Path not in allowed directories"
    
    def _search(self, query: str, path: Optional[str] = None) -> str:
        """Search code using ripgrep."""
        if not self._rg_available:
            return "Error: ripgrep (rg) not installed"
        
        if not query:
            return "Error: query is required for search"
        
        # Determine search paths
        if path:
            allowed, error = self._is_path_allowed(path)
            if not allowed:
                return f"Error: {error}"
            search_paths = [str(self.PROJECT_ROOT / path)]
        else:
            # Search all allowed directory paths
            search_paths = []
            for p in self.ALLOWED_PATHS:
                full = self.PROJECT_ROOT / p
                if full.exists() and full.is_dir():
                    search_paths.append(str(full))
        
        if not search_paths:
            return "Error: No valid paths to search"
        
        # Build rg command with safety limits
        cmd = [
            "rg",
            "--max-count=5",           # Max 5 matches per file
            "--max-filesize=100K",     # Skip large files
            "-n",                       # Line numbers
            "--no-heading",            # file:line:match format
            "-i",                       # Case insensitive
            "--type=go",
            "--type=py",
            "--type=sql",
            "--type=md",
            "--type=toml",
            "--type=json",
            "--type=yaml",
            query
        ] + search_paths
        
        try:
            result = subprocess.run(
                cmd,
                capture_output=True,
                text=True,
                timeout=10,
                cwd=str(self.PROJECT_ROOT)
            )
            output = result.stdout.strip()
        except subprocess.TimeoutExpired:
            return "Error: Search timed out"
        except Exception as e:
            return f"Error running search: {e}"
        
        if not output:
            return f"No matches found for '{query}'"
        
        # Filter results through security check and limit count
        lines = output.split("\n")
        filtered = []
        for line in lines:
            if ":" in line:
                file_part = line.split(":")[0]
                try:
                    rel = str(Path(file_part).relative_to(self.PROJECT_ROOT))
                    if self._is_path_allowed(rel)[0]:
                        # Make path relative in output
                        filtered.append(line.replace(str(self.PROJECT_ROOT) + "/", ""))
                except ValueError:
                    continue
            
            if len(filtered) >= self.MAX_SEARCH_RESULTS:
                filtered.append(f"... (limited to {self.MAX_SEARCH_RESULTS} results)")
                break
        
        if not filtered:
            return f"No accessible matches for '{query}'"
        
        return f"Search results for '{query}':\n" + "\n".join(filtered)
    
    def _list_files(self, path: str) -> str:
        """List directory contents."""
        allowed, error = self._is_path_allowed(path)
        if not allowed:
            return f"Error: {error}"
        
        abs_path = (self.PROJECT_ROOT / path).resolve()
        
        if not abs_path.exists():
            return f"Error: Path '{path}' does not exist"
        
        if abs_path.is_file():
            size = abs_path.stat().st_size
            lines = sum(1 for _ in open(abs_path, errors="replace"))
            return f"File: {path} ({size} bytes, {lines} lines)"
        
        if not abs_path.is_dir():
            return f"Error: '{path}' is not a file or directory"
        
        entries = []
        try:
            for entry in sorted(abs_path.iterdir()):
                rel_entry = str(entry.relative_to(self.PROJECT_ROOT))
                if not self._is_path_allowed(rel_entry)[0]:
                    continue
                
                if entry.is_dir():
                    try:
                        count = sum(1 for e in entry.iterdir() 
                                   if self._is_path_allowed(str(e.relative_to(self.PROJECT_ROOT)))[0])
                        entries.append(f"📁 {entry.name}/ ({count} items)")
                    except PermissionError:
                        entries.append(f"📁 {entry.name}/")
                else:
                    size = entry.stat().st_size
                    size_str = f"{size}B" if size < 1024 else f"{size // 1024}KB"
                    entries.append(f"📄 {entry.name} ({size_str})")
        except PermissionError:
            return f"Error: Permission denied"
        
        if not entries:
            return f"Directory '{path}' is empty or restricted"
        
        return f"Contents of {path}/:\n" + "\n".join(entries)
    
    def _read_file(self, path: str, start_line: Optional[int] = None, 
                   end_line: Optional[int] = None) -> str:
        """Read file contents, optionally with line range."""
        allowed, error = self._is_path_allowed(path)
        if not allowed:
            return f"Error: {error}"
        
        abs_path = (self.PROJECT_ROOT / path).resolve()
        
        if not abs_path.exists():
            return f"Error: File '{path}' does not exist"
        
        if not abs_path.is_file():
            return f"Error: '{path}' is a directory. Use list_files."
        
        # Check extension
        ext = abs_path.suffix.lower()
        if ext and ext not in self.ALLOWED_EXTENSIONS:
            if abs_path.name not in ["go.mod", "go.sum", "Makefile", "Dockerfile"]:
                return f"Error: File type '{ext}' not readable"
        
        # Check size
        size = abs_path.stat().st_size
        if size > self.MAX_FILE_SIZE:
            return f"Error: File too large ({size // 1024}KB > {self.MAX_FILE_SIZE // 1024}KB)"
        
        # Read file
        try:
            with open(abs_path, "r", encoding="utf-8", errors="replace") as f:
                all_lines = f.readlines()
        except Exception as e:
            return f"Error reading file: {e}"
        
        total_lines = len(all_lines)
        
        # Apply line range
        if start_line is not None or end_line is not None:
            start = (start_line or 1) - 1  # Convert to 0-indexed
            end = end_line or total_lines
            
            if start < 0:
                start = 0
            if end > total_lines:
                end = total_lines
            
            lines = all_lines[start:end]
            line_info = f"Lines {start + 1}-{end} of {total_lines}"
        else:
            # No range specified - apply max limit
            if total_lines > self.MAX_LINES:
                lines = all_lines[:self.MAX_LINES]
                line_info = f"Lines 1-{self.MAX_LINES} of {total_lines} (truncated)"
            else:
                lines = all_lines
                line_info = f"{total_lines} lines"
        
        content = "".join(lines)
        
        return f"=== {path} ({line_info}) ===\n\n{content}"
    
    def execute(self, action: str, path: Optional[str] = None, 
                query: Optional[str] = None, start_line: Optional[int] = None,
                end_line: Optional[int] = None, **kwargs) -> str:
        """Execute source code action."""
        if action == "search":
            return self._search(query or "", path)
        elif action == "list_files":
            if not path:
                return "Error: path is required for list_files"
            return self._list_files(path)
        elif action == "read_file":
            if not path:
                return "Error: path is required for read_file"
            return self._read_file(path, start_line, end_line)
        else:
            return f"Error: Unknown action '{action}'"
