"""
Claude Tech tool implementation.

Provides expert technical assistance using Claude Opus via AWS Bedrock.
Covers coding, Linux/Unix, networking, DevOps, sysadmin, and general tech.
Long responses are automatically pasted to botbin.
"""

import os
import json
import tempfile
import requests
from typing import Any, Dict, Optional
from .base import Tool


class ClaudeCodeTool(Tool):
    """Claude Opus technical expert via AWS Bedrock."""
    
    BEDROCK_REGION = "eu-west-1"
    BEDROCK_MODEL = "eu.anthropic.claude-opus-4-5-20251101-v1:0"
    BOTBIN_URL = "https://botbin.net/upload"
    
    # Threshold for pasting (characters) - IRC messages are ~400 bytes
    PASTE_THRESHOLD = 800
    
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
            "description": """Ask Claude Opus 4.5 (Anthropic's most capable model) for expert technical help. Use for:
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
            with tempfile.NamedTemporaryFile(mode="w", delete=False, suffix=".md") as tmp:
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
            paste_url = self._paste_to_botbin(response, f"claude_code.md")
            
            if paste_url:
                summary = self._extract_summary(response)
                return f"{summary} | Full response: {paste_url}"
            else:
                # Fallback: truncate if paste fails
                return response[:self.PASTE_THRESHOLD] + f"... [Response truncated - {len(response)} chars total]"
        
        return response
