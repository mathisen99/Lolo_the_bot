"""
Knowledge Base tool implementations.

Provides tools for ingesting documents into and retrieving from the knowledge base.
"""

from typing import Any, Dict
from .base import Tool


class KnowledgeBaseLearnTool(Tool):
    """Tool for learning from URLs and storing in knowledge base."""
    
    def __init__(self):
        """Initialize the learn tool."""
        self._manager = None
    
    @property
    def name(self) -> str:
        return "kb_learn"
    
    def _get_manager(self):
        """Lazy-load the manager to avoid import issues at startup."""
        if self._manager is None:
            from internal.knowledge.manager import KnowledgeBaseManager
            self._manager = KnowledgeBaseManager()
        return self._manager
    
    def get_definition(self) -> Dict[str, Any]:
        """Get tool definition for OpenAI API."""
        return {
            "type": "function",
            "name": "kb_learn",
            "description": """Learn content from a URL and store it in the knowledge base for future retrieval.

USE THIS WHEN:
- User says "learn this", "remember this", "save this for later"
- User shares a PDF, article, or documentation URL they want you to remember
- User links to research papers (e.g., arxiv.org PDFs) for future reference

SUPPORTED CONTENT:
- PDF documents (text extraction, images ignored)
- Web pages (article content extraction)
- Plain text, markdown, etc.

The content is chunked and embedded for efficient semantic search later.
Use kb_search to retrieve learned content.""",
            "parameters": {
                "type": "object",
                "properties": {
                    "url": {
                        "type": "string",
                        "description": "The URL to fetch and learn from. Must be a valid http/https URL."
                    }
                },
                "required": ["url"],
                "additionalProperties": False
            }
        }
    
    def execute(self, url: str, **kwargs) -> str:
        """
        Learn content from a URL.
        
        Args:
            url: The URL to ingest.
            
        Returns:
            Success/error message.
        """
        try:
            manager = self._get_manager()
            result = manager.learn_from_url(url)
            
            if result["success"]:
                return f"✓ Learned: '{result['title']}' - Stored {result['chunks_added']} chunks. Use kb_search to retrieve this information later."
            else:
                return f"Error: {result['message']}"
        except Exception as e:
            return f"Error learning from URL: {str(e)}"


class KnowledgeBaseSearchTool(Tool):
    """Tool for searching the knowledge base."""
    
    def __init__(self):
        """Initialize the search tool."""
        self._manager = None
    
    @property
    def name(self) -> str:
        return "kb_search"
    
    def _get_manager(self):
        """Lazy-load the manager."""
        if self._manager is None:
            from internal.knowledge.manager import KnowledgeBaseManager
            self._manager = KnowledgeBaseManager()
        return self._manager
    
    def get_definition(self) -> Dict[str, Any]:
        """Get tool definition for OpenAI API."""
        return {
            "type": "function",
            "name": "kb_search",
            "description": """Search the internal knowledge base for relevant information.

USE THIS WHEN:
- User asks about something that was previously learned via kb_learn
- User says "do you know about X", "what did that paper say about Y"
- User asks about internal/saved/learned content
- Questions about research papers, documentation previously ingested

This performs semantic search and returns the most relevant chunks.

DO NOT USE for general web searches - use web_search instead.
Use ONLY for content that was previously learned/ingested.""",
            "parameters": {
                "type": "object",
                "properties": {
                    "query": {
                        "type": "string",
                        "description": "The search query. Be specific about what you're looking for."
                    },
                    "n_results": {
                        "type": "integer",
                        "description": "Number of relevant chunks to retrieve (default: 5, max: 10).",
                        "default": 5
                    }
                },
                "required": ["query"],
                "additionalProperties": False
            }
        }
    
    def execute(self, query: str, n_results: int = 5, **kwargs) -> str:
        """
        Search the knowledge base.
        
        Args:
            query: The search query.
            n_results: Number of results to return.
            
        Returns:
            Formatted results or error message.
        """
        try:
            manager = self._get_manager()
            
            # Clamp n_results
            n_results = min(max(1, n_results), 10)
            
            results = manager.search(query, n_results=n_results)
            
            if not results:
                # Check if KB is empty
                sources = manager.list_sources()
                if not sources:
                    return "Knowledge base is empty. Use kb_learn to add content first."
                return f"No relevant results found for '{query}'. Knowledge base contains {len(sources)} sources: " + ", ".join(s['title'] for s in sources[:5])
            
            # Format results
            output_parts = [f"Found {len(results)} relevant chunk(s):\n"]
            
            for i, r in enumerate(results, 1):
                output_parts.append(f"\n--- Result {i} (from '{r['title']}') ---")
                output_parts.append(r['text'])
                output_parts.append(f"Source: {r['source_url']}")
            
            return "\n".join(output_parts)
            
        except Exception as e:
            return f"Error searching knowledge base: {str(e)}"


