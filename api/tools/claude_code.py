"""
Claude Code tool implementation.

Provides coding assistance using Claude Opus via AWS Bedrock.
Long responses are automatically pasted to botbin.
"""

import os
import json
import tempfile
import requests
from typing import Any, Dict, Optional
from .base import Tool


class ClaudeCodeTool(Tool):
    """Claude Opus coding assistant via AWS Bedrock."""
    
    BEDROCK_REGION = "eu-west-1"
    BEDROCK_MODEL = "eu.anthropic.claude-opus-4-5-20251101-v1:0"
    BOTBIN_URL = "https://botbin.net/upload"
    
    # Threshold for pasting (characters) - IRC messages are ~400 bytes
    PASTE_THRESHOLD = 800
    
    def __init__(self):
        """Initialize Claude code tool."""
        self.bearer_token = os.environ.get("AWS_BEARER_TOKEN_BEDROCK")
        self.botbin_key = os.environ.get("BOTBIN_API_KEY")
        self.bedrock_url = f"https://bedrock-runtime.{self.BEDROCK_REGION}.amazonaws.com/model/{self.BEDROCK_MODEL}/converse"
    
    @property
    def name(self) -> str:
        return "claude_code"
    
    def get_definition(self) -> Dict[str, Any]:
        """Get tool definition for OpenAI API."""
        return {
            "type": "function",
            "name": "claude_code",
            "description": "Ask Claude Opus (Anthropic's most capable model) for help with coding questions, code review, debugging, architecture advice, or complex programming problems. Returns detailed explanations with full code examples. Long responses are automatically pasted to botbin for easy viewing. Use this for in-depth coding assistance.",
            "parameters": {
                "type": "object",
                "properties": {
                    "question": {
                        "type": "string",
                        "description": "The coding question or request. Be specific about the language, framework, and what you're trying to achieve."
                    },
                    "context": {
                        "type": "string",
                        "description": "Optional additional context like existing code, error messages, or constraints."
                    },
                    "language": {
                        "type": "string",
                        "description": "Primary programming language (e.g., 'python', 'go', 'javascript', 'rust'). Helps Claude provide idiomatic code."
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
        language: str = "",
        **kwargs
    ) -> str:
        """
        Get coding help from Claude Opus.
        
        Args:
            question: The coding question
            context: Optional additional context
            language: Primary programming language
            
        Returns:
            Response with paste URL if long, or direct response if short
        """
        if not self.bearer_token:
            return "Error: AWS_BEARER_TOKEN_BEDROCK not configured"
        
        # Build the prompt
        prompt_parts = [
            "You are an expert programmer helping with a coding question.",
            "Provide clear, well-documented code examples with explanations.",
            "Use best practices and idiomatic patterns for the language.",
            ""
        ]
        
        if language:
            prompt_parts.append(f"Primary language: {language}")
        
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
