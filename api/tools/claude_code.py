"""
Claude Tech tool implementation.

Provides expert technical assistance using Claude Opus via AWS Bedrock.
Covers coding, Linux/Unix, networking, DevOps, sysadmin, and general tech.
Long responses are automatically pasted to botbin.
"""

import os
import json
import re
import tempfile
import requests
from typing import Any, Dict, List, Optional, Tuple
from .base import Tool


class ClaudeCodeTool(Tool):
    """Claude Opus technical expert via AWS Bedrock."""
    
    BEDROCK_REGION = "eu-west-1"
    BEDROCK_MODEL = "eu.anthropic.claude-opus-4-6-v1"
    BOTBIN_URL = "https://botbin.net/upload"
    
    # Threshold for pasting (characters) - IRC messages are ~400 bytes
    PASTE_THRESHOLD = 800

    # Map common markdown language tags to file extensions
    LANGUAGE_EXTENSIONS = {
        "python": "py",
        "py": "py",
        "javascript": "js",
        "js": "js",
        "node": "js",
        "nodejs": "js",
        "typescript": "ts",
        "ts": "ts",
        "c": "c",
        "cpp": "cpp",
        "c++": "cpp",
        "cc": "cpp",
        "cxx": "cpp",
        "csharp": "cs",
        "c#": "cs",
        "cs": "cs",
        "go": "go",
        "golang": "go",
        "rust": "rs",
        "rs": "rs",
        "java": "java",
        "kotlin": "kt",
        "kt": "kt",
        "swift": "swift",
        "php": "php",
        "ruby": "rb",
        "rb": "rb",
        "bash": "sh",
        "sh": "sh",
        "shell": "sh",
        "zsh": "sh",
        "powershell": "ps1",
        "ps1": "ps1",
        "sql": "sql",
        "html": "html",
        "css": "css",
        "scss": "scss",
        "sass": "sass",
        "json": "json",
        "yaml": "yml",
        "yml": "yml",
        "toml": "toml",
        "xml": "xml",
        "markdown": "md",
        "md": "md",
        "lua": "lua",
        "perl": "pl",
        "r": "r",
    }
    
    def __init__(self):
        """Initialize Claude tech tool."""
        self.bearer_token = os.environ.get("AWS_BEARER_TOKEN_BEDROCK")
        self.botbin_key = os.environ.get("BOTBIN_API_KEY")
        self.bedrock_url = f"https://bedrock-runtime.{self.BEDROCK_REGION}.amazonaws.com/model/{self.BEDROCK_MODEL}/converse"
    
    @property
    def name(self) -> str:
        return "claude_tech"
    
    def get_definition(self) -> Dict[str, Any]:
        """Get tool definition for OpenAI API."""
        return {
            "type": "function",
            "name": "claude_tech",
            "description": """Ask Claude Opus 4.6 (Anthropic's most capable model) for expert technical help. Use for:
- Programming: code review, debugging, architecture, algorithms, any language
- Linux/Unix: shell commands, bash scripting, system administration, permissions
- Networking: TCP/IP, DNS, firewalls, routing, troubleshooting connectivity
- DevOps: Docker, Kubernetes, CI/CD, infrastructure as code, deployment
- Databases: SQL, NoSQL, query optimization, schema design
- Security: encryption, authentication, hardening, vulnerability analysis
- Hardware: PC building, components, troubleshooting, compatibility
- CLI tools: git, vim, tmux, awk, sed, grep, find, and other command-line utilities

Returns detailed explanations with examples. Long responses auto-paste to botbin.""",
            "parameters": {
                "type": "object",
                "properties": {
                    "question": {
                        "type": "string",
                        "description": "The technical question or request. Be specific about what you're trying to achieve."
                    },
                    "context": {
                        "type": "string",
                        "description": "Optional additional context: error messages, current setup, OS/distro, constraints, what you've tried."
                    },
                    "topic": {
                        "type": "string",
                        "description": "Primary topic area (e.g., 'linux', 'python', 'networking', 'docker', 'git'). Helps focus the response."
                    }
                },
                "required": ["question"],
                "additionalProperties": False
            }
        }
    
    def _call_bedrock(self, prompt: str) -> str:
        """Call AWS Bedrock with Claude Opus."""
        if not self.bearer_token:
            return "Error: AWS_BEARER_TOKEN_BEDROCK not configured"
        
        headers = {
            "Authorization": f"Bearer {self.bearer_token}",
            "Content-Type": "application/json"
        }
        
        payload = {
            "messages": [
                {
                    "role": "user",
                    "content": [{"text": prompt}]
                }
            ]
        }
        
        try:
            response = requests.post(
                self.bedrock_url,
                headers=headers,
                json=payload,
                timeout=120  # 2 minute timeout for complex questions
            )
            
            if response.status_code != 200:
                return f"Error: Bedrock API returned {response.status_code}: {response.text[:200]}"
            
            result = response.json()
            
            # Extract text from response
            output = result.get("output", {})
            message = output.get("message", {})
            content = message.get("content", [])
            
            if content and len(content) > 0:
                return content[0].get("text", "No response text")
            
            return "Error: Empty response from Claude"
            
        except requests.exceptions.Timeout:
            return "Error: Request to Claude timed out"
        except requests.exceptions.RequestException as e:
            return f"Error: Request failed - {str(e)}"
        except json.JSONDecodeError as e:
            return f"Error: Invalid JSON response - {str(e)}"
    
    def _paste_to_botbin(self, content: str, filename: str = "code.md") -> Optional[str]:
        """Paste content to botbin and return URL."""
        if not self.botbin_key:
            return None
        
        try:
            file_suffix = os.path.splitext(filename)[1] or ".txt"
            with tempfile.NamedTemporaryFile(mode="w", encoding="utf-8", delete=False, suffix=file_suffix) as tmp:
                tmp.write(content)
                tmp_path = tmp.name
            
            try:
                with open(tmp_path, "rb") as f:
                    response = requests.post(
                        self.BOTBIN_URL,
                        headers={"Authorization": f"Bearer {self.botbin_key}"},
                        files={"file": (filename, f)},
                        data={"retention": "168h"},  # 1 week
                        timeout=30
                    )
                
                if response.status_code in (200, 201):
                    result = response.json()
                    return result.get("url")
            finally:
                os.unlink(tmp_path)
                
        except Exception:
            pass
        
        return None

    def _normalize_language_tag(self, tag: str) -> str:
        """Normalize a markdown code fence language tag."""
        if not tag:
            return ""
        normalized = tag.strip().lower()
        normalized = normalized.strip("{}").lstrip(".")
        if normalized.startswith("language-"):
            normalized = normalized[len("language-"):]
        # Drop extra metadata like "python title=foo.py"
        normalized = normalized.split()[0]
        return normalized

    def _language_to_extension(self, language: str) -> Optional[str]:
        """Map a language hint to a file extension."""
        normalized = self._normalize_language_tag(language)
        if not normalized:
            return None
        return self.LANGUAGE_EXTENSIONS.get(normalized)

    def _guess_extension_from_hints(self, *hints: str) -> Optional[str]:
        """Guess file extension from free-form topic/question hints."""
        for hint in hints:
            if not hint:
                continue
            tokens = re.findall(r"[a-zA-Z0-9+#_.-]+", hint.lower())
            for token in tokens:
                ext = self._language_to_extension(token)
                if ext:
                    return ext
        return None

    def _extract_fenced_code_blocks(self, text: str) -> List[Tuple[str, str]]:
        """Extract all triple-backtick fenced code blocks."""
        blocks: List[Tuple[str, str]] = []
        in_block = False
        language = ""
        current_lines: List[str] = []

        for line in text.splitlines():
            stripped = line.strip()

            if not in_block:
                if stripped.startswith("```"):
                    in_block = True
                    language = stripped[3:].strip()
                    current_lines = []
                continue

            if stripped.startswith("```"):
                code = "\n".join(current_lines).strip("\n")
                if code:
                    blocks.append((language, code))
                in_block = False
                language = ""
                current_lines = []
                continue

            current_lines.append(line)

        # Keep content if Claude forgets to close the final fence
        if in_block and current_lines:
            code = "\n".join(current_lines).strip("\n")
            if code:
                blocks.append((language, code))

        return blocks

    def _build_paste_payload(self, response: str, topic_hint: str, question_hint: str) -> Tuple[str, str]:
        """
        Build clean paste payload from Claude response.

        If fenced code blocks are present, remove fences and paste raw code.
        """
        code_blocks = self._extract_fenced_code_blocks(response)

        if code_blocks:
            if len(code_blocks) == 1:
                paste_content = code_blocks[0][1].strip("\n")
            else:
                paste_content = "\n\n".join(block for _, block in code_blocks).strip("\n")

            primary_language, _ = max(code_blocks, key=lambda block: len(block[1]))
            extension = (
                self._language_to_extension(primary_language)
                or self._guess_extension_from_hints(topic_hint)
                or self._guess_extension_from_hints(question_hint)
                or "txt"
            )
            return f"{paste_content}\n", f"claude_code.{extension}"

        # No fenced blocks: keep original text, but avoid markdown default
        extension = self._guess_extension_from_hints(topic_hint)
        filename = f"claude_code.{extension}" if extension else "claude_response.txt"
        return f"{response.rstrip()}\n", filename
    
    def _extract_summary(self, response: str, max_length: int = 300) -> str:
        """Extract a brief summary from the response."""
        # Try to get the first paragraph or meaningful chunk
        lines = response.strip().split('\n')
        summary_lines = []
        char_count = 0
        
        for line in lines:
            # Skip code blocks for summary
            if line.startswith('```'):
                continue
            # Skip headers but note them
            if line.startswith('#'):
                line = line.lstrip('#').strip()
            
            if line.strip():
                if char_count + len(line) > max_length:
                    break
                summary_lines.append(line.strip())
                char_count += len(line)
        
        summary = ' '.join(summary_lines)
        if len(summary) > max_length:
            summary = summary[:max_length-3] + "..."
        
        return summary if summary else "See full response at link"
    
    def execute(
        self,
        question: str,
        context: str = "",
        topic: str = "",
        language: str = "",  # Keep for backward compat
        **kwargs
    ) -> str:
        """
        Get technical help from Claude Opus.
        
        Args:
            question: The technical question
            context: Optional additional context
            topic: Primary topic area (or language for code)
            language: Alias for topic (backward compat)
            
        Returns:
            Response with paste URL if long, or direct response if short
        """
        if not self.bearer_token:
            return "Error: AWS_BEARER_TOKEN_BEDROCK not configured"
        
        # Use language as topic if topic not specified (backward compat)
        effective_topic = topic or language
        
        # Build the prompt
        prompt_parts = [
            "You are an expert technical assistant with deep knowledge of:",
            "- Programming (all languages, frameworks, best practices)",
            "- Linux/Unix systems, shell scripting, system administration", 
            "- Networking, protocols, troubleshooting",
            "- DevOps, containers, CI/CD, cloud infrastructure",
            "- Databases, security, hardware",
            "",
            "Provide clear, practical answers with examples and commands where relevant.",
            "For code, use best practices and explain your reasoning.",
            ""
        ]
        
        if effective_topic:
            prompt_parts.append(f"Primary topic: {effective_topic}")
        
        if context:
            prompt_parts.append(f"\nContext:\n{context}")
        
        prompt_parts.append(f"\nQuestion:\n{question}")
        
        prompt = '\n'.join(prompt_parts)
        
        # Call Claude
        response = self._call_bedrock(prompt)
        
        if response.startswith("Error:"):
            return response
        
        # Check if response needs pasting
        if len(response) > self.PASTE_THRESHOLD:
            # Paste to botbin
            paste_content, paste_filename = self._build_paste_payload(
                response=response,
                topic_hint=effective_topic,
                question_hint=question,
            )
            paste_url = self._paste_to_botbin(paste_content, paste_filename)
            
            if paste_url:
                summary = self._extract_summary(response)
                return f"{summary} | Full response: {paste_url}"
            else:
                # Fallback: truncate if paste fails
                return response[:self.PASTE_THRESHOLD] + f"... [Response truncated - {len(response)} chars total]"
        
        return response