class KnowledgeBaseListTool(Tool):
    """Tool for listing sources in the knowledge base."""
    
    def __init__(self):
        """Initialize the list tool."""
        self._manager = None
    
    @property
    def name(self) -> str:
        return "kb_list"
    
    def _get_manager(self):
        """Lazy-load the manager."""
        if self._manager is None:
            from internal.knowledge.manager import KnowledgeBaseManager
            self._manager = KnowledgeBaseManager()
        return self._manager
    
    def get_definition(self) -> Dict[str, Any]:
        """Get tool definition for OpenAI API."""
        return {
            "type": "function",
            "name": "kb_list",
            "description": """List all sources currently stored in the knowledge base.

USE THIS WHEN:
- User asks "what have you learned?", "what's in your memory?"
- User wants to see what documents are available for querying
- Before searching, to check if relevant content exists""",
            "parameters": {
                "type": "object",
                "properties": {},
                "additionalProperties": False
            }
        }
    
    def execute(self, **kwargs) -> str:
        """
        List all sources in the knowledge base.
        
        Returns:
            Formatted list of sources.
        """
        try:
            manager = self._get_manager()
            sources = manager.list_sources()
            
            if not sources:
                return "Knowledge base is empty. No documents have been learned yet. Ask the user to share a URL to learn from."
            
            # Build detailed output that the LLM should present to user
            output_parts = [
                f"I have learned {len(sources)} document(s). Here's what I know about:",
                ""
            ]
            
            for i, s in enumerate(sources, 1):
                output_parts.append(f"{i}. **{s['title']}**")
                output_parts.append(f"   Source: {s['url']}")
                output_parts.append(f"   ({s['chunks']} text chunks stored)")
                output_parts.append("")
            
            output_parts.append("You can ask me questions about any of these topics and I'll search my knowledge base for relevant information.")
            
            return "\n".join(output_parts)
            
        except Exception as e:
            return f"Error listing knowledge base: {str(e)}"


class KnowledgeBaseForgetTool(Tool):
    """Tool for removing content from the knowledge base."""
    
    def __init__(self):
        """Initialize the forget tool."""
        self._manager = None
    
    @property
    def name(self) -> str:
        return "kb_forget"
    
    def _get_manager(self):
        """Lazy-load the manager."""
        if self._manager is None:
            from internal.knowledge.manager import KnowledgeBaseManager
            self._manager = KnowledgeBaseManager()
        return self._manager
    
    def get_definition(self) -> Dict[str, Any]:
        """Get tool definition for OpenAI API."""
        return {
            "type": "function",
            "name": "kb_forget",
            "description": """Remove a previously learned URL from the knowledge base.

USE THIS WHEN:
- User says "forget this", "remove this from memory"
- User wants to re-learn a URL with updated content (forget then learn again)""",
            "parameters": {
                "type": "object",
                "properties": {
                    "url": {
                        "type": "string",
                        "description": "The URL to remove from the knowledge base."
                    }
                },
                "required": ["url"],
                "additionalProperties": False
            }
        }
    
    def execute(self, url: str, **kwargs) -> str:
        """
        Forget content from a URL.
        
        Args:
            url: The URL to forget.
            
        Returns:
            Success/error message.
        """
        try:
            manager = self._get_manager()
            result = manager.forget_url(url)
            
            if result["success"]:
                return f"✓ Removed {result['chunks_removed']} chunks from knowledge base."
            else:
                return f"Error: {result['message']}"
        except Exception as e:
            return f"Error forgetting URL: {str(e)}"
